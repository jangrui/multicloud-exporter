package config

import (
	"os"
	"path/filepath"
	"testing"

	"multicloud-exporter/internal/metrics"
)

func TestParseMetricMappings_S3(t *testing.T) {
	path := filepath.Join("..", "..", "configs", "mappings", "s3.metrics.yaml")
	m, err := ParseMetricMappings(path)
	if err != nil {
		t.Fatalf("ParseMetricMappings error: %v", err)
	}
	if m.Prefix != "s3" {
		t.Fatalf("prefix mismatch: %q", m.Prefix)
	}
	if m.Namespaces["aws"] != "AWS/S3" {
		t.Fatalf("aws namespace mismatch: %q", m.Namespaces["aws"])
	}
	if len(m.Canonical) == 0 {
		t.Fatalf("canonical empty")
	}
}

func TestLoadMetricMappings_Register(t *testing.T) {
	path := filepath.Join("..", "..", "configs", "mappings", "s3.metrics.yaml")
	if err := LoadMetricMappings(path); err != nil {
		t.Fatalf("LoadMetricMappings error: %v", err)
	}
	if metrics.GetNamespacePrefix("AWS/S3") != "s3" {
		t.Fatalf("namespace prefix not registered for AWS/S3")
	}
}

func TestParseMetricMappings_ALB(t *testing.T) {
	path := filepath.Join("..", "..", "configs", "mappings", "alb.metrics.yaml")
	m, err := ParseMetricMappings(path)
	if err != nil {
		t.Fatalf("ParseMetricMappings ALB error: %v", err)
	}
	if m.Prefix != "alb" {
		t.Fatalf("ALB prefix mismatch: %q", m.Prefix)
	}
	if m.Namespaces["aws"] != "AWS/ApplicationELB" {
		t.Fatalf("ALB aws namespace mismatch: %q", m.Namespaces["aws"])
	}
	if m.Canonical["new_connection"].Providers["aws"].Metric != "NewConnectionCount" {
		t.Fatalf("ALB new_connection aws metric mismatch: %q", m.Canonical["new_connection"].Providers["aws"].Metric)
	}
	if m.Canonical["active_connection"].Providers["aws"].Metric != "ActiveConnectionCount" {
		t.Fatalf("ALB active_connection aws metric mismatch: %q", m.Canonical["active_connection"].Providers["aws"].Metric)
	}
}

func TestParseMetricMappings_GWLB(t *testing.T) {
	path := filepath.Join("..", "..", "configs", "mappings", "gwlb.metrics.yaml")
	m, err := ParseMetricMappings(path)
	if err != nil {
		t.Fatalf("ParseMetricMappings GWLB error: %v", err)
	}
	if m.Prefix != "gwlb" {
		t.Fatalf("GWLB prefix mismatch: %q", m.Prefix)
	}
	if m.Namespaces["aws"] != "AWS/GatewayELB" {
		t.Fatalf("GWLB aws namespace mismatch: %q", m.Namespaces["aws"])
	}
	if m.Canonical["active_connection"].Providers["aws"].Metric != "ActiveFlowCount" {
		t.Fatalf("GWLB active_connection aws metric mismatch: %q", m.Canonical["active_connection"].Providers["aws"].Metric)
	}
	if m.Canonical["new_connection"].Providers["aws"].Metric != "NewFlowCount" {
		t.Fatalf("GWLB new_connection aws metric mismatch: %q", m.Canonical["new_connection"].Providers["aws"].Metric)
	}
}

// ========================================
// Task 4.1: 测试华为云 OBS 指标映射
// Property 2: 指标别名映射正确性
// Validates: Requirements 1.2, 1.3
// ========================================

