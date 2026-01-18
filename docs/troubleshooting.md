# 故障排查指南

本文档提供 multicloud-exporter 常见问题的排查方法和解决方案。

## 目录
- [日志排查](#日志排查)
- [指标排查](#指标排查)
- [常见问题](#常见问题)
  - [采集失败](#采集失败)
  - [限流问题](#限流问题)
  - [性能问题](#性能问题)
  - [分片问题](#分片问题)
  - [内存问题](#内存问题)
- [监控告警](#监控告警)

## 日志排查

### 日志级别

项目支持以下日志级别：

| 日志级别 | 说明 | 使用场景 |
|---------|------|---------|
| `debug` | 详细调试信息 | 开发环境调试、详细错误堆栈 |
| `info` | 关键流程节点 | 正常运行信息、采集成功 |
| `warn` | 可恢复的错误 | API 限流重试、单个资源采集失败 |
| `error` | 需要人工介入的错误 | 凭证无效、配置解析失败 |

### 日志格式

日志默认使用 JSON 格式，便于日志采集和分析：

```json
{
  "level": "error",
  "msg": "S3客户端创建失败",
  "cloud_provider": "aws",
  "account_id": "xxx",
  "region": "us-east-1",
  "error": "no credential providers"
}
```

### 查看日志

#### 本地运行

```bash
# 查看实时日志
tail -f logs/exporter.log

# 查看错误日志
grep "level\":\"error" logs/exporter.log

# 查看限流错误
grep "Throttling\|Rate exceeded\|TooManyRequests" logs/exporter.log

# 查看认证错误
grep "AuthFailure\|InvalidAccessKeyId\|SignatureDoesNotMatch" logs/exporter.log
```

#### Kubernetes 部署

```bash
# 查看实时日志
kubectl -n monitoring logs -f deployment/multicloud-exporter

# 查看错误日志
kubectl -n monitoring logs deployment/multicloud-exporter | grep "level\":\"error"

# 查看最近 100 行日志
kubectl -n monitoring logs --tail=100 deployment/multicloud-exporter

# 查看特定 Pod 的日志
kubectl -n monitoring logs -f <pod-name>
```

## 指标排查

### 查看指标

```bash
# 查看所有指标
curl -s http://localhost:9101/metrics

# 查看请求总数
curl -s http://localhost:9101/metrics | grep multicloud_request_total

# 查看限流次数
curl -s http://localhost:9101/metrics | grep multicloud_rate_limit_total

# 查看采集耗时
curl -s http://localhost:9101/metrics | grep multicloud_collection_duration_seconds

# 查看样本数
curl -s http://localhost:9101/metrics | grep multicloud_sample_count_total
```

### Prometheus 查询

```promql
# API 调用成功率
rate(multicloud_request_total{status="success"}[5m]) / ignoring(status) rate(multicloud_request_total[5m])

# 限流率
rate(multicloud_rate_limit_total[5m]) / rate(multicloud_request_total[5m])

# 采集周期耗时
multicloud_collection_duration_seconds

# 采集样本数
sum(increase(multicloud_sample_count_total[1m]))
```

## 常见问题

### 采集失败

#### 问题：所有采集任务失败，没有指标输出

**排查步骤**：

1. **检查账号配置**
   ```bash
   # 查看账号配置是否正确
   cat configs/accounts.yaml
   
   # 检查日志中的认证错误
   kubectl -n monitoring logs deployment/multicloud-exporter | grep "level\":\"error"
   ```

2. **检查凭证有效性**
   - 验证 AccessKeyID 和 AccessKeySecret 是否正确
   - 验证账号是否有足够的权限访问相关资源
   - 检查凭证是否已过期

3. **检查网络连接**
   ```bash
   # 检查是否能访问云厂商 API
   curl https://cms.cn-hangzhou.aliyuncs.com
   
   # 检查 DNS 解析
   nslookup cms.cn-hangzhou.aliyuncs.com
   ```

**解决方案**：
- 更新账号配置中的凭证
- 为账号添加足够的权限
- 检查网络连接和防火墙规则

#### 问题：单个区域采集失败

**排查步骤**：

1. **检查区域配置**
   ```bash
   # 查看日志中的区域错误
   kubectl -n monitoring logs deployment/multicloud-exporter | grep "InvalidRegionId\|Unsupported"
   ```

2. **检查账号区域权限**
   - 验证账号是否有该区域的访问权限
   - 验证该区域是否已启用相关服务

**解决方案**：
- 在账号配置中移除不支持的区域
- 为账号添加该区域的访问权限

### 限流问题

#### 问题：API 调用频繁触发限流

**排查步骤**：

1. **查看限流指标**
   ```promql
   # 限流率
   rate(multicloud_rate_limit_total[5m]) / rate(multicloud_request_total[5m])
   
   # 按云厂商统计的限流次数
   sum(rate(multicloud_rate_limit_total[5m])) by (cloud_provider)
   
   # 按 API 统计的限流次数
   sum(rate(multicloud_rate_limit_total[5m])) by (api)
   ```

2. **查看并发配置**
   ```bash
   # 查看 region_concurrency、metric_concurrency、product_concurrency 配置
   cat configs/server.yaml | grep -A 3 "concurrency:"
   ```

3. **查看采集间隔**
   ```bash
   # 查看 scrape_interval 配置
   cat configs/server.yaml | grep scrape_interval
   ```

**解决方案**：

1. **降低并发配置**
   ```yaml
   server:
     region_concurrency: 2    # 降低区域级并发
     metric_concurrency: 1    # 降低指标级并发
     product_concurrency: 1   # 降低产品级并发
   ```

2. **延长采集间隔**
   ```yaml
   server:
     scrape_interval: 120s  # 从 60s 延长到 120s
   ```

3. **启用缓存优化**
   - 启用华为云缓存优化（`huawei_cache.enabled: true`）
   - 启用资源发现缓存（适当增加 `discovery_ttl`）

### 性能问题

#### 问题：采集周期过长

**排查步骤**：

1. **查看采集耗时**
   ```bash
   # 查看采集周期耗时
   curl -s http://localhost:9101/metrics | grep multicloud_collection_duration_seconds
   
   # Prometheus 查询
   multicloud_collection_duration_seconds
   ```

2. **查看 API 请求耗时**
   ```promql
   # 平均请求耗时
   rate(multicloud_request_duration_seconds_sum[5m]) / rate(multicloud_request_duration_seconds_count[5m])
   
   # P95 请求耗时
   histogram_quantile(0.95, sum(rate(multicloud_request_duration_seconds_bucket[5m])) by (le))
   ```

3. **检查并发配置**
   - 检查并发配置是否过高
   - 检查总并发度（`region_concurrency × metric_concurrency × product_concurrency`）

**解决方案**：

1. **优化并发配置**
   - 降低 `metric_concurrency` 和 `product_concurrency`
   - 确保总并发度不超过 20

2. **启用缓存优化**
   - 启用华为云缓存优化
   - 适当增加 `discovery_ttl`

3. **优化资源发现**
   - 减少不必要的资源采集
   - 使用区域发现功能跳过空区域

### 分片问题

#### 问题：多副本部署时数据重复或丢失

**排查步骤**：

1. **检查分片配置**
   ```bash
   # 查看集群配置
   kubectl -n monitoring get pods -o wide
   
   # 查看分片配置指标
   curl -s http://localhost:9101/metrics | grep multicloud_cluster_config
   ```

2. **检查分片逻辑**
   - 确认各云厂商实现了内部分片（`SupportsInternalSharding()`）
   - 检查日志中的分片警告

**解决方案**：

1. **验证内部分片实现**
   - 确认各云厂商的 Provider 实现了 `SupportsInternalSharding()` 方法
   - 如果未实现，需要添加账号级分片逻辑

2. **检查分片配置 TTL**
   - 确认 `discovery_ttl` 设置为采集间隔的一半
   - 检查分片配置是否频繁刷新

#### 问题：滚动更新时数据不一致

**排查步骤**：

1. **检查集群配置刷新频率**
   ```promql
   # 集群配置刷新频率
   rate(multicloud_cluster_config_refresh_total[1h])
   ```

2. **查看首次采集延迟**
   ```promql
   # 首次采集延迟
   multicloud_first_run_delay_seconds
   ```

**解决方案**：

1. **启用首次采集策略**
   ```yaml
   server:
     first_run:
       strategy: auto  # 根据副本数自动选择
       max_delay: 180  # 最大延迟 180 秒
   ```

2. **优化分片配置 TTL**
   - 将 `discovery_ttl` 设置为采集间隔的一半

### 内存问题

#### 问题：内存持续增长导致 OOM

**排查步骤**：

1. **查看内存指标**
   ```bash
   # 查看容器内存使用
   kubectl top pods -n monitoring
   
   # 查看缓存大小
   curl -s http://localhost:9101/metrics | grep multicloud_cache_size_bytes
   ```

2. **检查缓存配置**
   ```bash
   # 查看标签缓存 TTL
   cat configs/server.yaml | grep tag_cache_ttl
   ```

**解决方案**：

1. **检查标签缓存清理机制**
   - 确认标签缓存有 TTL 配置
   - 适当减小 `tag_cache_ttl`

2. **检查区域管理器内存限制**
   ```yaml
   region_discovery:
     enabled: true
     discovery_interval: 1h  # 定期重置区域状态
     empty_threshold: 3     # 跳过空区域
   ```

3. **增加内存限制**
   - 在 deployment 中增加 `resources.limits.memory`

## 监控告警

### 推荐告警规则

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

#### 集群配置不稳定

```yaml
- alert: ClusterConfigUnstable
  expr: changes(multicloud_cluster_config_total[1h]) > 1
  for: 5m
  labels:
    severity: warning
  annotations:
    summary: "集群配置不稳定"
    description: "最近 1 小时内集群 Pod 数量变化了 {{ $value }} 次"
```

## 参考资源

- [错误处理和重试策略](./error-handling.md)
- [监控指标使用指南](./metrics-guide.md)
- [错误分类实现](../internal/providers/common/errors.go)
- [重试机制实现](../internal/providers/common/retry.go)
