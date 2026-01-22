# 四维监控指标文档

> 版本：v1.0.0 | 最后更新：2026-01-22

## 概述

四维监控指标用于监控 multicloud-exporter 的账号、产品、区域、资源四个维度的状态和健康度。通过这些指标，可以将问题诊断时间从小时缩短到分钟级别。

## 指标分类

### 1. 账号级指标

#### 账号状态总数
**指标名称**：`multicloud_account_status_total`

**类型**：Gauge

**标签**：
- `account_id`: 账号 ID
- `cloud_provider`: 云厂商（aliyun/tencent/huawei/aws）
- `status`: 状态（active/degraded/disabled）

**说明**：记录每个账号的当前状态。状态值：
- `active`: 账号正常采集
- `degraded`: 账号已降级，部分功能受限
- `disabled`: 账号已禁用，不进行采集

**Prometheus 查询示例**：
```promql
# 查询所有账号状态
multicloud_account_status_total

# 查询 degraded 状态的账号
multicloud_account_status_total{status="degraded"}

# 统计各状态账号数量
count by (status) (multicloud_account_status_total)
```

---

#### 账号跳过次数
**指标名称**：`multicloud_account_skip_total`

**类型**：Counter

**标签**：
- `account_id`: 账号 ID
- `cloud_provider`: 云厂商
- `reason`: 跳过原因（disabled/isolated/provider_not_found/adapter_not_found）

**说明**：记录账号采集被跳过的次数和原因。常见原因：
- `disabled`: 账号被禁用
- `isolated`: 账号被隔离（故障容限）
- `provider_not_found`: Provider 未注册
- `adapter_not_found`: 适配器未找到

**Prometheus 查询示例**：
```promql
# 查询账号跳过次数
rate(multicloud_account_skip_total[5m])

# 按原因统计跳过次数
sum by (reason) (rate(multicloud_account_skip_total[5m]))
```

---

#### 账号降级次数
**指标名称**：`multicloud_account_degraded_total`

**类型**：Counter

**标签**：
- `account_id`: 账号 ID
- `cloud_provider`: 云厂商
- `reason`: 降级原因（api_limit/timeout/network_error/cluster_sync）

**说明**：记录账号被降级的次数和原因。

**Prometheus 查询示例**：
```promql
# 查询账号降级次数
rate(multicloud_account_degraded_total[5m])

# 查询最近 1 小时降级次数
increase(multicloud_account_degraded_total[1h])
```

---

#### 账号状态变更次数
**指标名称**：`multicloud_account_status_change`

**类型**：Counter

**标签**：
- `account_id`: 账号 ID
- `cloud_provider`: 云厂商
- `old_status`: 旧状态
- `new_status`: 新状态
- `reason`: 变更原因（api_error/cluster_sync/manual）

**说明**：记录账号状态变更的次数和详细信息。

**Prometheus 查询示例**：
```promql
# 查询账号状态变更次数
rate(multicloud_account_status_change[5m])

# 查询从 active 变为 degraded 的次数
multicloud_account_status_change{old_status="active", new_status="degraded"}
```

---

### 2. 产品级指标

#### 产品状态总数
**指标名称**：`multicloud_product_status_total`

**类型**：Gauge

**标签**：
- `account_id`: 账号 ID
- `product_id`: 产品 ID（slb/cbwp/oss/rds/ecs 等）
- `status`: 状态（active/degraded/disabled）

**说明**：记录每个产品的当前状态。

**Prometheus 查询示例**：
```promql
# 查询所有产品状态
multicloud_product_status_total

# 查询 degraded 状态的产品
multicloud_product_status_total{status="degraded"}

# 统计各状态产品数量
count by (status) (multicloud_product_status_total)
```

---

#### 产品跳过次数
**指标名称**：`multicloud_product_skip_total`

**类型**：Counter

**标签**：
- `account_id`: 账号 ID
- `product_id`: 产品 ID
- `reason`: 跳过原因（isolated/provider_not_found/adapter_not_found）

**说明**：记录产品采集被跳过的次数和原因。

**Prometheus 查询示例**：
```promql
# 查询产品跳过次数
rate(multicloud_product_skip_total[5m])

# 按原因统计跳过次数
sum by (reason) (rate(multicloud_product_skip_total[5m]))
```

---

#### 产品降级次数
**指标名称**：`multicloud_product_degraded_total`

**类型**：Counter

**标签**：
- `account_id`: 账号 ID
- `product_id`: 产品 ID
- `reason`: 降级原因（api_limit/timeout/network_error/cluster_sync）

**说明**：记录产品被降级的次数和原因。

**Prometheus 查询示例**：
```promql
# 查询产品降级次数
rate(multicloud_product_degraded_total[5m])

# 查询最近 1 小时降级次数
increase(multicloud_product_degraded_total[1h])
```

