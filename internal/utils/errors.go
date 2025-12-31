package utils

import (
	"errors"
	"fmt"
)

// WrapError 包装错误并添加上下文信息
// 在整个代码库中提供统一的错误包装方式
func WrapError(err error, message string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", message, err)
}

// WrapErrorf 使用格式化字符串包装错误并添加上下文信息
func WrapErrorf(err error, format string, args ...interface{}) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", fmt.Sprintf(format, args...), err)
}

// Is 检查 err 是否等于 target
func Is(err, target error) bool {
	return errors.Is(err, target)
}

// As 检查 err 是否可以转换为 target 类型
func As(err error, target interface{}) bool {
	return errors.As(err, target)
}
