package common

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDefaultRetryConfig(t *testing.T) {
	cfg := DefaultRetryConfig()
	if cfg.MaxAttempts != 5 {
		t.Errorf("Default MaxAttempts = %d, want 5", cfg.MaxAttempts)
	}
	if cfg.InitialDelay != 200*time.Millisecond {
		t.Errorf("Default InitialDelay = %v, want 200ms", cfg.InitialDelay)
	}
	if cfg.MaxDelay != 5*time.Second {
		t.Errorf("Default MaxDelay = %v, want 5s", cfg.MaxDelay)
	}
	if cfg.BackoffFactor != 2.0 {
		t.Errorf("Default BackoffFactor = %f, want 2.0", cfg.BackoffFactor)
	}
}

func TestRetryWithBackoff_Success(t *testing.T) {
	attempts := 0
	fn := func() error {
		attempts++
		if attempts < 2 {
			return errors.New("temporary error")
		}
		return nil
	}

	shouldRetry := func(err error) bool {
		return err != nil
	}

	cfg := DefaultRetryConfig()
	cfg.MaxAttempts = 3
	err := RetryWithBackoff(context.Background(), cfg, fn, shouldRetry)

	if err != nil {
		t.Errorf("RetryWithBackoff should succeed, got error: %v", err)
	}
	if attempts != 2 {
		t.Errorf("Expected 2 attempts, got %d", attempts)
	}
}

func TestRetryWithBackoff_MaxAttempts(t *testing.T) {
	attempts := 0
	fn := func() error {
		attempts++
		return errors.New("persistent error")
	}

	shouldRetry := func(err error) bool {
		return true
	}

	cfg := DefaultRetryConfig()
	cfg.MaxAttempts = 3
	err := RetryWithBackoff(context.Background(), cfg, fn, shouldRetry)

	if err == nil {
		t.Error("RetryWithBackoff should return error after max attempts")
	}
	if attempts != 4 { // MaxAttempts + 1 (initial attempt)
		t.Errorf("Expected 4 attempts, got %d", attempts)
	}
}

func TestRetryWithBackoff_NoRetry(t *testing.T) {
	attempts := 0
	fn := func() error {
		attempts++
		return errors.New("auth error")
	}

	classifier := &AliyunErrorClassifier{}
	shouldRetry := ShouldRetryForLimitError(classifier)

	cfg := DefaultRetryConfig()
	cfg.MaxAttempts = 5
	err := RetryWithBackoff(context.Background(), cfg, fn, shouldRetry)

	if err == nil {
		t.Error("RetryWithBackoff should return error")
	}
	// 对于认证错误，不应该重试，只尝试一次
	if attempts != 1 {
		t.Errorf("Expected 1 attempt (no retry for auth error), got %d", attempts)
	}
}

func TestRetryWithBackoff_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	fn := func() error {
		return errors.New("error")
	}

	shouldRetry := func(err error) bool {
		return true
	}

	cfg := DefaultRetryConfig()
	err := RetryWithBackoff(ctx, cfg, fn, shouldRetry)

	if err != context.Canceled {
		t.Errorf("RetryWithBackoff should return context.Canceled, got %v", err)
	}
}

func TestShouldRetryForLimitError(t *testing.T) {
	classifier := &AliyunErrorClassifier{}
	shouldRetry := ShouldRetryForLimitError(classifier)

	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"limit error should retry", errors.New("Throttling"), true},
		{"network error should retry", errors.New("timeout"), true},
		{"auth error should not retry", errors.New("InvalidAccessKeyId"), false},
		{"region skip should not retry", errors.New("InvalidRegionId"), false},
		{"unknown error should not retry", errors.New("other"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shouldRetry(tt.err)
			if result != tt.expected {
				t.Errorf("shouldRetry(%q) = %v, want %v", tt.err.Error(), result, tt.expected)
			}
		})
	}
}

func TestShouldRetryForNetworkError(t *testing.T) {
	classifier := &AliyunErrorClassifier{}
	shouldRetry := ShouldRetryForNetworkError(classifier)

	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"network error should retry", errors.New("timeout"), true},
		{"limit error should not retry", errors.New("Throttling"), false},
		{"auth error should not retry", errors.New("InvalidAccessKeyId"), false},
		{"region skip should not retry", errors.New("InvalidRegionId"), false},
		{"unknown error should not retry", errors.New("other"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shouldRetry(tt.err)
			if result != tt.expected {
				t.Errorf("shouldRetry(%q) = %v, want %v", tt.err.Error(), result, tt.expected)
			}
		})
	}
}

func TestPow(t *testing.T) {
	tests := []struct {
		x, y, expected float64
	}{
		{2.0, 0, 1.0},
		{2.0, 1, 2.0},
		{2.0, 2, 4.0},
		{2.0, 3, 8.0},
		{2.0, 4, 16.0},
	}

	for _, tt := range tests {
		result := pow(tt.x, tt.y)
		if result != tt.expected {
			t.Errorf("pow(%f, %f) = %f, want %f", tt.x, tt.y, result, tt.expected)
		}
	}
}
