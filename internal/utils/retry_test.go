package utils

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestIsTransientError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error is not transient",
			err:      nil,
			expected: false,
		},
		{
			name:     "rate limit error is transient",
			err:      errors.New("rate limit exceeded"),
			expected: true,
		},
		{
			name:     "timeout error is transient",
			err:      errors.New("context timeout exceeded"),
			expected: true,
		},
		{
			name:     "temporary error is transient",
			err:      errors.New("temporary network failure"),
			expected: true,
		},
		{
			name:     "503 error is transient",
			err:      errors.New("HTTP 503 Service Unavailable"),
			expected: true,
		},
		{
			name:     "429 error is transient",
			err:      errors.New("HTTP 429 Too Many Requests"),
			expected: true,
		},
		{
			name:     "permanent error is not transient",
			err:      errors.New("authentication failed"),
			expected: false,
		},
		{
			name:     "validation error is not transient",
			err:      errors.New("invalid input"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsTransientError(tt.err)
			if got != tt.expected {
				t.Errorf("IsTransientError() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestRetry_Success(t *testing.T) {
	attempts := 0
	fn := func() error {
		attempts++
		if attempts < 3 {
			return errors.New("rate limit exceeded")
		}
		return nil
	}

	config := &RetryConfig{
		MaxAttempts: 5,
		WaitTime:    10 * time.Millisecond,
		Multiplier:  1.5,
	}

	err := Retry(config, fn)
	if err != nil {
		t.Errorf("Retry() should succeed, got error: %v", err)
	}

	if attempts != 3 {
		t.Errorf("Expected 3 attempts, got %d", attempts)
	}
}

func TestRetry_AllAttemptsFailed(t *testing.T) {
	config := &RetryConfig{
		MaxAttempts: 3,
		WaitTime:    10 * time.Millisecond,
	}

	attempts := 0
	fn := func() error {
		attempts++
		return errors.New("rate limit exceeded")
	}

	err := Retry(config, fn)
	if err == nil {
		t.Error("Retry() should return error when all attempts fail")
	}

	if attempts != 3 {
		t.Errorf("Expected 3 attempts, got %d", attempts)
	}
}

func TestRetry_NonTransientError(t *testing.T) {
	config := &RetryConfig{
		MaxAttempts: 5,
		WaitTime:    10 * time.Millisecond,
	}

	attempts := 0
	fn := func() error {
		attempts++
		return errors.New("authentication failed")
	}

	err := Retry(config, fn)
	if err == nil {
		t.Error("Retry() should return error for non-transient error")
	}

	if attempts != 1 {
		t.Errorf("Non-transient error should not retry, got %d attempts", attempts)
	}
}

func TestRetryWithContext_Cancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	config := &RetryConfig{
		MaxAttempts: 10,
		WaitTime:    100 * time.Millisecond,
	}

	attempts := 0
	fn := func() error {
		attempts++
		// Cancel after first attempt
		if attempts == 1 {
			cancel()
		}
		return errors.New("rate limit exceeded")
	}

	err := RetryWithContext(ctx, config, fn)
	if err == nil {
		t.Error("RetryWithContext() should return error when context is cancelled")
	}

	if attempts > 2 {
		t.Errorf("Should stop quickly after context cancellation, got %d attempts", attempts)
	}
}

func TestRetry_ExponentialBackoff(t *testing.T) {
	config := &RetryConfig{
		MaxAttempts: 4,
		WaitTime:    50 * time.Millisecond,
		MaxWait:     200 * time.Millisecond,
		Multiplier:  2.0,
	}

	attempts := 0
	fn := func() error {
		attempts++
		return errors.New("temporary timeout error - always fails")
	}

	// We can't easily measure actual wait times, but we can verify it doesn't hang
	start := time.Now()
	_ = Retry(config, fn)
	elapsed := time.Since(start)

	// With exponential backoff: 50ms + 100ms + 200ms = 350ms minimum
	// Should be at least 300ms (allowing some variance)
	if elapsed < 300*time.Millisecond {
		t.Errorf("Exponential backoff didn't wait long enough: %v", elapsed)
	}

	if attempts != 4 {
		t.Errorf("Expected 4 attempts, got %d", attempts)
	}
}

func TestDefaultRetryConfig(t *testing.T) {
	config := DefaultRetryConfig()

	if config.MaxAttempts != 3 {
		t.Errorf("Default MaxAttempts = %d, want 3", config.MaxAttempts)
	}
	if config.WaitTime != 1*time.Second {
		t.Errorf("Default WaitTime = %v, want 1s", config.WaitTime)
	}
	if config.MaxWait != 30*time.Second {
		t.Errorf("Default MaxWait = %v, want 30s", config.MaxWait)
	}
	if config.Multiplier != 2.0 {
		t.Errorf("Default Multiplier = %f, want 2.0", config.Multiplier)
	}
}

func TestRetry_NilConfig(t *testing.T) {
	attempts := 0
	fn := func() error {
		attempts++
		if attempts < 3 {
			return errors.New("temporary error")
		}
		return nil
	}

	err := Retry(nil, fn)
	if err != nil {
		t.Errorf("Retry with nil config should use defaults, got error: %v", err)
	}
}
