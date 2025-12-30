package main

import (
	"os"
)

// getEnv 获取环境变量，如果不存在返回空字符串
func getEnv(key string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return ""
}
