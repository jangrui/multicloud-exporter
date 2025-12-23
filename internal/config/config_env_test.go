package config

import (
	"os"
	"testing"
)

func TestExpandEnv_DefaultValue(t *testing.T) {
	_ = os.Unsetenv("ABC")
	in := "value=${ABC:-fallback}"
	out := expandEnv(in)
	if out != "value=fallback" {
		t.Fatalf("expandEnv default mismatch: got=%q", out)
	}
}

func TestExpandEnv_Provided(t *testing.T) {
	_ = os.Setenv("ABC", "zzz")
	defer func() { _ = os.Unsetenv("ABC") }()
	in := "value=${ABC:-fallback}"
	out := expandEnv(in)
	if out != "value=zzz" {
		t.Fatalf("expandEnv provided mismatch: got=%q", out)
	}
}