func TestLoadMetricMappings_HuaweiMapping(t *testing.T) {
	// 重置 metrics 包状态
	metrics.Reset()

	path := filepath.Join("..", "..", "configs", "mappings", "s3.metrics.yaml")
	if err := LoadMetricMappings(path); err != nil {
		t.Fatalf("LoadMetricMappings error: %v", err)
	}

	// 验证华为云 namespace prefix 注册
	if prefix := metrics.GetNamespacePrefix("SYS.OBS"); prefix != "s3" {
		t.Errorf("Huawei namespace prefix mismatch: got %q, want %q", prefix, "s3")
	}

	// 验证华为云指标别名映射
	testCases := []struct {
		nativeMetric    string
		canonicalMetric string
	}{
		{"capacity_total", "storage_usage_bytes"},
		{"get_request_count", "requests_get"},
		{"put_request_count", "requests_put"},
		{"request_count_per_second", "requests_total"},
		{"upload_bytes_extranet", "traffic_internet_rx_bytes"},
		{"download_bytes_extranet", "traffic_internet_tx_bytes"},
		{"request_success_rate", "availability_pct"},
		{"download_total_request_latency", "latency_e2e_get_ms"},
		{"first_byte_latency", "latency_first_byte_ms"},
	}

	for _, tc := range testCases {
		t.Run(tc.nativeMetric, func(t *testing.T) {
			gauge, _ := metrics.NamespaceGauge("SYS.OBS", tc.nativeMetric)
			if gauge == nil {
				t.Fatalf("NamespaceGauge returned nil for metric %q", tc.nativeMetric)
			}

			// 验证指标别名映射是否正确
			alias := metrics.GetMetricAlias("SYS.OBS", tc.nativeMetric)
			if alias != tc.canonicalMetric {
				t.Errorf("Metric alias mismatch for %q: got %q, want %q", tc.nativeMetric, alias, tc.canonicalMetric)
			}
		})
	}
}

// ========================================
// Task 4.2: 测试腾讯云 COS 指标映射
// Property 2: 指标别名映射正确性
// Property 6: 缩放因子应用正确性
// Validates: Requirements 2.2, 2.3
// ========================================

func TestLoadMetricMappings_TencentMapping(t *testing.T) {
	// 重置 metrics 包状态
	metrics.Reset()

	path := filepath.Join("..", "..", "configs", "mappings", "s3.metrics.yaml")
	if err := LoadMetricMappings(path); err != nil {
		t.Fatalf("LoadMetricMappings error: %v", err)
	}

	// 验证腾讯云 namespace prefix 注册
	if prefix := metrics.GetNamespacePrefix("QCE/COS"); prefix != "s3" {
		t.Errorf("Tencent namespace prefix mismatch: got %q, want %q", prefix, "s3")
	}

	// 验证腾讯云指标别名映射
	testCases := []struct {
		nativeMetric    string
		canonicalMetric string
		expectedScale   float64
	}{
		{"StdStorage", "storage_usage_bytes", 1048576}, // 1MB = 1048576 Bytes
		{"GetRequests", "requests_get", 1},
		{"HeadRequests", "requests_head", 1},
		{"PutRequests", "requests_put", 1},
		{"TotalRequests", "requests_total", 1},
		{"InternetTrafficUp", "traffic_internet_rx_bytes", 1},
		{"InternetTrafficDown", "traffic_internet_tx_bytes", 1},
		{"RequestsSuccessRate", "availability_pct", 1},
		{"5xxResponse", "response_server_error_count", 1},
		{"FirstByteDelay", "latency_first_byte_ms", 1},
	}

	for _, tc := range testCases {
		t.Run(tc.nativeMetric, func(t *testing.T) {
			gauge, _ := metrics.NamespaceGauge("QCE/COS", tc.nativeMetric)
			if gauge == nil {
				t.Fatalf("NamespaceGauge returned nil for metric %q", tc.nativeMetric)
			}

			// 验证指标别名映射
			alias := metrics.GetMetricAlias("QCE/COS", tc.nativeMetric)
			if alias != tc.canonicalMetric {
				t.Errorf("Metric alias mismatch for %q: got %q, want %q", tc.nativeMetric, alias, tc.canonicalMetric)
			}

			// 验证缩放因子
			scale := metrics.GetMetricScale("QCE/COS", tc.canonicalMetric)
			if scale != tc.expectedScale {
				t.Errorf("Scale factor mismatch for %q: got %v, want %v", tc.canonicalMetric, scale, tc.expectedScale)
			}
		})
	}
}

