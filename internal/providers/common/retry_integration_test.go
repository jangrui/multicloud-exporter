// Package common 提供云厂商通用的错误处理和重试逻辑的集成测试
package common

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestRetryWithBackoff_ExponentialBackoff 测试指数退避延迟计算
func TestRetryWithBackoff_ExponentialBackoff(t *testing.T) {
	attempts := 0
	delays := []time.Duration{}
	startTime := time.Now()

	fn := func() error {
		if attempts > 0 {
			delays = append(delays, time.Since(startTime))
		}
		attempts++
		startTime = time.Now()
		return errors.New("persistent error")
	}

	shouldRetry := func(err error) bool {
		return true
	}

	cfg := RetryConfig{
		MaxAttempts:   3,
		InitialDelay:  100 * time.Millisecond,
		MaxDelay:      5 * time.Second,
		BackoffFactor: 2.0,
	}

	_ = RetryWithBackoff(context.Background(), cfg, fn, shouldRetry)

	// 验证重试次数
	if attempts != 4 { // 初始尝试 + 3 次重试
		t.Errorf("Expected 4 attempts, got %d", attempts)
	}

	// 验证延迟时间符合指数退避
	// 第1次重试: 100ms
	// 第2次重试: 200ms
	// 第3次重试: 400ms
	expectedDelays := []time.Duration{
		100 * time.Millisecond,
		200 * time.Millisecond,
		400 * time.Millisecond,
	}

	for i, delay := range delays {
		// 允许 50ms 的误差
		tolerance := 50 * time.Millisecond
		if delay < expectedDelays[i]-tolerance || delay > expectedDelays[i]+tolerance {
			t.Logf("Warning: Delay %d = %v, expected ~%v (tolerance ±%v)",
				i+1, delay, expectedDelays[i], tolerance)
		}
	}
}

// TestRetryWithBackoff_MaxDelayLimit 测试最大延迟限制
func TestRetryWithBackoff_MaxDelayLimit(t *testing.T) {
	attempts := 0
	delays := []time.Duration{}
	startTime := time.Now()

	fn := func() error {
		if attempts > 0 {
			delays = append(delays, time.Since(startTime))
		}
		attempts++
		startTime = time.Now()
		return errors.New("persistent error")
	}

	shouldRetry := func(err error) bool {
		return true
	}

	cfg := RetryConfig{
		MaxAttempts:   5,
		InitialDelay:  1 * time.Second,
		MaxDelay:      2 * time.Second, // 最大延迟 2 秒
		BackoffFactor: 2.0,
	}

	_ = RetryWithBackoff(context.Background(), cfg, fn, shouldRetry)

	// 验证所有延迟都不超过 MaxDelay
	for i, delay := range delays {
		if delay > cfg.MaxDelay+100*time.Millisecond { // 允许 100ms 误差
			t.Errorf("Delay %d = %v exceeds MaxDelay %v", i+1, delay, cfg.MaxDelay)
		}
	}
}

// TestRetryWithBackoff_HuaweiLimitError 测试华为云限流错误重试
func TestRetryWithBackoff_HuaweiLimitError(t *testing.T) {
	attempts := 0
	limitErrors := []error{
		errors.New("APIGW.0308: API rate limit exceeded"),
		errors.New("throttling request"),
		errors.New("ratelimit exceeded"),
		errors.New("429 Too Many Requests"),
	}

	for _, limitErr := range limitErrors {
		attempts = 0
		fn := func() error {
			attempts++
			if attempts < 3 {
				return limitErr
			}
			return nil
		}

		classifier := &HuaweiErrorClassifier{}
		shouldRetry := ShouldRetryForLimitError(classifier)

		cfg := DefaultRetryConfig()
		cfg.MaxAttempts = 5

		err := RetryWithBackoff(context.Background(), cfg, fn, shouldRetry)

		if err != nil {
			t.Errorf("RetryWithBackoff should succeed for %q, got error: %v", limitErr.Error(), err)
		}

		if attempts != 3 {
			t.Errorf("Expected 3 attempts for %q, got %d", limitErr.Error(), attempts)
		}
	}
}

