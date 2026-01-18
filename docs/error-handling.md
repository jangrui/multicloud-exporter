# 错误处理和重试策略

本文档说明 multicloud-exporter 的错误处理机制和重试策略。

## 目录
- [错误分类](#错误分类)
- [重试策略](#重试策略)
- [限流处理](#限流处理)
- [错误日志](#错误日志)
- [监控指标](#监控指标)

## 错误分类

项目使用统一的错误分类体系，将所有错误分为以下几类：

### 1. 认证错误（auth_error）
- **定义**：由于无效的访问密钥、签名不匹配或权限不足导致的错误
- **云厂商错误示例**：
  - 阿里云：`InvalidAccessKeyId`、`Forbidden`、`SignatureDoesNotMatch`
  - 腾讯云：`AuthFailure`、`InvalidCredential`
  - 华为云：`unauthorized`、`authentication failed`、`authenticate failed`
  - AWS：`ExpiredToken`、`InvalidClientTokenId`、`AccessDenied`
- **处理策略**：不重试，直接返回错误并记录日志

### 2. 限流错误（limit_error）
- **定义**：由于 API 调用频率过高触发的限流错误
- **云厂商错误示例**：
  - 阿里云：`Throttling`、`flow control`
  - 腾讯云：`RequestLimitExceeded`
  - 华为云：`APIGW.0308`、`throttling`、`429`、`TooManyRequests`、`rate limit`
  - AWS：`Throttling`、`Rate exceeded`、`TooManyRequests`
- **处理策略**：重试（指数退避），记录限流指标

### 3. 区域错误（region_skip）
- **定义**：由于不支持的区域或资源不存在导致的错误
- **云厂商错误示例**：
  - 阿里云：`InvalidRegionId`、`Unsupported`
  - 腾讯云：`InvalidParameter.Region`（推测）
  - 华为云：`region not supported`、`invalid region id`
  - AWS：`InvalidParameterValue`（区域不存在）
- **处理策略**：不重试，跳过该区域

### 4. 网络错误（network_error）
- **定义**：由于网络超时、连接失败或临时网络故障导致的错误
- **云厂商错误示例**：
  - 阿里云：`timeout`、`unreachable`、`Temporary network`
  - 腾讯云：`timeout`、`network`
  - 华为云：`timeout`、`unreachable`、`network`、`connection`、`temporarily unavailable`
  - AWS：`timeout`、`network`
- **处理策略**：重试（指数退避）

### 5. 未知错误（error）
- **定义**：无法明确分类的错误
- **处理策略**：不重试，记录到日志便于后续分析

## 重试策略

### 统一重试配置

所有 API 调用使用统一的重试配置，默认值如下：

| 配置项 | 默认值 | 说明 |
|-------|--------|------|
| `MaxAttempts` | 5 | 最大重试次数（不包括首次尝试）|
| `InitialDelay` | 200ms | 初始延迟时间 |
| `MaxDelay` | 5s | 最大延迟时间 |
| `BackoffFactor` | 2.0 | 退避因子（指数退避）|

### 重试决策

根据错误类型决定是否重试：

| 错误类型 | 是否重试 | 说明 |
|---------|---------|------|
| `auth_error` | ❌ 不重试 | 认证错误无法通过重试解决 |
| `limit_error` | ✅ 重试 | 限流错误应自动重试 |
| `region_skip` | ❌ 不重试 | 区域错误无法通过重试解决 |
| `network_error` | ✅ 重试 | 网络错误可能临时恢复 |
| `error`（未知） | ❌ 不重试 | 未知错误不重试，避免浪费资源 |

### 重试延迟计算

使用指数退避策略计算重试延迟：

```
延迟 = min(InitialDelay × BackoffFactor^(重试次数), MaxDelay)
```

示例：
- 第 1 次重试：200ms × 2^0 = 200ms
- 第 2 次重试：200ms × 2^1 = 400ms
- 第 3 次重试：200ms × 2^2 = 800ms
- 第 4 次重试：200ms × 2^3 = 1600ms
- 第 5 次重试：200ms × 2^4 = 3200ms

### 上下文取消

重试过程中会检查上下文是否已取消，支持优雅关闭：
- 如果上下文已取消，立即返回 `context.Canceled` 或 `context.DeadlineExceeded`
- 不会继续等待剩余的重试延迟

## 限流处理

### 限流检测

所有 API 调用都会检测限流错误，当检测到限流时：

1. **记录限流指标**：`multicloud_rate_limit_total{cloud_provider="aliyun", api="DescribeMetricList"}`
2. **继续重试**：按照指数退避策略自动重试
3. **记录日志**：记录限流错误，便于监控和排查

### 限流指标监控

通过 PromQL 查询限流率：

```promql
# 限流率（限流次数 / 总请求数）
rate(multicloud_rate_limit_total[5m]) / rate(multicloud_request_total[5m])
```

### 限流告警规则示例

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

## 错误日志

### 日志级别

项目使用 `zap` 日志库，支持结构化日志。根据错误类型使用不同的日志级别：

| 日志级别 | 适用场景 | 示例 |
|---------|---------|------|
| `Debug` | 详细调试信息（API 原始响应） | 描述完整的错误堆栈 |
| `Info` | 关键流程节点（采集成功） | 记录重试成功 |
| `Warn` | 可恢复的错误（API 限流重试） | 记录限流重试 |
| `Error` | 需要人工介入的错误（凭证无效） | 记录认证失败 |

### 日志格式

日志包含以下结构化字段：

```json
{
  "level": "warn",
  "msg": "API 调用失败，正在重试",
  "cloud_provider": "aliyun",
  "api": "DescribeMetricList",
  "account_id": "xxx",
  "region": "cn-hangzhou",
  "error": "Throttling",
  "attempt": 2,
  "max_attempts": 5,
  "delay": "400ms"
}
```

### 未知错误日志

对于未知错误，系统会记录特殊日志，便于后续分析：

```
UNKNOWN ERROR: provider=aliyun api=DescribeMetricList error=some unknown error message
```

## 监控指标

### API 调用指标

#### RequestTotal（请求总数）
```yaml
multicloud_request_total{cloud_provider="aliyun", api="DescribeMetricList", status="success"}
```
- **status**：success/auth_error/limit_error/region_skip/network_error/error

#### RequestDuration（请求耗时）
```yaml
multicloud_request_duration_seconds{cloud_provider="aliyun", api="DescribeMetricList"}
```

#### RateLimitTotal（限流次数）
```yaml
multicloud_rate_limit_total{cloud_provider="aliyun", api="DescribeMetricList"}
```

### API 调用成功率

```promql
# 整体 API 调用成功率
rate(multicloud_request_total{status="success"}[5m]) / ignoring(status) rate(multicloud_request_total[5m])

# 按云厂商统计的成功率
rate(multicloud_request_total{status="success"}[5m]) / ignoring(status) rate(multicloud_request_total[5m])

# 按 API 统计的成功率
rate(multicloud_request_total{status="success"}[5m]) / ignoring(status) group_left() rate(multicloud_request_total[5m])
```

### 限流率

```promql
# 限流率（限流次数 / 总请求数）
rate(multicloud_rate_limit_total[5m]) / rate(multicloud_request_total[5m])

# 阿里云最近 5 分钟的限流率
rate(multicloud_rate_limit_total{cloud_provider="aliyun"}[5m]) / rate(multicloud_request_total{cloud_provider="aliyun"}[5m])
```

### 错误率

```promql
# 按错误类型统计的错误率
rate(multicloud_request_total{status=~"auth_error|limit_error|network_error|error"}[5m]) / rate(multicloud_request_total[5m])

# 认证错误率
rate(multicloud_request_total{status="auth_error"}[5m]) / rate(multicloud_request_total[5m])

# 限流错误率
rate(multicloud_request_total{status="limit_error"}[5m]) / rate(multicloud_request_total[5m])
```

## 使用示例

### 统一重试使用

```go
// 1. 导入必要的包
import (
    "context"
    "multicloud-exporter/internal/providers/common"
)

// 2. 创建错误分类器
classifier := &common.AliyunErrorClassifier{}

// 3. 创建重试函数（仅对限流和网络错误重试）
shouldRetry := common.ShouldRetryForLimitError(classifier)

// 4. 使用 RetryWithBackoff 执行 API 调用
cfg := common.DefaultRetryConfig()
err := common.RetryWithBackoff(context.Background(), cfg, func() error {
    return someAPI()
}, shouldRetry)

// 5. 处理结果
if err != nil {
    // 处理错误
}
```

### 错误分类使用

```go
// 1. 导入必要的包
import "multicloud-exporter/internal/providers/common"

// 2. 使用错误分类器分类错误
err := someAPI()
status := common.ClassifyAliyunError(err)

// 3. 根据错误类型处理
switch status {
case common.ErrorStatusAuth:
    // 处理认证错误
case common.ErrorStatusLimit:
    // 处理限流错误
case common.ErrorStatusRegion:
    // 处理区域错误
case common.ErrorStatusNetwork:
    // 处理网络错误
default:
    // 处理未知错误
}
```

### 监控指标查询

```promql
# 查询阿里云 DescribeMetricList API 的调用成功率
rate(multicloud_request_total{cloud_provider="aliyun", api="DescribeMetricList", status="success"}[5m]) 
/ ignoring(status) 
rate(multicloud_request_total{cloud_provider="aliyun", api="DescribeMetricList"}[5m])

# 查询所有云厂商的限流率
rate(multicloud_rate_limit_total[5m]) / rate(multicloud_request_total[5m])
```

## 参考资源

- [监控指标使用指南](./metrics-guide.md)
- [重试机制实现](../internal/providers/common/retry.go)
- [错误分类实现](../internal/providers/common/errors.go)

## 注意事项

1. **认证错误处理**：认证错误需要人工介入，检查访问密钥是否有效
2. **限流处理**：限流错误会自动重试，但如果持续出现，建议降低并发配置或延长采集间隔
3. **区域错误处理**：区域错误会跳过该区域，建议检查账号的区域权限配置
4. **网络错误处理**：网络错误会自动重试，但如果持续出现，建议检查网络连接
5. **未知错误处理**：未知错误会记录到日志，建议定期检查日志并优化错误分类器