// ========================================
// Task 4.3: 测试动态云厂商解析
// Property 1: 动态云厂商解析完整性
// Validates: Requirements 4.1
// ========================================

func TestLoadMetricMappings_DynamicProviderParsing(t *testing.T) {
	// 重置 metrics 包状态
	metrics.Reset()

	path := filepath.Join("..", "..", "configs", "mappings", "s3.metrics.yaml")

	// 解析配置文件
	mapping, err := ParseMetricMappings(path)
	if err != nil {
		t.Fatalf("ParseMetricMappings error: %v", err)
	}

	// 验证所有云厂商都在 namespaces 中定义
	expectedProviders := []string{"aliyun", "tencent", "aws", "huawei"}
	for _, provider := range expectedProviders {
		if _, exists := mapping.Namespaces[provider]; !exists {
			t.Errorf("Provider %q not found in namespaces", provider)
		}
	}

	// 加载映射
	if err := LoadMetricMappings(path); err != nil {
		t.Fatalf("LoadMetricMappings error: %v", err)
	}

	// 验证所有云厂商的 namespace prefix 都已注册
	for provider, namespace := range mapping.Namespaces {
		t.Run(provider, func(t *testing.T) {
			prefix := metrics.GetNamespacePrefix(namespace)
			if prefix != "s3" {
				t.Errorf("Provider %q namespace %q prefix mismatch: got %q, want %q",
					provider, namespace, prefix, "s3")
			}
		})
	}

	// 验证至少有一个共享指标在所有云厂商中都有映射
	// 选择 storage_usage_bytes 作为测试指标（所有四家云厂商都支持）
	sharedMetric := "storage_usage_bytes"
	entry, exists := mapping.Canonical[sharedMetric]
	if !exists {
		t.Fatalf("Shared metric %q not found in canonical mappings", sharedMetric)
	}

	for _, provider := range expectedProviders {
		t.Run(provider+"_"+sharedMetric, func(t *testing.T) {
			if _, exists := entry.Providers[provider]; !exists {
				t.Errorf("Provider %q does not have mapping for shared metric %q", provider, sharedMetric)
			}
		})
	}
}

// ========================================
// Task 4.4: 测试向后兼容性
// Property 3: 向后兼容性保持
// Validates: Requirements 3.1, 3.2
// ========================================

