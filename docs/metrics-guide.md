# 监控指标使用指南

本文档说明如何使用 multicloud-exporter 的监控指标，包括派生指标的计算方法。

## 目录
- [基础指标](#基础指标)
- [派生指标](#派生指标)
- [常用 PromQL 查询示例](#常用-promql-查询示例)

## 基础指标

### 1. API 调用指标

#### RequestTotal（请求总数）
```yaml
multicloud_request_total{cloud_provider="aliyun", api="DescribeMetricList", status="success"}
```
- **标签**：
  - `cloud_provider`: 云厂商（aliyun/tencent/aws/huawei）
  - `api`: API 名称
  - `status`: 请求状态（success/auth_error/limit_error/region_skip/network_error/error）

#### RequestDuration（请求耗时）
```yaml
multicloud_request_duration_seconds{cloud_provider="aliyun", api="DescribeMetricList"}
```
- **标签**：
  - `cloud_provider`: 云厂商
  - `api`: API 名称

#### RateLimitTotal（限流次数）
```yaml
multicloud_rate_limit_total{cloud_provider="aliyun", api="DescribeMetricList"}
```
- **标签**：
  - `cloud_provider`: 云厂商
  - `api`: API 名称

### 2. 采集指标

#### CollectionDuration（采集周期总耗时）
```yaml
multicloud_collection_duration_seconds
```

#### SampleCountTotal（样本总数）
```yaml
multicloud_sample_count_total{account_id="xxx", region="cn-hangzhou", resource_type="slb", namespace="acs_slb_dashboard"}
```
- **标签**：
  - `account_id`: 账号 ID
  - `region`: 区域
  - `resource_type`: 资源类型
  - `namespace`: 命名空间

### 3. 缓存指标

#### CacheHitTotal（缓存命中次数）
```yaml
multicloud_cache_hit_total{cache_type="resource_discovery"}
```

#### CacheMissTotal（缓存未命中次数）
```yaml
multicloud_cache_miss_total{cache_type="resource_discovery"}
```

#### CacheHitRatio（缓存命中率）
```yaml
multicloud_cache_hit_ratio{cache_type="resource_discovery"}
```

#### CacheSizeBytes（缓存大小）
```yaml
multicloud_cache_size_bytes{cache_type="resource_discovery"}
```

#### CacheEntriesTotal（缓存条目数）
```yaml
multicloud_cache_entries_total{cache_type="resource_discovery"}
```

### 4. 区域发现指标

#### RegionStatus（区域状态统计）
```yaml
multicloud_region_status_total{cloud_provider="aliyun", status="active"}
```
- **status**: active/empty/unknown

#### RegionDiscoveryDuration（区域发现耗时）
```yaml
multicloud_region_discovery_duration_seconds{cloud_provider="aliyun"}
```

#### RegionSkippedTotal（跳过的空区域次数）
```yaml
multicloud_region_skip_total{cloud_provider="aliyun"}
```

### 5. 集群配置指标

#### ClusterConfigRefreshTotal（集群配置刷新次数）
```yaml
multicloud_cluster_config_refresh_total
```

#### ClusterConfigRefreshDuration（集群配置刷新耗时）
```yaml
multicloud_cluster_config_refresh_duration_seconds
```

#### ClusterConfigTotal（当前集群总 Pod 数）
```yaml
multicloud_cluster_config_total
```

#### ClusterConfigIndex（当前 Pod 索引）
```yaml
multicloud_cluster_config_index
```

#### FirstRunDelaySeconds（首次采集延迟）
```yaml
multicloud_first_run_delay_seconds{pod_index="0", strategy="sequential"}
```

## 派生指标

### 1. API 调用成功率

#### 计算方法
```promql
# 整体 API 调用成功率
rate(multicloud_request_total{status="success"}[5m]) / rate(multicloud_request_total[5m])

# 按云厂商统计的 API 调用成功率
rate(multicloud_request_total{status="success"}[5m]) / ignoring(status) rate(multicloud_request_total[5m])

# 按 API 统计的成功率
rate(multicloud_request_total{status="success"}[5m]) / ignoring(status) group_left() rate(multicloud_request_total[5m])
```

#### 查询示例
```promql
# 最近 5 分钟阿里云 API 调用成功率
rate(multicloud_request_total{cloud_provider="aliyun", status="success"}[5m]) / ignoring(status) rate(multicloud_request_total{cloud_provider="aliyun"}[5m])

# 按错误类型统计的失败率
rate(multicloud_request_total{status=~"auth_error|limit_error|network_error"}[5m])
```

### 2. 缓存命中率

#### 计算方法
```promql
# 资源发现缓存命中率
rate(multicloud_cache_hit_total{cache_type="resource_discovery"}[5m]) / (rate(multicloud_cache_hit_total{cache_type="resource_discovery"}[5m]) + rate(multicloud_cache_miss_total{cache_type="resource_discovery"}[5m]))
```

#### 查询示例
```promql
# 最近 5 分钟资源发现缓存命中率
rate(multicloud_cache_hit_total{cache_type="resource_discovery"}[5m]) / (rate(multicloud_cache_hit_total{cache_type="resource_discovery"}[5m]) + rate(multicloud_cache_miss_total{cache_type="resource_discovery"}[5m]))
```

### 3. 限流率

#### 计算方法
```promql
# 限流率（限流次数 / 总请求数）
rate(multicloud_rate_limit_total[5m]) / rate(multicloud_request_total[5m])
```

#### 查询示例
```promql
# 阿里云最近 5 分钟的限流率
rate(multicloud_rate_limit_total{cloud_provider="aliyun"}[5m]) / rate(multicloud_request_total{cloud_provider="aliyun"}[5m])
```

### 4. API 请求错误率

#### 计算方法
```promql
# 按错误类型统计的错误率
rate(multicloud_request_total{status=~"auth_error|limit_error|network_error|error"}[5m]) / rate(multicloud_request_total[5m])
```

#### 查询示例
```promql
# 最近 5 分钟认证错误率
rate(multicloud_request_total{status="auth_error"}[5m]) / rate(multicloud_request_total[5m])

# 最近 5 分钟限流错误率
rate(multicloud_request_total{status="limit_error"}[5m]) / rate(multicloud_request_total[5m])
```

### 5. 采集效率指标

#### 计算方法
```promql
# 平均每秒采集的样本数
rate(multicloud_sample_count_total[1m])

# 按账号统计的采集样本数
sum by (account_id) (multicloud_sample_count_total)

# 按区域统计的采集样本数
sum by (region) (multicloud_sample_count_total)

# 按资源类型统计的采集样本数
sum by (resource_type) (multicloud_sample_count_total)
```

### 6. 分片健康度指标

#### 计算方法
```promql
# 集群 Pod 数（应稳定）
multicloud_cluster_config_total

# 当前 Pod 索引（0 ~ total-1）
multicloud_cluster_config_index

# 分片配置刷新频率
rate(multicloud_cluster_config_refresh_total[5m])

# 分片配置刷新耗时
multicloud_cluster_config_refresh_duration_seconds
```

#### 查询示例
```promql
# 检查集群配置是否稳定（Pod 数量变化）
changes(multicloud_cluster_config_total[1h])

# 检查分片配置是否频繁刷新
rate(multicloud_cluster_config_refresh_total[1h]) > 0.1
```

### 7. 区域健康度指标

#### 计算方法
```promql
# 活跃区域数
multicloud_region_status_total{status="active"}

# 空区域数
multicloud_region_status_total{status="empty"}

# 未知区域数
multicloud_region_status_total{status="unknown"}

# 跳过区域率
rate(multicloud_region_skip_total[5m])
```

#### 查询示例
```promql
# 检查各云厂商的区域分布
sum by (cloud_provider) (multicloud_region_status_total)

# 检查是否有大量未知区域
multicloud_region_status_total{status="unknown"} / ignoring(status) group_left() sum by (cloud_provider) (multicloud_region_status_total)
```

## 常用 PromQL 查询示例

### 1. 监控告警规则示例

#### API 调用成功率过低
```yaml
- alert: APISuccessRateTooLow
  expr: rate(multicloud_request_total{status="success"}[5m]) / ignoring(status) rate(multicloud_request_total[5m]) < 0.95
  for: 5m
  labels:
    severity: warning
  annotations:
    summary: "API 调用成功率过低"
    description: "云厂商 {{ $labels.cloud_provider }} 的 API 调用成功率为 {{ $value | humanizePercentage }}"
```

#### 限流率过高
```yaml
- alert: RateLimitTooHigh
  expr: rate(multicloud_rate_limit_total[5m]) / rate(multicloud_request_total[5m]) > 0.1
  for: 5m
  labels:
    severity: warning
  annotations:
    summary: "API 限流率过高"
    description: "云厂商 {{ $labels.cloud_provider }} 的限流率为 {{ $value | humanizePercentage }}"
```

#### 采集周期过长
```yaml
- alert: CollectionDurationTooHigh
  expr: multicloud_collection_duration_seconds > 120
  for: 5m
  labels:
    severity: warning
  annotations:
    summary: "采集周期过长"
    description: "最近一次采集耗时 {{ $value }}s"
```

#### 缓存命中率过低
```yaml
- alert: CacheHitRateTooLow
  expr: rate(multicloud_cache_hit_total[5m]) / (rate(multicloud_cache_hit_total[5m]) + rate(multicloud_cache_miss_total[5m])) < 0.5
  for: 5m
  labels:
    severity: info
  annotations:
    summary: "缓存命中率过低"
    description: "缓存类型 {{ $labels.cache_type }} 的命中率为 {{ $value | humanizePercentage }}"
```

### 2. Grafana Dashboard 面板建议

#### API 调用概览
- **API 调用成功率**：`rate(multicloud_request_total{status="success"}[5m]) / ignoring(status) rate(multicloud_request_total[5m])`
- **API 请求总量**：`sum(rate(multicloud_request_total[5m]))`
- **限流次数**：`sum(rate(multicloud_rate_limit_total[5m]))`
- **平均请求延迟**：`histogram_quantile(0.95, sum(rate(multicloud_request_duration_seconds_bucket[5m])) by (le))`

#### 采集概览
- **采集周期耗时**：`multicloud_collection_duration_seconds`
- **样本采集数**：`sum(increase(multicloud_sample_count_total[1m]))`
- **活跃区域数**：`sum(multicloud_region_status_total{status="active"})`
- **缓存命中率**：`rate(multicloud_cache_hit_total[5m]) / (rate(multicloud_cache_hit_total[5m]) + rate(multicloud_cache_miss_total[5m]))`

#### 集群健康度
- **集群 Pod 数**：`multicloud_cluster_config_total`
- **当前 Pod 索引**：`multicloud_cluster_config_index`
- **分片配置刷新次数**：`rate(multicloud_cluster_config_refresh_total[1h])`
- **分片配置刷新耗时**：`multicloud_cluster_config_refresh_duration_seconds`

## 注意事项

1. **指标基数**：Counter 类型指标（如 `*_total`）应使用 `rate()` 或 `increase()` 函数计算速率
2. **时间窗口**：根据采集频率选择合适的时间窗口（如 `[5m]`、`[1h]`）
3. **标签过滤**：使用标签过滤查询特定指标（如 `{cloud_provider="aliyun"}`）
4. **聚合函数**：使用 `sum()`、`avg()`、`max()` 等聚合函数计算汇总指标
5. **性能考虑**：复杂的 PromQL 查询可能影响 Grafana 性能，建议合理使用 Recording Rules

## 扩展指标

如果需要添加新的监控指标，请参考以下步骤：

1. 在 `internal/metrics/metrics.go` 中定义指标
2. 在代码中适当位置调用指标记录函数
3. 更新本文档说明新指标的用途和查询方法
4. 更新 Grafana Dashboard 面板

## 参考资源

- [Prometheus 查询语言文档](https://prometheus.io/docs/prometheus/latest/querying/basics/)
- [PromQL 函数参考](https://prometheus.io/docs/prometheus/latest/querying/functions/)
- [Grafana 变量参考](https://grafana.com/docs/grafana/latest/variables/)
