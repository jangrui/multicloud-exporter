package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateMappingFile_ValidFiles(t *testing.T) {
	// 测试实际的映射文件
	mappingsDir := filepath.Join("..", "..", "configs", "mappings")

	files, err := filepath.Glob(filepath.Join(mappingsDir, "*.yaml"))
	if err != nil {
		t.Fatalf("列出映射文件失败: %v", err)
	}

	if len(files) == 0 {
		t.Skip("没有找到映射文件")
	}

	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			if err := validateMappingFile(f); err != nil {
				t.Errorf("验证映射文件失败: %v", err)
			}
		})
	}
}

func TestValidateMappingFile_InvalidYAML(t *testing.T) {
	// 创建临时文件测试无效 YAML
	tmpDir := t.TempDir()
	invalidFile := filepath.Join(tmpDir, "invalid.yaml")

	content := []byte(`
prefix: test
namespaces:
  aliyun: acs_test
  invalid yaml: [
`)

	if err := os.WriteFile(invalidFile, content, 0644); err != nil {
		t.Fatalf("创建测试文件失败: %v", err)
	}

	if err := validateMappingFile(invalidFile); err == nil {
		t.Error("期望返回错误，但返回了 nil")
	}
}

func TestValidateMappingFile_MissingPrefix(t *testing.T) {
	tmpDir := t.TempDir()
	invalidFile := filepath.Join(tmpDir, "no-prefix.yaml")

	content := []byte(`
namespaces:
  aliyun: acs_test
canonical:
  test_metric:
    aliyun:
      metric: TestMetric
`)

	if err := os.WriteFile(invalidFile, content, 0644); err != nil {
		t.Fatalf("创建测试文件失败: %v", err)
	}

	if err := validateMappingFile(invalidFile); err == nil {
		t.Error("期望返回错误（缺少 prefix），但返回了 nil")
	}
}

func TestValidateMappingFile_EmptyCanonical(t *testing.T) {
	tmpDir := t.TempDir()
	invalidFile := filepath.Join(tmpDir, "empty-canonical.yaml")

	content := []byte(`
prefix: test
namespaces:
  aliyun: acs_test
canonical: {}
`)

	if err := os.WriteFile(invalidFile, content, 0644); err != nil {
		t.Fatalf("创建测试文件失败: %v", err)
	}

	if err := validateMappingFile(invalidFile); err == nil {
		t.Error("期望返回错误（canonical 为空），但返回了 nil")
	}
}