---

### 3. 区域级指标

#### 区域状态总数
**指标名称**：`multicloud_region_status_total`

**类型**：Gauge

**标签**：
- `account_id`: 账号 ID
- `product_id`: 产品 ID
- `region`: 区域（cn-hangzhou/cn-beijing 等）
- `status`: 状态（active/degraded/disabled）

**说明**：记录每个区域的当前状态。

**Prometheus 查询示例**：
```promql
# 查询所有区域状态
multicloud_region_status_total

# 查询 degraded 状态的区域
multicloud_region_status_total{status="degraded"}

# 统计各状态区域数量
count by (status) (multicloud_region_status_total)
```

---

#### 区域跳过次数
**指标名称**：`multicloud_region_skip_total`

**类型**：Counter

**标签**：
- `account_id`: 账号 ID
- `product_id`: 产品 ID
- `region`: 区域
- `reason`: 跳过原因（isolated/provider_not_found/adapter_not_found）

**说明**：记录区域采集被跳过的次数和原因。

**Prometheus 查询示例**：
```promql
# 查询区域跳过次数
rate(multicloud_region_skip_total[5m])

# 按原因统计跳过次数
sum by (reason) (rate(multicloud_region_skip_total[5m]))
```

---

#### 区域降级次数
**指标名称**：`multicloud_region_degraded_total`

**类型**：Counter

**标签**：
- `account_id`: 账号 ID
- `product_id`: 产品 ID
- `region`: 区域
- `reason`: 降级原因（api_limit/timeout/network_error/cluster_sync）

**说明**：记录区域被降级的次数和原因。

**Prometheus 查询示例**：
```promql
# 查询区域降级次数
rate(multicloud_region_degraded_total[5m])

# 查询最近 1 小时降级次数
increase(multicloud_region_degraded_total[1h])
```

---

### 4. 资源级指标

#### 资源状态总数
**指标名称**：`multicloud_resource_status_total`

**类型**：Gauge

**标签**：
- `account_id`: 账号 ID
- `product_id`: 产品 ID
- `region`: 区域
- `resource_id`: 资源 ID
- `status`: 状态（active/degraded/disabled）

**说明**：记录每个资源的当前状态。

**Prometheus 查询示例**：
```promql
# 查询所有资源状态
multicloud_resource_status_total

# 查询 degraded 状态的资源
multicloud_resource_status_total{status="degraded"}

# 统计各状态资源数量
count by (status) (multicloud_resource_status_total)
```

---

#### 资源跳过次数
**指标名称**：`multicloud_resource_skip_total`

**类型**：Counter

**标签**：
- `account_id`: 账号 ID
- `product_id`: 产品 ID
- `region`: 区域
- `resource_id`: 资源 ID
- `reason`: 跳过原因（isolated/provider_not_found/adapter_not_found）

**说明**：记录资源采集被跳过的次数和原因。

**Prometheus 查询示例**：
```promql
# 查询资源跳过次数
rate(multicloud_resource_skip_total[5m])

# 按原因统计跳过次数
sum by (reason) (rate(multicloud_resource_skip_total[5m]))
```

---

#### 资源降级次数
**指标名称**：`multicloud_resource_degraded_total`

**类型**：Counter

**标签**：
- `account_id`: 账号 ID
- `product_id`: 产品 ID
- `region`: 区域
- `resource_id`: 资源 ID
- `reason`: 降级原因（api_limit/timeout/network_error/cluster_sync）

**说明**：记录资源被降级的次数和原因。

**Prometheus 查询示例**：
```promql
# 查询资源降级次数
rate(multicloud_resource_degraded_total[5m])

# 查询最近 1 小时降级次数
increase(multicloud_resource_degraded_total[1h])
```

---

## Grafana 告警规则

### 账号级告警

#### 账号降级告警
```yaml
- alert: MulticloudAccountDegraded
  expr: multicloud_account_status_total{status="degraded"} > 0
  for: 5m
  labels:
    severity: warning
    dimension: account
  annotations:
    summary: "账号已降级: {{ $labels.account_id }}"
    description: "云厂商 {{ $labels.cloud_provider }} 的账号 {{ $labels.account_id }} 已降级超过 5 分钟"
```

#### 账号跳过率告警
```yaml
- alert: MulticloudAccountHighSkipRate
  expr: rate(multicloud_account_skip_total[5m]) > 0.1
  for: 10m
  labels:
    severity: warning
    dimension: account
  annotations:
    summary: "账号跳过率过高: {{ $labels.account_id }}"
    description: "账号 {{ $labels.account_id }} 的跳过率超过 0.1 次/分钟"
```

