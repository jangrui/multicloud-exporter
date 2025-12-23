// Package config 提供配置加载功能
package config

import (
	"fmt"
	"os"
)

// LoadConfigFile 从指定路径或默认路径列表加载配置文件
// 该函数提供了统一的配置文件加载逻辑，支持指定路径和默认路径回退机制
//
// 参数说明：
//   - path: 指定的配置文件路径。如果为空字符串，则尝试 defaultPaths 中的路径
//   - defaultPaths: 默认路径列表，按顺序尝试，找到第一个存在的文件即返回
//
// 返回值：
//   - []byte: 文件内容，如果加载失败则为 nil
//   - string: 实际使用的文件路径，如果加载失败则为空字符串
//   - error: 错误信息，如果加载成功则为 nil
//
// 示例：
//   // 从指定路径加载
//   data, actualPath, err := LoadConfigFile("/path/to/config.yaml", []string{})
//
//   // 从默认路径加载
//   data, actualPath, err := LoadConfigFile("", []string{"/app/configs/server.yaml", "./configs/server.yaml"})
func LoadConfigFile(path string, defaultPaths []string) ([]byte, string, error) {
	// 如果指定了路径，直接使用
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, "", fmt.Errorf("failed to read config file %s: %v", path, err)
		}
		return data, path, nil
	}

	// 否则尝试默认路径
	for _, p := range defaultPaths {
		if _, err := os.Stat(p); err == nil {
			data, err := os.ReadFile(p)
			if err != nil {
				return nil, "", fmt.Errorf("failed to read config file %s: %v", p, err)
			}
			return data, p, nil
		}
	}

	return nil, "", fmt.Errorf("config file not found in default paths: %v", defaultPaths)
}

