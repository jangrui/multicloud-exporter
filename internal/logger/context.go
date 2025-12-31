package logger

import (
	"fmt"
	"strings"
)

// ContextLogger 包装标准日志记录器，提供一致的上下文字段
// 如 account_id、region、namespace 等，遵循 DRY 原则
type ContextLogger struct {
	// fields 按顺序存储键值对
	fields []interface{}
	// prefix 是可选的消息前缀（如 "Aliyun"）
	prefix string
}

// NewContextLogger 创建带有基础上下文的新日志记录器
// 用法: logger.NewContextLogger("Aliyun", "account_id", accID, "region", region)
func NewContextLogger(prefix string, kv ...interface{}) *ContextLogger {
	return &ContextLogger{
		prefix: prefix,
		fields: kv,
	}
}

// With 添加更多上下文字段并返回新的 ContextLogger 实例
func (l *ContextLogger) With(kv ...interface{}) *ContextLogger {
	newFields := make([]interface{}, len(l.fields)+len(kv))
	copy(newFields, l.fields)
	copy(newFields[len(l.fields):], kv)
	return &ContextLogger{
		prefix: l.prefix,
		fields: newFields,
	}
}

// buildBaseMessage 构建带前缀和上下文字段的基础消息
func (l *ContextLogger) buildBaseMessage(content string) string {
	var sb strings.Builder

	if l.prefix != "" {
		sb.WriteString(l.prefix)
		sb.WriteString(" ")
	}

	sb.WriteString(content)

	// 追加上下文字段
	for i := 0; i < len(l.fields); i += 2 {
		if i+1 < len(l.fields) {
			sb.WriteString(fmt.Sprintf(" %s=%v", l.fields[i], l.fields[i+1]))
		}
	}

	return sb.String()
}

// Debug 记录调试消息
func (l *ContextLogger) Debug(msg string) {
	if Log != nil {
		Log.Debug(l.buildBaseMessage(msg))
	}
}

// Debugf 记录格式化的调试消息
func (l *ContextLogger) Debugf(format string, args ...interface{}) {
	if Log != nil {
		Log.Debug(l.buildBaseMessage(fmt.Sprintf(format, args...)))
	}
}

// Info 记录信息消息
func (l *ContextLogger) Info(msg string) {
	if Log != nil {
		Log.Info(l.buildBaseMessage(msg))
	}
}

// Infof 记录格式化的信息消息
func (l *ContextLogger) Infof(format string, args ...interface{}) {
	if Log != nil {
		Log.Info(l.buildBaseMessage(fmt.Sprintf(format, args...)))
	}
}

// Warn 记录警告消息
func (l *ContextLogger) Warn(msg string) {
	if Log != nil {
		Log.Warn(l.buildBaseMessage(msg))
	}
}

// Warnf 记录格式化的警告消息
func (l *ContextLogger) Warnf(format string, args ...interface{}) {
	if Log != nil {
		Log.Warn(l.buildBaseMessage(fmt.Sprintf(format, args...)))
	}
}

// Error 记录错误消息
func (l *ContextLogger) Error(msg string) {
	if Log != nil {
		Log.Error(l.buildBaseMessage(msg))
	}
}

// Errorf 记录格式化的错误消息
func (l *ContextLogger) Errorf(format string, args ...interface{}) {
	if Log != nil {
		Log.Error(l.buildBaseMessage(fmt.Sprintf(format, args...)))
	}
}
