package utils

import (
	"context"
	"fmt"
	"time"
)

// RetryConfig 配置重试行为
type RetryConfig struct {
	MaxAttempts int           // 最大尝试次数（包括首次）
	WaitTime    time.Duration // 重试之间的等待时间
	MaxWait     time.Duration // 最大等待时间
	Multiplier  float64       // 指数退避的倍数（默认 2.0）
}

// DefaultRetryConfig 返回用于重试瞬态故障的合理默认值
func DefaultRetryConfig() *RetryConfig {
	return &RetryConfig{
		MaxAttempts: 3,
		WaitTime:    1 * time.Second,
		MaxWait:     30 * time.Second,
		Multiplier:  2.0,
	}
}

// IsTransientError 检查错误是否为瞬态错误（应该重试）
// 此函数应根据云 SDK 已知的瞬态错误进行定制
func IsTransientError(err error) bool {
	if err == nil {
		return false
	}

	// 添加特定的瞬态错误类型
	// 例如：限流错误、超时错误、临时网络故障
	errMsg := err.Error()

	// 常见的瞬态错误模式
	transientPatterns := []string{
		"rate limit",
		"timeout",
		"temporary",
		"connection reset",
		"connection refused",
		"503", // Service Unavailable
		"502", // Bad Gateway
		"504", // Gateway Timeout
		"429", // Too Many Requests
	}

	for _, pattern := range transientPatterns {
		if contains(errMsg, pattern) {
			return true
		}
	}

	return false
}

// RetryWithContext 使用指数退避重试函数
// 如果上下文被取消则停止重试
func RetryWithContext(ctx context.Context, config *RetryConfig, fn func() error) error {
	if config == nil {
		config = DefaultRetryConfig()
	}

	var lastErr error
	wait := config.WaitTime

	for attempt := 0; attempt < config.MaxAttempts; attempt++ {
		// 在尝试前检查上下文
		if ctx.Err() != nil {
			return fmt.Errorf("上下文在第 %d 次尝试前被取消: %w", attempt+1, ctx.Err())
		}

		lastErr = fn()
		if lastErr == nil {
			return nil
		}

		// 如果是最后一次尝试或错误不是瞬态的，则不重试
		if attempt == config.MaxAttempts-1 || !IsTransientError(lastErr) {
			return lastErr
		}

		// 重试前等待
		select {
		case <-ctx.Done():
			return fmt.Errorf("上下文在第 %d 次尝试后的重试等待期间被取消: %w", attempt+1, ctx.Err())
		case <-time.After(wait):
			// 继续下一次尝试
		}

		// 使用指数退避计算下一次等待时间
		wait = time.Duration(float64(wait) * config.Multiplier)
		if wait > config.MaxWait {
			wait = config.MaxWait
		}
	}

	return lastErr
}

// Retry 便捷函数，使用后台上下文
func Retry(config *RetryConfig, fn func() error) error {
	return RetryWithContext(context.Background(), config, fn)
}

// contains 检查字符串是否包含子字符串（不区分大小写）
func contains(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr ||
			len(s) > len(substr) && (s[:len(substr)] == substr ||
				s[len(s)-len(substr):] == substr ||
				containsMiddle(s, substr)))
}

func containsMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
