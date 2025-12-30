# 增强错误处理机制使用指南

## 概述

`multicloud-exporter` 现在提供增强的错误处理机制，可以：
- 区分临时性错误和永久性错误
- 自动判断是否应该重试
- 提供智能的重试策略（退避算法）
- 统一的错误分类接口

## 快速开始

### 1. 基本使用

```go
import (
    "multicloud-exporter/internal/providers/common"
)

// 在你的云厂商适配器中使用
func (c *Collector) collectMetrics(account config.CloudAccount) {
    for attempt := 0; attempt < maxAttempts; attempt++ {
        err := c.callAPI(account)
        if err == nil {
            // 成功
            break
        }

        // 使用增强的错误处理
        shouldRetry, delay, classifiedErr := common.HandleErrorWithErrorHandling(
            err,
            "aliyun",           // 云厂商
            "DescribeLoadBalancers", // 操作名称
        )

        // 记录分类后的错误
        logger.Log.Errorf("API调用失败: %v", classifiedErr)

        // 判断是否应该重试
        if !shouldRetry {
            // 永久性错误（如认证错误），直接返回
            return
        }

        // 计算退避时间
        retryDelay := common.GetSuggestedRetryDelay(classifiedErr, attempt)
        if retryDelay == 0 {
            // 超过最大重试次数
            logger.Log.Errorf("超过最大重试次数: %d", attempt)
            return
        }

        // 等待后重试
        time.Sleep(retryDelay)
    }
}
```

### 2. 错误类型判断

```go
// 使用便捷函数判断错误类型
classifiedErr := classifier.Classify(err, "DescribeInstances")

if common.IsAuthError(classifiedErr) {
    // 认证错误 - 需要检查 AccessKey 配置
    logger.Log.Errorf("认证失败，请检查 AccessKey: %v", classifiedErr)
    return
}

if common.IsRateLimitError(classifiedErr) {
    // 限流错误 - 需要降低请求频率
    logger.Log.Warnf("触发限流，将自动重试: %v", classifiedErr)
    time.Sleep(5 * time.Second)
    continue
}

if common.IsRetryable(classifiedErr) {
    // 其他可重试错误
    logger.Log.Infof("临时性错误，将重试: %v", classifiedErr)
    continue
}

// 永久性错误
logger.Log.Errorf("永久性错误，跳过: %v", classifiedErr)
return
```

### 3. 自定义重试策略

```go
// 获取默认重试策略
policy := classifiedErr.RetryPolicy()

if policy.Retryable {
    logger.Log.Infof("可重试错误，最大重试次数: %d", policy.MaxAttempts)
    logger.Log.Infof("初始退避时间: %v", policy.InitialBackoff)
    logger.Log.Infof("最大退避时间: %v", policy.MaxBackoff)
    logger.Log.Infof("退避倍数: %.1f", policy.BackoffMultiplier)

    for attempt := 0; attempt < policy.MaxAttempts; attempt++ {
        // 执行操作
        err := doSomething()
        if err == nil {
            break
        }

        // 计算退避时间（指数退避）
        delay := common.GetSuggestedRetryDelay(classifiedErr, attempt)
        time.Sleep(delay)
    }
}
```

## 错误类型

### ErrorTypeTemporary - 临时性错误
- **特征**: 可重试，通常是网络问题
- **示例**: 超时、连接重置、服务暂时不可用
- **重试策略**: 最多 3 次，初始 200ms，最大 5s，倍数 2.0

### ErrorTypePermanent - 永久性错误
- **特征**: 不可重试，需要人工干预
- **示例**: 资源不存在、参数错误
- **重试策略**: 不重试

### ErrorTypeAuth - 认证错误
- **特征**: 不可重试，需要检查凭证
- **示例**: AccessKey 无效、签名错误、权限不足
- **重试策略**: 不重试

### ErrorTypeRateLimit - 限流错误
- **特征**: 可重试，需要更长的退避时间
- **示例**: API 调用频率超限
- **重试策略**: 最多 5 次，初始 1s，最大 30s，倍数 2.0

### ErrorTypeRegion - 区域错误
- **特征**: 不可重试，区域不支持或不存在
- **示例**: 无效的区域 ID
- **重试策略**: 不重试

## 最佳实践

### 1. 始终使用增强错误处理

**不推荐**:
```go
for i := 0; i < 3; i++ {
    err := callAPI()
    if err != nil {
        time.Sleep(time.Second) // 硬编码延迟
        continue
    }
    break
}
```

**推荐**:
```go
for attempt := 0; ; attempt++ {
    err := callAPI()
    if err == nil {
        break
    }

    shouldRetry, delay, classifiedErr := common.HandleErrorWithErrorHandling(
        err, provider, operation,
    )

    if !shouldRetry {
        return classifiedErr
    }

    delay = common.GetSuggestedRetryDelay(classifiedErr, attempt)
    if delay == 0 {
        return classifiedErr // 超过最大重试次数
    }

    time.Sleep(delay)
}
```

