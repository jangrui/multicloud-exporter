# 工具函数包

本包提供 multicloud-exporter 项目中使用的通用工具函数。

## HTTP 客户端

### `NewHTTPClient() *http.Client`
创建一个优化的 HTTP 客户端，专为云服务 API 调用提供合理的默认配置：

- **超时时间**：30 秒
- **连接池**：最大 100 个空闲连接，每个主机 10 个
- **HTTP/2**：启用
- **代理**：从环境变量读取（HTTP_PROXY、HTTPS_PROXY、NO_PROXY）
- **Keep-Alive**：90 秒空闲超时

示例：
```go
client := utils.NewHTTPClient()
resp, err := client.Get("https://api.example.com")
```

### `NewHTTPClientWithTimeout(timeout time.Duration) *http.Client`
创建一个自定义超时的 HTTP 客户端，同时保持其他优化配置。

## 错误处理

### `WrapError(err error, message string) error`
包装错误并添加上下文信息。如果 err 为 nil，则返回 nil。

示例：
```go
err := apiCall()
if err != nil {
    return utils.WrapError(err, "获取 S3 存储桶失败")
}
```

### `IsTransientError(err error) bool`
检查错误是否为瞬态错误（应该重试）。识别以下错误类型：
- 限流错误
- 超时错误
- 临时网络故障
- HTTP 502、503、504 错误
- HTTP 429（Too Many Requests）

## 重试机制

### `Retry(config *RetryConfig, fn func() error) error`
使用指数退避对瞬态错误进行重试。

### `RetryWithContext(ctx context.Context, config *RetryConfig, fn func() error) error`
与 Retry 相同，但支持上下文取消。

示例：
```go
config := &utils.RetryConfig{
    MaxAttempts: 3,
    WaitTime:    1 * time.Second,
    MaxWait:     30 * time.Second,
    Multiplier:  2.0,
}

err := utils.Retry(config, func() error {
    return fetchMetrics()
})
```

## 集群分片

### `ShardIndex(total, index int) int`
计算集群分片的索引。

### `ShouldProcess(totalShards, shardIndex, itemID int) bool`
确定某个项目是否应由当前分片处理。

这些工具函数有助于在集群的多个 exporter 实例之间分配工作负载。

## 配置解析

### `ParseDurationDays(s string) (time.Duration, error)`
解析支持 "d" 后缀（天）的持续时间字符串。

示例：
```go
duration, err := utils.ParseDurationDays("7d")  // 7 天
duration, err := utils.ParseDurationDays("24h") // 24 小时
```
