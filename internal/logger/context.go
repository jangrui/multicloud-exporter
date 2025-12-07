package logger

import (
	"fmt"
	"strings"
)

// ContextLogger wraps the standard logger to provide consistent context fields
// like account_id, region, namespace, etc. to adhere to DRY principles.
type ContextLogger struct {
	// fields stores key-value pairs in order
	fields []interface{}
	// prefix is an optional string to prepend to messages (e.g. "Aliyun")
	prefix string
}

// NewContextLogger creates a new logger with base context
// Usage: logger.NewContextLogger("Aliyun", "account_id", accID, "region", region)
func NewContextLogger(prefix string, kv ...interface{}) *ContextLogger {
	return &ContextLogger{
		prefix: prefix,
		fields: kv,
	}
}

// With adds more context fields and returns a new ContextLogger instance
func (l *ContextLogger) With(kv ...interface{}) *ContextLogger {
	newFields := make([]interface{}, len(l.fields)+len(kv))
	copy(newFields, l.fields)
	copy(newFields[len(l.fields):], kv)
	return &ContextLogger{
		prefix: l.prefix,
		fields: newFields,
	}
}

// buildBaseMessage constructs the base message with prefix and context fields
func (l *ContextLogger) buildBaseMessage(content string) string {
	var sb strings.Builder

	if l.prefix != "" {
		sb.WriteString(l.prefix)
		sb.WriteString(" ")
	}

	sb.WriteString(content)

	// Append context fields
	for i := 0; i < len(l.fields); i += 2 {
		if i+1 < len(l.fields) {
			sb.WriteString(fmt.Sprintf(" %s=%v", l.fields[i], l.fields[i+1]))
		}
	}

	return sb.String()
}

// Debug logs a debug message
func (l *ContextLogger) Debug(msg string) {
	if Log != nil {
		Log.Debug(l.buildBaseMessage(msg))
	}
}

// Debugf logs a formatted debug message
func (l *ContextLogger) Debugf(format string, args ...interface{}) {
	if Log != nil {
		Log.Debug(l.buildBaseMessage(fmt.Sprintf(format, args...)))
	}
}

// Info logs an info message
func (l *ContextLogger) Info(msg string) {
	if Log != nil {
		Log.Info(l.buildBaseMessage(msg))
	}
}

// Infof logs a formatted info message
func (l *ContextLogger) Infof(format string, args ...interface{}) {
	if Log != nil {
		Log.Info(l.buildBaseMessage(fmt.Sprintf(format, args...)))
	}
}

// Warn logs a warning message
func (l *ContextLogger) Warn(msg string) {
	if Log != nil {
		Log.Warn(l.buildBaseMessage(msg))
	}
}

// Warnf logs a formatted warning message
func (l *ContextLogger) Warnf(format string, args ...interface{}) {
	if Log != nil {
		Log.Warn(l.buildBaseMessage(fmt.Sprintf(format, args...)))
	}
}

// Error logs an error message
func (l *ContextLogger) Error(msg string) {
	if Log != nil {
		Log.Error(l.buildBaseMessage(msg))
	}
}

// Errorf logs a formatted error message
func (l *ContextLogger) Errorf(format string, args ...interface{}) {
	if Log != nil {
		Log.Error(l.buildBaseMessage(fmt.Sprintf(format, args...)))
	}
}