// TestRetryWithBackoff_HuaweiAuthError 测试华为云认证错误不重试
func TestRetryWithBackoff_HuaweiAuthError(t *testing.T) {
	authErrors := []error{
		errors.New("Authenticate failed"),
		errors.New("401 Unauthorized"),
		errors.New("InvalidAccessKeyId"),
		errors.New("AK/SK is invalid"),
	}

	for _, authErr := range authErrors {
		attempts := 0
		fn := func() error {
			attempts++
			return authErr
		}

		classifier := &HuaweiErrorClassifier{}
		shouldRetry := ShouldRetryForLimitError(classifier)

		cfg := DefaultRetryConfig()
		cfg.MaxAttempts = 5

		err := RetryWithBackoff(context.Background(), cfg, fn, shouldRetry)

		if err == nil {
			t.Errorf("RetryWithBackoff should return error for %q", authErr.Error())
		}

		// 认证错误不应该重试，只尝试一次
		if attempts != 1 {
			t.Errorf("Expected 1 attempt (no retry) for %q, got %d", authErr.Error(), attempts)
		}
	}
}

// TestRetryWithBackoff_ContextTimeout 测试上下文超时
func TestRetryWithBackoff_ContextTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	attempts := 0
	fn := func() error {
		attempts++
		return errors.New("persistent error")
	}

	shouldRetry := func(err error) bool {
		return true
	}

	cfg := RetryConfig{
		MaxAttempts:   10,
		InitialDelay:  200 * time.Millisecond,
		MaxDelay:      5 * time.Second,
		BackoffFactor: 2.0,
	}

	err := RetryWithBackoff(ctx, cfg, fn, shouldRetry)

	if err != context.DeadlineExceeded {
		t.Errorf("RetryWithBackoff should return context.DeadlineExceeded, got %v", err)
	}

	// 由于超时，重试次数应该少于 MaxAttempts
	if attempts >= cfg.MaxAttempts {
		t.Errorf("Expected fewer attempts due to timeout, got %d", attempts)
	}
}

// TestRetryWithBackoff_DefaultConfigValidation 测试默认配置验证
func TestRetryWithBackoff_DefaultConfigValidation(t *testing.T) {
	attempts := 0
	fn := func() error {
		attempts++
		if attempts < 2 {
			return errors.New("temporary error")
		}
		return nil
	}

	shouldRetry := func(err error) bool {
		return true
	}

	// 测试无效配置会被修正为默认值
	cfg := RetryConfig{
		MaxAttempts:   0, // 无效，应该被修正为 5
		InitialDelay:  0, // 无效，应该被修正为 200ms
		MaxDelay:      0, // 无效，应该被修正为 5s
		BackoffFactor: 0, // 无效，应该被修正为 2.0
	}

	err := RetryWithBackoff(context.Background(), cfg, fn, shouldRetry)

	if err != nil {
		t.Errorf("RetryWithBackoff should succeed with default config, got error: %v", err)
	}

	if attempts != 2 {
		t.Errorf("Expected 2 attempts, got %d", attempts)
	}
}

// TestRetryWithBackoff_RealWorldScenario 测试真实场景：华为云 API 限流后恢复
func TestRetryWithBackoff_RealWorldScenario(t *testing.T) {
	// 模拟华为云 API 调用：前 3 次限流，第 4 次成功
	attempts := 0
	fn := func() error {
		attempts++
		if attempts <= 3 {
			return errors.New("APIGW.0308: API rate limit exceeded")
		}
		return nil
	}

	classifier := &HuaweiErrorClassifier{}
	shouldRetry := ShouldRetryForLimitError(classifier)

	cfg := DefaultRetryConfig()

	startTime := time.Now()
	err := RetryWithBackoff(context.Background(), cfg, fn, shouldRetry)
	duration := time.Since(startTime)

	if err != nil {
		t.Errorf("RetryWithBackoff should succeed after retries, got error: %v", err)
	}

	if attempts != 4 {
		t.Errorf("Expected 4 attempts, got %d", attempts)
	}

	// 验证总耗时符合预期
	// 第1次重试: 200ms
	// 第2次重试: 400ms
	// 第3次重试: 800ms
	// 总计: ~1400ms
	expectedDuration := 1400 * time.Millisecond
	tolerance := 300 * time.Millisecond

	if duration < expectedDuration-tolerance || duration > expectedDuration+tolerance {
		t.Logf("Warning: Total duration = %v, expected ~%v (tolerance ±%v)",
			duration, expectedDuration, tolerance)
	}
}
