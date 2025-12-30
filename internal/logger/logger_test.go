package logger

import (
	"multicloud-exporter/internal/config"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInit(t *testing.T) {
	// Test Init with nil config
	Init(nil)
	assert.NotNil(t, Log)

	// Test Init with stdout
	cfg := &config.LogConfig{
		Level:  "debug",
		Output: "stdout",
		Format: "console",
	}
	Init(cfg)
	assert.NotNil(t, Log)

	// Test Init with file (mocking path to avoid real file creation if possible, but here we just use temp)
	cfgFile := &config.LogConfig{
		Level:  "info",
		Output: "file",
		Format: "json",
		File: &config.FileLogConfig{
			Path: "test.log",
		},
	}
	Init(cfgFile)
	assert.NotNil(t, Log)

	// Test Init with both
	cfgBoth := &config.LogConfig{
		Level:  "warn",
		Output: "both",
		Format: "console",
		File: &config.FileLogConfig{
			Path: "test_both.log",
		},
	}
	Init(cfgBoth)
	assert.NotNil(t, Log)

	// Clean up
	Sync()
}
