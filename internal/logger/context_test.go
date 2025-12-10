package logger

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestContextLogger(t *testing.T) {
	// Setup observer to capture logs
	core, observedLogs := observer.New(zap.DebugLevel)
	originalLog := Log
	Log = zap.New(core).Sugar()
	defer func() { Log = originalLog }()

	// Test NewContextLogger
	l := NewContextLogger("TestPrefix", "key1", "value1")
	assert.NotNil(t, l)

	// Test Debug
	l.Debug("debug msg")
	assert.Equal(t, 1, observedLogs.Len())
	assert.Equal(t, "TestPrefix debug msg key1=value1", observedLogs.All()[0].Message)
	assert.Equal(t, zap.DebugLevel, observedLogs.All()[0].Level)

	// Test Info
	l.Info("info msg")
	assert.Equal(t, 2, observedLogs.Len())
	assert.Equal(t, "TestPrefix info msg key1=value1", observedLogs.All()[1].Message)
	assert.Equal(t, zap.InfoLevel, observedLogs.All()[1].Level)

	// Test Warn
	l.Warn("warn msg")
	assert.Equal(t, 3, observedLogs.Len())
	assert.Equal(t, "TestPrefix warn msg key1=value1", observedLogs.All()[2].Message)
	assert.Equal(t, zap.WarnLevel, observedLogs.All()[2].Level)

	// Test Error
	l.Error("error msg")
	assert.Equal(t, 4, observedLogs.Len())
	assert.Equal(t, "TestPrefix error msg key1=value1", observedLogs.All()[3].Message)
	assert.Equal(t, zap.ErrorLevel, observedLogs.All()[3].Level)

	// Test With
	l2 := l.With("key2", "value2")
	l2.Info("with msg")
	assert.Equal(t, 5, observedLogs.Len())
	assert.Equal(t, "TestPrefix with msg key1=value1 key2=value2", observedLogs.All()[4].Message)

	// Test Formatted logs
	l.Debugf("debug %s", "fmt")
	assert.Equal(t, "TestPrefix debug fmt key1=value1", observedLogs.All()[5].Message)

	l.Infof("info %s", "fmt")
	assert.Equal(t, "TestPrefix info fmt key1=value1", observedLogs.All()[6].Message)

	l.Warnf("warn %s", "fmt")
	assert.Equal(t, "TestPrefix warn fmt key1=value1", observedLogs.All()[7].Message)

	l.Errorf("error %s", "fmt")
	assert.Equal(t, "TestPrefix error fmt key1=value1", observedLogs.All()[8].Message)
}