func TestLoadMetricMappings_BackwardCompatibility(t *testing.T) {
	// 重置 metrics 包状态
	metrics.Reset()

	path := filepath.Join("..", "..", "configs", "mappings", "s3.metrics.yaml")
	if err := LoadMetricMappings(path); err != nil {
		t.Fatalf("LoadMetricMappings error: %v", err)
	}

	// 测试阿里云的向后兼容性
	t.Run("Aliyun", func(t *testing.T) {
		// 验证 namespace prefix
		if prefix := metrics.GetNamespacePrefix("acs_oss_dashboard"); prefix != "s3" {
			t.Errorf("Aliyun namespace prefix changed: got %q, want %q", prefix, "s3")
		}

		// 验证关键指标映射
		aliyunMetrics := map[string]string{
			"UserStorage":         "storage_usage_bytes",
			"GetObjectCount":      "requests_get",
			"PutObjectCount":      "requests_put",
			"TotalRequestCount":   "requests_total",
			"InternetRecv":        "traffic_internet_rx_bytes",
			"InternetSend":        "traffic_internet_tx_bytes",
			"Availability":        "availability_pct",
			"ServerErrorCount":    "response_server_error_count",
			"GetObjectE2eLatency": "latency_e2e_get_ms",
		}

		for nativeMetric, canonicalMetric := range aliyunMetrics {
			gauge, _ := metrics.NamespaceGauge("acs_oss_dashboard", nativeMetric)
			if gauge == nil {
				t.Errorf("Aliyun metric %q not registered", nativeMetric)
				continue
			}

			// 验证指标别名映射
			alias := metrics.GetMetricAlias("acs_oss_dashboard", nativeMetric)
			if alias != canonicalMetric {
				t.Errorf("Aliyun metric %q alias changed: got %q, want %q", nativeMetric, alias, canonicalMetric)
			}
		}
	})

	// 测试 AWS 的向后兼容性
	t.Run("AWS", func(t *testing.T) {
		// 验证 namespace prefix
		if prefix := metrics.GetNamespacePrefix("AWS/S3"); prefix != "s3" {
			t.Errorf("AWS namespace prefix changed: got %q, want %q", prefix, "s3")
		}

		// 验证关键指标映射
		awsMetrics := map[string]string{
			"BucketSizeBytes":  "storage_usage_bytes",
			"GetRequests":      "requests_get",
			"HeadRequests":     "requests_head",
			"PutRequests":      "requests_put",
			"AllRequests":      "requests_total",
			"BytesUploaded":    "traffic_internet_rx_bytes",
			"BytesDownloaded":  "traffic_internet_tx_bytes",
			"5xxErrors":        "response_server_error_count",
			"FirstByteLatency": "latency_first_byte_ms",
		}

		for nativeMetric, canonicalMetric := range awsMetrics {
			gauge, _ := metrics.NamespaceGauge("AWS/S3", nativeMetric)
			if gauge == nil {
				t.Errorf("AWS metric %q not registered", nativeMetric)
				continue
			}

			// 验证指标别名映射
			alias := metrics.GetMetricAlias("AWS/S3", nativeMetric)
			if alias != canonicalMetric {
				t.Errorf("AWS metric %q alias changed: got %q, want %q", nativeMetric, alias, canonicalMetric)
			}
		}
	})
}

// ========================================
// Task 4.5: 测试配置缺失容错
// Property 4: 配置缺失容错性
// Validates: Requirements 4.3
// ========================================

func TestLoadMetricMappings_PartialConfig(t *testing.T) {
	// 重置 metrics 包状态
	metrics.Reset()

	path := filepath.Join("..", "..", "configs", "mappings", "s3.metrics.yaml")

	// 解析配置文件
	mapping, err := ParseMetricMappings(path)
	if err != nil {
		t.Fatalf("ParseMetricMappings error: %v", err)
	}

	// 查找只有部分云厂商定义的指标
	// 例如：latency_e2e_get_ms 只有 aliyun 和 huawei
	partialMetric := "latency_e2e_get_ms"
	entry, exists := mapping.Canonical[partialMetric]
	if !exists {
		t.Fatalf("Partial metric %q not found in canonical mappings", partialMetric)
	}

	// 验证只有部分云厂商有定义
	hasAliyun := entry.Providers["aliyun"].Metric != ""
	hasHuawei := entry.Providers["huawei"].Metric != ""
	hasTencent := entry.Providers["tencent"].Metric != ""
	hasAWS := entry.Providers["aws"].Metric != ""

	if !hasAliyun || !hasHuawei {
		t.Fatalf("Test metric %q should have aliyun and huawei definitions", partialMetric)
	}
	if hasTencent || hasAWS {
		t.Fatalf("Test metric %q should not have tencent or aws definitions", partialMetric)
	}

	// 加载映射（应该成功，不报错）
	if err := LoadMetricMappings(path); err != nil {
		t.Fatalf("LoadMetricMappings should handle partial config without error: %v", err)
	}

	// 验证有定义的云厂商能正确注册
	t.Run("Aliyun_has_mapping", func(t *testing.T) {
		gauge, _ := metrics.NamespaceGauge("acs_oss_dashboard", "GetObjectE2eLatency")
		if gauge == nil {
			t.Error("Aliyun metric should be registered")
		}
	})

	t.Run("Huawei_has_mapping", func(t *testing.T) {
		gauge, _ := metrics.NamespaceGauge("SYS.OBS", "download_total_request_latency")
		if gauge == nil {
			t.Error("Huawei metric should be registered")
		}
	})

	// 验证没有定义的云厂商不会注册该指标
	// 注意：这里我们不能直接测试"不存在"，因为 NamespaceGauge 会创建新的 gauge
	// 我们只能验证系统没有崩溃，能正常处理缺失配置
}