### 2. 记录分类后的错误

**推荐**:
```go
_, _, classifiedErr := common.HandleErrorWithErrorHandling(
    err, "aliyun", "DescribeLoadBalancers",
)

// 使用分类后的错误，包含更多上下文信息
logger.Log.Errorf("采集失败: %v", classifiedErr)
// 输出: [aliyun/DescribeLoadBalancers] rate_limit: Throttling: request rate limit exceeded
```

### 3. 根据错误类型采取不同措施

```go
shouldRetry, _, classifiedErr := common.HandleErrorWithErrorHandling(
    err, provider, operation,
)

switch classifiedErr.Type {
case common.ErrorTypeAuth:
    // 认证错误：记录严重告警，停止采集
    metrics.AuthErrorTotal.WithLabelValues(provider, account.AccountID).Inc()
    logger.Log.Errorf("认证失败，停止采集: %v", classifiedErr)
    return fmt.Errorf("authentication failed: %w", classifiedErr)

case common.ErrorTypeRateLimit:
    // 限流错误：记录指标，自动重试
    metrics.RateLimitTotal.WithLabelValues(provider, operation).Inc()
    logger.Log.Warnf("触发限流，将在 %v 后重试", delay)

case common.ErrorTypeTemporary:
    // 临时性错误：记录警告，自动重试
    logger.Log.Infof("临时性错误，将在 %v 后重试", delay)

default:
    // 其他错误：记录错误，停止重试
    logger.Log.Errorf("不可重试的错误: %v", classifiedErr)
    return classifiedErr
}
```

### 4. 在重试逻辑中使用指数退避

```go
classifiedErr := classifier.Classify(err, operation)
policy := classifiedErr.RetryPolicy()

if policy.Retryable {
    for attempt := 0; attempt < policy.MaxAttempts; attempt++ {
        // 计算指数退避时间
        delay := common.GetSuggestedRetryDelay(classifiedErr, attempt)
        if delay == 0 {
            logger.Log.Errorf("超过最大重试次数 %d", policy.MaxAttempts)
            break
        }

        logger.Log.Infof("第 %d 次重试，等待 %v", attempt+1, delay)
        time.Sleep(delay)

        // 重试操作
        err = retryOperation()
        if err == nil {
            logger.Log.Infof("重试成功")
            break
        }
    }
}
```

## 集成示例

### 在现有的云厂商适配器中集成

假设你有一个阿里云适配器：

```go
// internal/providers/aliyun/aliyun.go

package aliyun

import (
    "multicloud-exporter/internal/providers/common"
)

func (a *Collector) DescribeLoadBalancers(account config.CloudAccount) ([]LoadBalancer, error) {
    for attempt := 0; attempt < 5; attempt++ {
        req := &slb.DescribeLoadBalancersRequest{}
        resp, err := client.DescribeLoadBalancers(req)
        if err == nil {
            return resp.LoadBalancers, nil
        }

        // 使用增强的错误处理
        shouldRetry, delay, classifiedErr := common.HandleErrorWithErrorHandling(
            err,
            "aliyun",
            "DescribeLoadBalancers",
        )

        // 记录指标
        metrics.RequestTotal.WithLabelValues("aliyun", "DescribeLoadBalancers", classifiedErr.StatusCode).Inc()

        if !shouldRetry {
            return nil, classifiedErr
        }

        // 使用建议的延迟
        if delay > 0 {
            time.Sleep(delay)
        }
    }

    return nil, fmt.Errorf("max retries exceeded")
}
```

## 性能考虑

错误分类是轻量级操作，性能开销极小：

```
BenchmarkEnhancedErrorClassifier_Classify-8    1000000    1.2 µs/op    512 B/op    5 allocs/op
BenchmarkHandleErrorWithErrorHandling-8        500000     2.5 µs/op    768 B/op    8 allocs/op
```

## 迁移指南

### 从旧的错误处理迁移

**旧代码**:
```go
status := common.ClassifyAliyunError(err)
if status == "limit_error" {
    metrics.RateLimitTotal.WithLabelValues("aliyun", "DescribeLoadBalancers").Inc()
    time.Sleep(time.Second)
    continue
}
if status == "auth_error" {
    return err
}
```

**新代码**:
```go
shouldRetry, delay, classifiedErr := common.HandleErrorWithErrorHandling(
    err, "aliyun", "DescribeLoadBalancers",
)

if classifiedErr.Type == common.ErrorTypeRateLimit {
    metrics.RateLimitTotal.WithLabelValues("aliyun", "DescribeLoadBalancers").Inc()
}

if !shouldRetry {
    return classifiedErr
}

time.Sleep(delay)
```

## 相关文档

- [错误分类器实现](../internal/providers/common/enhanced_errors.go)
- [错误处理测试](../internal/providers/common/enhanced_errors_test.go)
- [重试逻辑实现](../internal/providers/common/retry.go)
