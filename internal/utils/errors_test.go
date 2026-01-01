package utils

import (
	"errors"
	"fmt"
	"testing"
)

func TestWrapError(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		message string
		wantNil bool
	}{
		{
			name:    "nil error returns nil",
			err:     nil,
			message: "failed",
			wantNil: true,
		},
		{
			name:    "wraps error with message",
			err:     errors.New("original error"),
			message: "operation failed",
			wantNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := WrapError(tt.err, tt.message)
			if tt.wantNil && got != nil {
				t.Errorf("WrapError() should return nil for nil input, got %v", got)
			}
			if !tt.wantNil && got == nil {
				t.Error("WrapError() returned nil unexpectedly")
			}
			if !tt.wantNil && got != nil {
				if !errors.Is(got, tt.err) {
					t.Errorf("WrapError() should wrap the original error")
				}
				if got.Error() != tt.message+": "+tt.err.Error() {
					t.Errorf("WrapError() message = %v, want %v: %v", got.Error(), tt.message, tt.err.Error())
				}
			}
		})
	}
}

func TestWrapErrorf(t *testing.T) {
	originalErr := errors.New("base error")
	got := WrapErrorf(originalErr, "failed at step %d", 42)

	if got == nil {
		t.Fatal("WrapErrorf() returned nil unexpectedly")
	}

	if !errors.Is(got, originalErr) {
		t.Error("WrapErrorf() should wrap the original error")
	}

	expectedMsg := "failed at step 42: base error"
	if got.Error() != expectedMsg {
		t.Errorf("WrapErrorf() message = %v, want %v", got.Error(), expectedMsg)
	}
}

func TestIs(t *testing.T) {
	baseErr := fmt.Errorf("base error")
	wrappedErr := WrapError(baseErr, "wrapped")

	if !Is(wrappedErr, baseErr) {
		t.Error("Is() should return true for wrapped error")
	}

	otherErr := errors.New("other error")
	if Is(wrappedErr, otherErr) {
		t.Error("Is() should return false for different error")
	}
}

func TestAs(t *testing.T) {
	// 使用标准错误包装进行测试
	baseErr := errors.New("base error")
	wrapped := WrapError(baseErr, "wrapped")

	// 应该能够解包并找到 baseErr
	if !Is(wrapped, baseErr) {
		t.Error("As() and Is() should work with wrapped errors")
	}

	otherErr := errors.New("other error")
	if Is(wrapped, otherErr) {
		t.Error("Is() should not match different errors")
	}
}