// ========================================
// Task 4.6: 测试新云厂商扩展性
// Property 5: 新云厂商扩展性
// Validates: Requirements 4.2
// ========================================

func TestLoadMetricMappings_NewProviderExtensibility(t *testing.T) {
	// 创建临时测试配置文件，包含一个新的虚拟云厂商
	tmpDir := t.TempDir()
	testConfigPath := filepath.Join(tmpDir, "test-s3.yaml")

	testConfig := `prefix: s3
namespaces:
  aliyun: acs_oss_dashboard
  tencent: QCE/COS
  aws: AWS/S3
  huawei: SYS.OBS
  google: GCS  # 新增的虚拟云厂商

canonical:
  storage_usage_bytes:
    description: "存储空间使用量"
    aliyun:
      metric: UserStorage
      unit: Bytes
      scale: 1
    tencent:
      metric: StdStorage
      unit: Bytes
      scale: 1048576
    aws:
      metric: BucketSizeBytes
      unit: Bytes
      scale: 1
    huawei:
      metric: capacity_total
      unit: Bytes
      scale: 1
    google:
      metric: storage_total_bytes
      unit: Bytes
      scale: 1
  requests_get:
    description: "GET 请求数"
    aliyun:
      metric: GetObjectCount
      unit: count
      scale: 1
    google:
      metric: get_request_count
      unit: count
      scale: 1
`

	if err := os.WriteFile(testConfigPath, []byte(testConfig), 0644); err != nil {
		t.Fatalf("Failed to create test config: %v", err)
	}

	// 重置 metrics 包状态
	metrics.Reset()

	// 加载包含新云厂商的配置
	if err := LoadMetricMappings(testConfigPath); err != nil {
		t.Fatalf("LoadMetricMappings should handle new provider without code changes: %v", err)
	}

	// 验证新云厂商的 namespace prefix 已注册
	if prefix := metrics.GetNamespacePrefix("GCS"); prefix != "s3" {
		t.Errorf("New provider namespace prefix not registered: got %q, want %q", prefix, "s3")
	}

	// 验证新云厂商的指标别名已注册
	gauge, _ := metrics.NamespaceGauge("GCS", "storage_total_bytes")
	if gauge == nil {
		t.Fatal("New provider metric not registered")
	}

	// 验证指标别名映射
	alias := metrics.GetMetricAlias("GCS", "storage_total_bytes")
	if alias != "storage_usage_bytes" {
		t.Errorf("New provider metric alias incorrect: got %q, want %q", alias, "storage_usage_bytes")
	}

	// 验证新云厂商的第二个指标
	gauge2, _ := metrics.NamespaceGauge("GCS", "get_request_count")
	if gauge2 == nil {
		t.Fatal("New provider second metric not registered")
	}

	alias2 := metrics.GetMetricAlias("GCS", "get_request_count")
	if alias2 != "requests_get" {
		t.Errorf("New provider second metric alias incorrect: got %q, want %q", alias2, "requests_get")
	}
}

// ========================================
// Task 4.7: 测试配置验证
// Property 7: 配置验证完整性
// Validates: Requirements 5.1, 5.4
// ========================================

// 注意：TestValidateAllMappings_OK 和 TestValidateMappingStructure_BadTopLevel
// 已经在 mappings_validate_test.go 中定义，这里不重复实现

// ========================================
// Helper functions
// ========================================

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && containsHelper(s, substr)))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
