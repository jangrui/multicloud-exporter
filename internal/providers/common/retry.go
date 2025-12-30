// Package common 提供云厂商通用的错误处理和重试逻辑
package common

import (
	"context"
	"time"
)

// RetryConfig 定义重试配置
type RetryConfig struct {
	// MaxAttempts 最大重试次数（不包括首次尝试），默认 5
	MaxAttempts int
	// InitialDelay 初始延迟时间，默认 200ms
	InitialDelay time.Duration
	// MaxDelay 最大延迟时间，默认 5s
	MaxDelay time.Duration
	// BackoffFactor 退避因子，默认 2.0（指数退避）
	BackoffFactor float64
}

// DefaultRetryConfig 返回默认重试配置（符合项目规范）
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:   5,
		InitialDelay:  200 * time.Millisecond,
		MaxDelay:      5 * time.Second,
		BackoffFactor: 2.0,
	}
}

// RetryWithBackoff 使用指数退避策略执行重试
// 该函数实现了符合项目规范的重试逻辑：对于认证错误和区域跳过错误，不重试直接返回。
//
// 参数说明：
//   - ctx: 上下文，用于支持取消操作
//   - cfg: 重试配置，如果配置值无效，将使用默认值
//   - fn: 要执行的函数，返回 error。如果 error 为 nil，则成功，不再重试
//   - shouldRetry: 用于判断错误是否应该重试的函数。如果返回 false，则不再重试
//
// 返回值：
//   - 如果成功，返回 nil
//   - 如果达到最大重试次数仍失败，返回最后一次的错误
//   - 如果上下文被取消，返回 context.Canceled 或 context.DeadlineExceeded
//
// 示例：
//
//	cfg := common.DefaultRetryConfig()
//	classifier := &common.AliyunErrorClassifier{}
//	shouldRetry := common.ShouldRetryForLimitError(classifier)
//	err := common.RetryWithBackoff(ctx, cfg, func() error {
//	    return someAPI()
//	}, shouldRetry)
func RetryWithBackoff(ctx context.Context, cfg RetryConfig, fn func() error, shouldRetry func(error) bool) error {
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 5
	}
	if cfg.InitialDelay <= 0 {
		cfg.InitialDelay = 200 * time.Millisecond
	}
	if cfg.MaxDelay <= 0 {
		cfg.MaxDelay = 5 * time.Second
	}
	if cfg.BackoffFactor <= 0 {
		cfg.BackoffFactor = 2.0
	}

	var lastErr error
	for attempt := 0; attempt <= cfg.MaxAttempts; attempt++ {
		// 检查上下文是否已取消
		if ctx != nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
		}

		err := fn()
		if err == nil {
			return nil
		}

		lastErr = err

		// 判断是否应该重试
		if !shouldRetry(err) {
			return err
		}

		// 如果已经是最后一次尝试，不再等待
		if attempt >= cfg.MaxAttempts {
			break
		}

		// 计算延迟时间（指数退避）
		delay := time.Duration(float64(cfg.InitialDelay) * pow(cfg.BackoffFactor, float64(attempt)))
		if delay > cfg.MaxDelay {
			delay = cfg.MaxDelay
		}

		// 等待延迟时间
		if ctx != nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		} else {
			time.Sleep(delay)
		}
	}

	return lastErr
}

// pow 计算 x 的 y 次方（简单实现，避免引入 math 包）
func pow(x, y float64) float64 {
	result := 1.0
	for i := 0; i < int(y); i++ {
		result *= x
	}
	return result
}

// ShouldRetryForLimitError 返回一个函数，用于判断是否应该重试（仅对限流和网络错误重试）
// 对于认证错误和区域跳过错误，不重试
//
// 参数：
//   - classifier: 错误分类器，用于将错误分类为统一的错误状态码
//
// 返回值：
//
//	返回一个函数，该函数接受 error 并返回 bool，表示是否应该重试
//
// 示例：
//
//	classifier := &common.AliyunErrorClassifier{}
//	shouldRetry := common.ShouldRetryForLimitError(classifier)
//	if shouldRetry(err) {
//	    // 应该重试
//	}
func ShouldRetryForLimitError(classifier ErrorClassifier) func(error) bool {
	return func(err error) bool {
		status := classifier.Classify(err)
		// 对于认证错误和区域跳过错误，不重试
		if status == ErrorStatusAuth || status == ErrorStatusRegion {
			return false
		}
		// 对于限流和网络错误，重试
		return status == ErrorStatusLimit || status == ErrorStatusNetwork
	}
}

// ShouldRetryForNetworkError 返回一个函数，用于判断是否应该重试（仅对网络错误重试）
// 对于认证错误、限流错误和区域跳过错误，不重试
//
// 参数：
//   - classifier: 错误分类器，用于将错误分类为统一的错误状态码
//
// 返回值：
//
//	返回一个函数，该函数接受 error 并返回 bool，表示是否应该重试
//
// 示例：
//
//	classifier := &common.AliyunErrorClassifier{}
//	shouldRetry := common.ShouldRetryForNetworkError(classifier)
//	if shouldRetry(err) {
//	    // 应该重试
//	}
func ShouldRetryForNetworkError(classifier ErrorClassifier) func(error) bool {
	return func(err error) bool {
		status := classifier.Classify(err)
		// 仅对网络错误重试
		return status == ErrorStatusNetwork
	}
}