---

### 产品级告警

#### 产品降级告警
```yaml
- alert: MulticloudProductDegraded
  expr: multicloud_product_status_total{status="degraded"} > 0
  for: 5m
  labels:
    severity: warning
    dimension: product
  annotations:
    summary: "产品已降级: {{ $labels.product_id }}"
    description: "账号 {{ $labels.account_id }} 的产品 {{ $labels.product_id }} 已降级超过 5 分钟"
```

#### 产品跳过率告警
```yaml
- alert: MulticloudProductHighSkipRate
  expr: rate(multicloud_product_skip_total[5m]) > 0.1
  for: 10m
  labels:
    severity: warning
    dimension: product
  annotations:
    summary: "产品跳过率过高: {{ $labels.product_id }}"
    description: "产品 {{ $labels.product_id }} 的跳过率超过 0.1 次/分钟"
```

---

### 区域级告警

#### 区域降级告警
```yaml
- alert: MulticloudRegionDegraded
  expr: multicloud_region_status_total{status="degraded"} > 0
  for: 5m
  labels:
    severity: warning
    dimension: region
  annotations:
    summary: "区域已降级: {{ $labels.region }}"
    description: "账号 {{ $labels.account_id }} 产品 {{ $labels.product_id }} 的区域 {{ $labels.region }} 已降级超过 5 分钟"
```

#### 区域跳过率告警
```yaml
- alert: MulticloudRegionHighSkipRate
  expr: rate(multicloud_region_skip_total[5m]) > 0.1
  for: 10m
  labels:
    severity: warning
    dimension: region
  annotations:
    summary: "区域跳过率过高: {{ $labels.region }}"
    description: "区域 {{ $labels.region }} 的跳过率超过 0.1 次/分钟"
```

---

### 资源级告警

#### 资源降级告警
```yaml
- alert: MulticloudResourceDegraded
  expr: multicloud_resource_status_total{status="degraded"} > 0
  for: 5m
  labels:
    severity: warning
    dimension: resource
  annotations:
    summary: "资源已降级: {{ $labels.resource_id }}"
    description: "资源 {{ $labels.resource_id }} 已降级超过 5 分钟"
```

#### 资源跳过率告警
```yaml
- alert: MulticloudResourceHighSkipRate
  expr: rate(multicloud_resource_skip_total[5m]) > 0.1
  for: 10m
  labels:
    severity: warning
    dimension: resource
  annotations:
    summary: "资源跳过率过高: {{ $labels.resource_id }}"
    description: "资源 {{ $labels.resource_id }} 的跳过率超过 0.1 次/分钟"
```

---

## 使用场景

### 场景 1：快速定位采集失败

**问题**：某个账号的指标一直没有更新

**诊断步骤**：
1. 查询账号状态
   ```promql
   multicloud_account_status_total{account_id="your-account-id"}
   ```
2. 如果状态为 `degraded`，查询降级原因
   ```promql
   multicloud_account_degraded_total{account_id="your-account-id"}
   ```
3. 根据原因采取行动（如调整 API 限流、修复网络问题等）

### 场景 2：监控降级恢复

**问题**：降级的资源是否自动恢复

**诊断步骤**：
1. 查询降级次数趋势
   ```promql
   rate(multicloud_product_degraded_total[1h])
   ```
2. 查询状态变更
   ```promql
   multicloud_account_status_change{new_status="active"}
   ```
3. 验证自动恢复机制是否正常工作

### 场景 3：集群同步监控

**问题**：集群模式下不同 Pod 是否同步状态

**诊断步骤**：
1. 查询集群同步原因
   ```promql
   multicloud_product_degraded_total{reason="cluster_sync"}
   ```
2. 查询集群同步失败次数
   ```promql
   rate(multicloud_broadcast_failed_total[5m])
   ```

---

## 最佳实践

### 1. 监控维度选择

- **账号级**：用于监控整体云账号健康度
- **产品级**：用于定位具体产品问题
- **区域级**：用于区域级问题排查
- **资源级**：用于精细级资源监控

### 2. 告警级别建议

- **Critical**：账号降级超过 15 分钟
- **Warning**：产品/区域降级超过 5 分钟
- **Info**：资源降级或状态变更

### 3. 查询时间窗口

- **实时监控**：使用 `rate()` 函数和 5m 窗口
- **趋势分析**：使用 `increase()` 函数和 1h 窗口
- **状态快照**：直接查询 Gauge 指标

---

## 相关文档

- [架构规范](../.opencode/rules/05-architecture.md)
- [指标映射规范](../.opencode/rules/04-metrics.md)
- [API 调用规范](../.opencode/rules/03-api.md)
- [质量保证](../.opencode/rules/07-quality.md)
