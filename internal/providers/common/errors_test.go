package common

import (
	"errors"
	"testing"
)

func TestAliyunErrorClassifier(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{"auth error - InvalidAccessKeyId", errors.New("InvalidAccessKeyId"), ErrorStatusAuth},
		{"auth error - Forbidden", errors.New("Forbidden"), ErrorStatusAuth},
		{"auth error - SignatureDoesNotMatch", errors.New("SignatureDoesNotMatch"), ErrorStatusAuth},
		{"limit error - Throttling", errors.New("Throttling"), ErrorStatusLimit},
		{"limit error - flow control", errors.New("flow control"), ErrorStatusLimit},
		{"region skip - InvalidRegionId", errors.New("InvalidRegionId"), ErrorStatusRegion},
		{"region skip - Unsupported", errors.New("Unsupported"), ErrorStatusRegion},
		{"network error - timeout", errors.New("timeout"), ErrorStatusNetwork},
		{"network error - unreachable", errors.New("unreachable"), ErrorStatusNetwork},
		{"network error - Temporary network", errors.New("Temporary network"), ErrorStatusNetwork},
		{"unknown error", errors.New("other error"), ErrorStatusUnknown},
	}

	classifier := &AliyunErrorClassifier{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifier.Classify(tt.err)
			if result != tt.expected {
				t.Errorf("ClassifyAliyunError(%q) = %q, want %q", tt.err.Error(), result, tt.expected)
			}
		})
	}
}

func TestTencentErrorClassifier(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{"auth error - AuthFailure", errors.New("AuthFailure"), ErrorStatusAuth},
		{"auth error - InvalidCredential", errors.New("InvalidCredential"), ErrorStatusAuth},
		{"limit error - RequestLimitExceeded", errors.New("RequestLimitExceeded"), ErrorStatusLimit},
		{"network error - timeout", errors.New("timeout"), ErrorStatusNetwork},
		{"network error - network", errors.New("network"), ErrorStatusNetwork},
		{"unknown error", errors.New("other error"), ErrorStatusUnknown},
	}

	classifier := &TencentErrorClassifier{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifier.Classify(tt.err)
			if result != tt.expected {
				t.Errorf("ClassifyTencentError(%q) = %q, want %q", tt.err.Error(), result, tt.expected)
			}
		})
	}
}

func TestAWSErrorClassifier(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{"auth error - ExpiredToken", errors.New("ExpiredToken"), ErrorStatusAuth},
		{"auth error - InvalidClientTokenId", errors.New("InvalidClientTokenId"), ErrorStatusAuth},
		{"auth error - AccessDenied", errors.New("AccessDenied"), ErrorStatusAuth},
		{"limit error - Throttling", errors.New("Throttling"), ErrorStatusLimit},
		{"limit error - Rate exceeded", errors.New("Rate exceeded"), ErrorStatusLimit},
		{"limit error - TooManyRequests", errors.New("TooManyRequests"), ErrorStatusLimit},
		{"network error - timeout", errors.New("timeout"), ErrorStatusNetwork},
		{"network error - network", errors.New("network"), ErrorStatusNetwork},
		{"unknown error", errors.New("other error"), ErrorStatusUnknown},
	}

	classifier := &AWSErrorClassifier{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifier.Classify(tt.err)
			if result != tt.expected {
				t.Errorf("ClassifyAWSError(%q) = %q, want %q", tt.err.Error(), result, tt.expected)
			}
		})
	}
}

func TestCompatibilityFunctions(t *testing.T) {
	// 测试兼容函数
	err := errors.New("Throttling")
	
	if ClassifyAliyunError(err) != ErrorStatusLimit {
		t.Errorf("ClassifyAliyunError should return limit_error for Throttling")
	}
	
	if ClassifyTencentError(errors.New("RequestLimitExceeded")) != ErrorStatusLimit {
		t.Errorf("ClassifyTencentError should return limit_error for RequestLimitExceeded")
	}
	
	if ClassifyAWSError(errors.New("Throttling")) != ErrorStatusLimit {
		t.Errorf("ClassifyAWSError should return limit_error for Throttling")
	}
}

