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

func TestValidateMappingFile_EmptyNamespaces(t *testing.T) {
	tmpDir := t.TempDir()
	invalidFile := filepath.Join(tmpDir, "empty-namespaces.yaml")

	content := []byte(`
prefix: test
namespaces: {}
canonical:
  test_metric:
    aliyun:
      metric: TestMetric
`)

	if err := os.WriteFile(invalidFile, content, 0644); err != nil {
		t.Fatalf("创建测试文件失败: %v", err)
	}

	if err := validateMappingFile(invalidFile); err == nil {
		t.Error("期望返回错误（namespaces 为空），但返回了 nil")
	}
}

func TestValidateMappingFile_DuplicateCanonical(t *testing.T) {
	tmpDir := t.TempDir()
	invalidFile := filepath.Join(tmpDir, "duplicate-canonical.yaml")

	content := []byte(`
prefix: test
namespaces:
  aliyun: acs_test
canonical:
  test_metric:
    aliyun:
      metric: TestMetric
  test_metric:
    tencent:
      metric: TestMetric
`)

	if err := os.WriteFile(invalidFile, content, 0644); err != nil {
		t.Fatalf("创建测试文件失败: %v", err)
	}

	if err := validateMappingFile(invalidFile); err == nil {
		t.Error("期望返回错误（重复的规范名称），但返回了 nil")
	}
}

func TestValidateMappingFile_NoProviderMetric(t *testing.T) {
	tmpDir := t.TempDir()
	invalidFile := filepath.Join(tmpDir, "no-provider-metric.yaml")

	content := []byte(`
prefix: test
namespaces:
  aliyun: acs_test
canonical:
  test_metric:
    aliyun:
      dimensions: ["InstanceId"]
`)

	if err := os.WriteFile(invalidFile, content, 0644); err != nil {
		t.Fatalf("创建测试文件失败: %v", err)
	}

	if err := validateMappingFile(invalidFile); err == nil {
		t.Error("期望返回错误（没有云厂商的指标），但返回了 nil")
	}
}

func TestValidateMappingFile_ValidComplete(t *testing.T) {
	tmpDir := t.TempDir()
	validFile := filepath.Join(tmpDir, "valid.yaml")

	content := []byte(`
prefix: test_prefix
namespaces:
  aliyun: acs_test
  tencent: qce_test
canonical:
  test_metric:
    description: "测试指标"
    aliyun:
      metric: TestMetric
      dimensions: ["InstanceId"]
      unit: "Count"
      scale: 1.0
      description: "阿里云测试指标"
    tencent:
      metric: TestMetric
      dimensions: ["InstanceId"]
`)

	if err := os.WriteFile(validFile, content, 0644); err != nil {
		t.Fatalf("创建测试文件失败: %v", err)
	}

	if err := validateMappingFile(validFile); err != nil {
		t.Errorf("期望验证通过，但返回了错误: %v", err)
	}
}

func TestValidateMappingFile_MultipleProviders(t *testing.T) {
	tmpDir := t.TempDir()
	validFile := filepath.Join(tmpDir, "multi-provider.yaml")

	content := []byte(`
prefix: multi_provider
namespaces:
  aliyun: acs_multi
  tencent: qce_multi
  huawei: Huawei_multi
  aws: AWS_multi
canonical:
  network_in:
    description: "入站网络流量"
    aliyun:
      metric: NetworkIn
      dimensions: ["instanceId"]
    tencent:
      metric: InTraffic
      dimensions: ["instanceId"]
    huawei:
      metric: network_in_rate
      dimensions: ["instance_id"]
    aws:
      metric: NetworkIn
      dimensions: ["InstanceId"]
`)

	if err := os.WriteFile(validFile, content, 0644); err != nil {
		t.Fatalf("创建测试文件失败: %v", err)
	}

	if err := validateMappingFile(validFile); err != nil {
		t.Errorf("期望验证通过，但返回了错误: %v", err)
	}
}

func TestValidateMappingFile_PartialProvider(t *testing.T) {
	tmpDir := t.TempDir()
	validFile := filepath.Join(tmpDir, "partial-provider.yaml")

	content := []byte(`
prefix: partial
namespaces:
  aliyun: acs_partial
canonical:
  partial_metric:
    description: "部分云厂商支持的指标"
    aliyun:
      metric: PartialMetric
      dimensions: ["InstanceId"]
`)

	if err := os.WriteFile(validFile, content, 0644); err != nil {
		t.Fatalf("创建测试文件失败: %v", err)
	}

	if err := validateMappingFile(validFile); err != nil {
		t.Errorf("期望验证通过，但返回了错误: %v", err)
	}
}
