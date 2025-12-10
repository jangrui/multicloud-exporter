package config

import (
	"os"
	"testing"

	"multicloud-exporter/internal/metrics"
)

func TestLoadMetricMappings(t *testing.T) {
	// Create a temporary mapping file
	content := []byte(`
prefix: test
namespaces:
  aliyun: acs_test
  tencent: QCE/TEST
canonical:
  test_metric:
    aliyun:
      metric: AliyunMetric
      scale: 1
    tencent:
      metric: TencentMetric
      scale: 10
`)
	tmpfile, err := os.CreateTemp("", "mapping_test.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(tmpfile.Name()) }()
	if _, err := tmpfile.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}

	// Test loading
	if err := LoadMetricMappings(tmpfile.Name()); err != nil {
		t.Fatalf("LoadMetricMappings failed: %v", err)
	}

	// Verify registration (indirectly via metrics package)
	// Since metrics package uses global state, we can verify by checking if the alias is registered.
	// We assume metrics package works (it's tested separately ideally), but here we check if Load called it.

	// Check Aliyun
	alias := metrics.GetMetricAlias("acs_test", "AliyunMetric")
	if alias != "test_metric" {
		t.Errorf("expected alias test_metric, got %s", alias)
	}

	// Check Tencent
	aliasT := metrics.GetMetricAlias("QCE/TEST", "TencentMetric")
	if aliasT != "test_metric" {
		t.Errorf("expected alias test_metric, got %s", aliasT)
	}

	// Check Scale
	scale := metrics.GetMetricScale("QCE/TEST", "TencentMetric")
	if scale != 10 {
		t.Errorf("expected scale 10, got %f", scale)
	}
}

func TestLoadMetricMappings_MissingPrefix(t *testing.T) {
	content := []byte(`
namespaces:
  aliyun: acs_test
`)
	tmpfile, err := os.CreateTemp("", "mapping_invalid.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(tmpfile.Name()) }()
	if _, err := tmpfile.Write(content); err != nil {
		t.Fatal(err)
	}
	_ = tmpfile.Close()

	if err := LoadMetricMappings(tmpfile.Name()); err == nil {
		t.Fatal("expected error for missing prefix, got nil")
	}
}

func TestLoadMetricMappings_MissingNamespaces(t *testing.T) {
	content := []byte(`
prefix: test
`)
	tmpfile, err := os.CreateTemp("", "mapping_invalid_ns.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(tmpfile.Name()) }()
	if _, err := tmpfile.Write(content); err != nil {
		t.Fatal(err)
	}
	_ = tmpfile.Close()

	if err := LoadMetricMappings(tmpfile.Name()); err == nil {
		t.Fatal("expected error for missing namespaces, got nil")
	}
}

func TestParseMetricMappings(t *testing.T) {
	content := []byte(`
prefix: test
namespaces:
  aliyun: acs_test
canonical:
  test_metric:
    aliyun:
      metric: AliyunMetric
`)
	tmpfile, err := os.CreateTemp("", "mapping_parse.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(tmpfile.Name()) }()
	if _, err := tmpfile.Write(content); err != nil {
		t.Fatal(err)
	}
	_ = tmpfile.Close()

	mapping, err := ParseMetricMappings(tmpfile.Name())
	if err != nil {
		t.Fatalf("ParseMetricMappings failed: %v", err)
	}
	if mapping.Prefix != "test" {
		t.Errorf("expected prefix test, got %s", mapping.Prefix)
	}
}

func TestParseMetricMappings_Error(t *testing.T) {
	// Test file not found
	_, err := ParseMetricMappings("non_existent_file.yaml")
	if err == nil {
		t.Error("expected error for non-existent file, got nil")
	}

	// Test invalid YAML
	content := []byte(`invalid_yaml: : :`)
	tmpfile, err := os.CreateTemp("", "mapping_invalid_parse.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(tmpfile.Name()) }()
	if _, err := tmpfile.Write(content); err != nil {
		t.Fatal(err)
	}
	_ = tmpfile.Close()

	_, err = ParseMetricMappings(tmpfile.Name())
	if err == nil {
		t.Error("expected error for invalid YAML, got nil")
	}
}

func TestLoadMetricMappings_FileError(t *testing.T) {
	// Test file not found
	err := LoadMetricMappings("non_existent_file.yaml")
	if err == nil {
		t.Error("expected error for non-existent file, got nil")
	}

	// Test invalid YAML
	content := []byte(`invalid_yaml: : :`)
	tmpfile, err := os.CreateTemp("", "mapping_invalid_load.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(tmpfile.Name()) }()
	if _, err := tmpfile.Write(content); err != nil {
		t.Fatal(err)
	}
	_ = tmpfile.Close()

	err = LoadMetricMappings(tmpfile.Name())
	if err == nil {
		t.Error("expected error for invalid YAML, got nil")
	}
}
