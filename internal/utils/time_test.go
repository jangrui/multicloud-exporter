package utils

import (
	"testing"
	"time"
)

func TestParseDurationDays(t *testing.T) {
	d, err := ParseDuration("2d")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if d != 48*time.Hour {
		t.Fatalf("val")
	}
}
