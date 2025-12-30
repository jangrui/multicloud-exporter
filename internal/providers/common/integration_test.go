package common

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestErrorClassification_Integration 测试错误分类的集成场景
func TestErrorClassification_Integration(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		classifier ErrorClassifier
		expected   string
	}{
		{
			name:       "Aliyun auth error integration",
			err:        errors.New("InvalidAccessKeyId: The Access Key Id provided does not exist"),
			classifier: AliyunClassifier,
			expected:   ErrorStatusAuth,
		},
		{
			name:       "Aliyun rate limit integration",
			err:        errors.New("Throttling: Request was denied due to request throttling"),
			classifier: AliyunClassifier,
			expected:   ErrorStatusLimit,
		},
		{
			name:       "Tencent auth error integration",
			err:        errors.New("AuthFailure: Invalid credential"),
			classifier: TencentClassifier,
			expected:   ErrorStatusAuth,
		},
		{
			name:       "Tencent rate limit integration",
			err:        errors.New("RequestLimitExceeded: API request rate limit exceeded"),
			classifier: TencentClassifier,
			expected:   ErrorStatusLimit,
		},
		{
			name:       "AWS auth error integration",
			err:        errors.New("ExpiredToken: The security token included in the request is expired"),
			classifier: AWSClassifier,
			expected:   ErrorStatusAuth,
		},
		{
			name:       "AWS rate limit integration",
			err:        errors.New("Throttling: Rate exceeded"),
			classifier: AWSClassifier,
			expected:   ErrorStatusLimit,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.classifier.Classify(tt.err)
			if result != tt.expected {
				t.Errorf("Classify(%q) = %q, want %q", tt.err.Error(), result, tt.expected)
			}
		})
	}
}

// TestRetryWithBackoff_Integration 测试重试逻辑的集成场景
func TestRetryWithBackoff_Integration(t *testing.T) {
	t.Run("retry on rate limit then succeed", func(t *testing.T) {
		attempts := 0
		fn := func() error {
			attempts++
			if attempts < 3 {
				return errors.New("Throttling")
			}
			return nil
		}

		classifier := &AliyunErrorClassifier{}
		shouldRetry := ShouldRetryForLimitError(classifier)

		cfg := DefaultRetryConfig()
		cfg.MaxAttempts = 5
		cfg.InitialDelay = 10 * time.Millisecond // 缩短延迟以便快速测试

		err := RetryWithBackoff(context.Background(), cfg, fn, shouldRetry)
		if err != nil {
			t.Errorf("RetryWithBackoff should succeed after retries, got error: %v", err)
		}
		if attempts != 3 {
			t.Errorf("Expected 3 attempts, got %d", attempts)
		}
	})

	t.Run("retry on network error then succeed", func(t *testing.T) {
		attempts := 0
		fn := func() error {
			attempts++
			if attempts < 2 {
				return errors.New("timeout")
			}
			return nil
		}

		classifier := &AliyunErrorClassifier{}
		shouldRetry := ShouldRetryForNetworkError(classifier)

		cfg := DefaultRetryConfig()
		cfg.MaxAttempts = 5
		cfg.InitialDelay = 10 * time.Millisecond

		err := RetryWithBackoff(context.Background(), cfg, fn, shouldRetry)
		if err != nil {
			t.Errorf("RetryWithBackoff should succeed after retries, got error: %v", err)
		}
		if attempts != 2 {
			t.Errorf("Expected 2 attempts, got %d", attempts)
		}
	})

	t.Run("no retry on auth error", func(t *testing.T) {
		attempts := 0
		fn := func() error {
			attempts++
			return errors.New("InvalidAccessKeyId")
		}

		classifier := &AliyunErrorClassifier{}
		shouldRetry := ShouldRetryForLimitError(classifier)

		cfg := DefaultRetryConfig()
		cfg.MaxAttempts = 5
		cfg.InitialDelay = 10 * time.Millisecond

		err := RetryWithBackoff(context.Background(), cfg, fn, shouldRetry)
		if err == nil {
			t.Error("RetryWithBackoff should return error for auth error")
		}
		if attempts != 1 {
			t.Errorf("Expected 1 attempt (no retry for auth error), got %d", attempts)
		}
	})

	t.Run("exponential backoff timing", func(t *testing.T) {
		attempts := 0
		delays := []time.Duration{}
		start := time.Now()

		fn := func() error {
			attempts++
			if attempts > 1 {
				delays = append(delays, time.Since(start))
			}
			if attempts < 3 {
				return errors.New("Throttling")
			}
			return nil
		}

		classifier := &AliyunErrorClassifier{}
		shouldRetry := ShouldRetryForLimitError(classifier)

		cfg := DefaultRetryConfig()
		cfg.MaxAttempts = 5
		cfg.InitialDelay = 50 * time.Millisecond
		cfg.BackoffFactor = 2.0

		err := RetryWithBackoff(context.Background(), cfg, fn, shouldRetry)
		if err != nil {
			t.Errorf("RetryWithBackoff should succeed, got error: %v", err)
		}

		// 验证指数退避：第二次重试应该在 ~50ms 后，第三次应该在 ~100ms 后
		if len(delays) >= 1 {
			// 允许一些误差（±20ms）
			expectedDelay1 := 50 * time.Millisecond
			if delays[0] < expectedDelay1-20*time.Millisecond || delays[0] > expectedDelay1+20*time.Millisecond {
				t.Errorf("First retry delay should be around %v, got %v", expectedDelay1, delays[0])
			}
		}
		if len(delays) >= 2 {
			// 第二次重试应该在第一次延迟后约 100ms（50ms * 2）
			expectedDelay2 := 150 * time.Millisecond // 50ms (first) + 100ms (second)
			if delays[1] < expectedDelay2-30*time.Millisecond || delays[1] > expectedDelay2+30*time.Millisecond {
				t.Errorf("Second retry delay should be around %v, got %v", expectedDelay2, delays[1])
			}
		}
	})
}

// TestErrorHandling_EdgeCases 测试边缘情况
func TestErrorHandling_EdgeCases(t *testing.T) {
	t.Run("nil error", func(t *testing.T) {
		classifier := &AliyunErrorClassifier{}
		result := classifier.Classify(nil)
		if result != ErrorStatusUnknown {
			t.Errorf("Classify(nil) = %q, want %q", result, ErrorStatusUnknown)
		}
	})

	t.Run("empty error message", func(t *testing.T) {
		classifier := &AliyunErrorClassifier{}
		result := classifier.Classify(errors.New(""))
		if result != ErrorStatusUnknown {
			t.Errorf("Classify(empty) = %q, want %q", result, ErrorStatusUnknown)
		}
	})

	t.Run("multiple error keywords", func(t *testing.T) {
		// 如果错误消息包含多个关键词，应该优先匹配更具体的错误类型
		classifier := &AliyunErrorClassifier{}
		err := errors.New("InvalidAccessKeyId and Throttling")
		result := classifier.Classify(err)
		// 由于代码中先检查 auth，所以应该返回 auth_error
		if result != ErrorStatusAuth {
			t.Errorf("Classify(multiple keywords) = %q, want %q", result, ErrorStatusAuth)
		}
	})
}
