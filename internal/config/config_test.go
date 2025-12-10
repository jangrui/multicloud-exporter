package config

import (
	"os"
	"testing"
)

func TestExpandEnv(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		env      map[string]string
		expected string
	}{
		{
			name:     "no env vars",
			input:    "val: ${VAR}",
			env:      nil,
			expected: "val: ",
		},
		{
			name:     "simple substitution",
			input:    "val: ${VAR}",
			env:      map[string]string{"VAR": "123"},
			expected: "val: 123",
		},
		{
			name:     "default value used",
			input:    "val: ${VAR:-456}",
			env:      nil,
			expected: "val: 456",
		},
		{
			name:     "default value ignored when env set",
			input:    "val: ${VAR:-456}",
			env:      map[string]string{"VAR": "123"},
			expected: "val: 123",
		},
		{
			name:     "empty env var uses default",
			input:    "val: ${VAR:-456}",
			env:      map[string]string{"VAR": ""},
			expected: "val: 456", // My implementation treats empty env as "not set" effectively if I want defaults?
			// Wait, os.LookupEnv returns empty string and true.
			// My code: if v, ok := os.LookupEnv(k); ok && v != "" { return v } return def
			// So empty env var -> use default. This matches shell behavior `:-`.
		},
		{
			name:     "multiple vars",
			input:    "a: ${A:-1}, b: ${B:-2}",
			env:      map[string]string{"A": "10"},
			expected: "a: 10, b: 2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set env vars
			for k, v := range tt.env {
				_ = os.Setenv(k, v)
				defer func(k string) {
					_ = os.Unsetenv(k)
				}(k)
			}
			// Ensure unset vars are unset (cleanup might be needed if env leaks, but here we assume clean state or overwrite)
			// For "no env vars" case, we need to make sure VAR is not set.
			if tt.env == nil {
				_ = os.Unsetenv("VAR")
			}

			got := expandEnv(tt.input)
			if got != tt.expected {
				t.Errorf("expandEnv(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
