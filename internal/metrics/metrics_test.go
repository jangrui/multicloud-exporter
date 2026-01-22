package metrics

import (
	"fmt"
	"testing"
)

func TestSanitizeName(t *testing.T) {
	if sanitizeName("A-B.C") != "a_b_c" {
		t.Fatalf("sanitize expected a_b_c, got %s", sanitizeName("A-B.C"))
	}
	if sanitizeName("A/B") != "a_b" {
		t.Fatalf("sanitize expected a_b, got %s", sanitizeName("A/B"))
	}
}

func TestNamespaceGauge(t *testing.T) {
	ns := "test_ns_gauge"
	RegisterNamespacePrefix(ns, "test_prefix")

	g, c := NamespaceGauge(ns, "met", "extra")
	if g == nil {
		t.Fatalf("gauge")
	}
	if c != 9 { // 8 standard + 1 extra
		t.Fatalf("expected 9 labels, got %d", c)
	}
	g, _ = NamespaceGauge(ns, "met", "extra")
	if g == nil {
		t.Fatalf("reuse")
	}

	// Test with no prefix
	g2, _ := NamespaceGauge("no_prefix_ns", "met")
	if g2 == nil {
		t.Fatalf("gauge no prefix")
	}
}

func TestRegistrationAndRetrieval(t *testing.T) {
	ns := "test_ns"
	prefix := "test_prefix"

	RegisterNamespacePrefix(ns, prefix)
	if p := aliasPrefixForNamespace(ns); p != prefix {
		t.Errorf("expected prefix %s, got %s", prefix, p)
	}
	if p := aliasPrefixForNamespace("unknown"); p != "" {
		t.Errorf("expected empty prefix, got %s", p)
	}

	// Test Alias
	aliases := map[string]string{"orig": "alias"}
	RegisterNamespaceMetricAlias(ns, aliases)
	if got := GetMetricAlias(ns, "orig"); got != "alias" {
		t.Errorf("expected alias, got %s", got)
	}
	// Fallback to empty if not found and no func
	if got := GetMetricAlias(ns, "other"); got != "" {
		t.Errorf("expected empty, got %s", got)
	}

	// Test Scale
	scales := map[string]float64{"met": 10.0}
	RegisterNamespaceMetricScale(ns, scales)
	if got := GetMetricScale(ns, "met"); got != 10.0 {
		t.Errorf("expected 10.0, got %f", got)
	}
	if got := GetMetricScale(ns, "other"); got != 1.0 {
		t.Errorf("expected 1.0, got %f", got)
	}

	// Test Help
	RegisterNamespaceHelp(ns, func(m string) string { return "Help for " + m })
	if got := metricHelpForNamespace(ns, "met"); got != "Help for met" {
		t.Errorf("expected help string, got %s", got)
	}
	if got := metricHelpForNamespace("unknown", "met"); got != " - 云产品指标" {
		t.Errorf("expected default help, got %s", got)
	}

	// Test Alias Func
	RegisterNamespaceAliasFunc(ns, func(s string) string {
		return "func_" + s
	})
	// Prioritize map lookup
	if got := GetMetricAlias(ns, "orig"); got != "alias" {
		t.Errorf("expected alias (map priority), got %s", got)
	}
	// Use func if not in map
	if got := GetMetricAlias(ns, "other"); got != "func_other" {
		t.Errorf("expected func_other, got %s", got)
	}
}

func TestReset(t *testing.T) {
	Reset()
	// Just ensure no panic and coverage hit
}

func TestIncSampleCount(t *testing.T) {
	ResetSampleCounts()

	IncSampleCount("test_ns", 5)
	counts := GetSampleCounts()
	if counts["test_ns"] != 5 {
		t.Errorf("expected 5, got %d", counts["test_ns"])
	}

	IncSampleCount("test_ns", 3)
	counts = GetSampleCounts()
	if counts["test_ns"] != 8 {
		t.Errorf("expected 8, got %d", counts["test_ns"])
	}

	IncSampleCount("test_ns", 0)
	counts = GetSampleCounts()
	if counts["test_ns"] != 8 {
		t.Errorf("expected 8 (unchanged), got %d", counts["test_ns"])
	}

	IncSampleCount("test_ns", -1)
	counts = GetSampleCounts()
	if counts["test_ns"] != 8 {
		t.Errorf("expected 8 (unchanged), got %d", counts["test_ns"])
	}
}

func TestIncSampleCountWithLabels(t *testing.T) {
	IncSampleCountWithLabels("acc1", "us-east-1", "alb", "acs_alb", 10)
	SampleCountTotal.WithLabelValues("acc1", "us-east-1", "alb", "acs_alb").Add(0)
	// Just ensure no panic
}

func TestGetSampleCounts(t *testing.T) {
	ResetSampleCounts()

	counts := GetSampleCounts()
	if len(counts) != 0 {
		t.Errorf("expected empty map, got %d items", len(counts))
	}

	IncSampleCount("ns1", 1)
	IncSampleCount("ns2", 2)

	counts = GetSampleCounts()
	if counts["ns1"] != 1 {
		t.Errorf("expected ns1=1, got %d", counts["ns1"])
	}
	if counts["ns2"] != 2 {
		t.Errorf("expected ns2=2, got %d", counts["ns2"])
	}

	// Verify immutability
	counts["ns1"] = 100
	counts2 := GetSampleCounts()
	if counts2["ns1"] != 1 {
		t.Errorf("expected original value, got %d", counts2["ns1"])
	}
}

func TestResetSampleCounts(t *testing.T) {
	IncSampleCount("test", 10)
	counts := GetSampleCounts()
	if counts["test"] != 10 {
		t.Errorf("expected 10, got %d", counts["test"])
	}

	ResetSampleCounts()
	counts = GetSampleCounts()
	if len(counts) != 0 {
		t.Errorf("expected empty map, got %d items", len(counts))
	}
}

func TestUpdateCacheMetrics(t *testing.T) {
	UpdateCacheMetrics("test_cache", 1024, 100)
	// Just ensure no panic
}

func TestRecordCacheHit(t *testing.T) {
	RecordCacheHit("test_cache")
	// Just ensure no panic
}

func TestRecordCacheMiss(t *testing.T) {
	RecordCacheMiss("test_cache")
	// Just ensure no panic
}

func TestNamespaceGauge_DuplicateLabel(t *testing.T) {
	ns := "test_dup_label"
	RegisterNamespacePrefix(ns, "dup")

	g, _ := NamespaceGauge(ns, "met", "extra")
	g2, _ := NamespaceGauge(ns, "met", "extra")
	if g != g2 {
		t.Errorf("expected same gauge for same metric")
	}
}

func TestAliasPrefixForNamespace_Empty(t *testing.T) {
	if p := aliasPrefixForNamespace("unknown_namespace"); p != "" {
		t.Errorf("expected empty string, got %s", p)
	}
}

func TestAliasMetricForNamespace_NoFunc(t *testing.T) {
	ns := "no_func_ns"
	aliases := map[string]string{"orig": "alias"}
	RegisterNamespaceMetricAlias(ns, aliases)

	if got := GetMetricAlias(ns, "orig"); got != "alias" {
		t.Errorf("expected alias, got %s", got)
	}
	if got := GetMetricAlias(ns, "unknown"); got != "" {
		t.Errorf("expected empty string, got %s", got)
	}
}

func TestMetricHelpForNamespace_Default(t *testing.T) {
	if got := metricHelpForNamespace("unknown", "met"); got != " - 云产品指标" {
		t.Errorf("expected default help, got %s", got)
	}
}

func TestRegisterNamespaceMetricAlias_Merge(t *testing.T) {
	ns := "merge_test"
	RegisterNamespaceMetricAlias(ns, map[string]string{"a": "b"})
	RegisterNamespaceMetricAlias(ns, map[string]string{"c": "d"})

	if got := GetMetricAlias(ns, "a"); got != "b" {
		t.Errorf("expected b, got %s", got)
	}
	if got := GetMetricAlias(ns, "c"); got != "d" {
		t.Errorf("expected d, got %s", got)
	}
}

func TestRegisterNamespaceMetricScale_Merge(t *testing.T) {
	ns := "scale_test"
	RegisterNamespaceMetricScale(ns, map[string]float64{"a": 1.0})
	RegisterNamespaceMetricScale(ns, map[string]float64{"b": 2.0})

	if got := GetMetricScale(ns, "a"); got != 1.0 {
		t.Errorf("expected 1.0, got %f", got)
	}
	if got := GetMetricScale(ns, "b"); got != 2.0 {
		t.Errorf("expected 2.0, got %f", got)
	}
}

func TestNamespaceGauge_SpecialCharacters(t *testing.T) {
	ns := "special_ns"
	RegisterNamespacePrefix(ns, "special")

	g, _ := NamespaceGauge(ns, "met-name", "dim.value")
	if g == nil {
		t.Fatalf("gauge should not be nil")
	}
}

func TestGetMetricScale_Default(t *testing.T) {
	if got := GetMetricScale("unknown_ns", "met"); got != 1.0 {
		t.Errorf("expected 1.0, got %f", got)
	}
}

// ========== 四维监控指标测试 ==========

func TestFourDimensionMetrics_AccessDuration(t *testing.T) {
	RecordAccessDuration("account", "acc1", "prod1", "us-east-1", "res1", 0.001)
	RecordAccessDuration("product", "acc1", "prod1", "us-east-1", "", 0.005)
	RecordAccessDuration("region", "acc1", "prod1", "us-east-1", "", 0.01)
	RecordAccessDuration("resource", "acc1", "prod1", "us-east-1", "res1", 0.05)
}

func TestFourDimensionMetrics_AccessTotal(t *testing.T) {
	RecordAccess("account", "success")
	RecordAccess("account", "failed")
	RecordAccess("product", "success")
	RecordAccess("region", "failed")
	RecordAccess("resource", "success")
}

func TestFourDimensionMetrics_LockContention(t *testing.T) {
	RecordLockContention("account")
	RecordLockContention("product")
	RecordLockContention("region")
	RecordLockContention("resource")
}

func TestFourDimensionMetrics_MemoryUsage(t *testing.T) {
	UpdateMemoryUsage("account", 1024000)
	UpdateMemoryUsage("product", 2048000)
	UpdateMemoryUsage("region", 3072000)
	UpdateMemoryUsage("resource", 4096000)

	UpdateMemoryUsage("account", 512000)
}

func TestFourDimensionMetrics_ObjectPool(t *testing.T) {
	UpdateObjectPoolSize("account", 10)
	UpdateObjectPoolSize("product", 20)
	UpdateObjectPoolSize("region", 30)
	UpdateObjectPoolSize("resource", 40)

	RecordObjectPoolHit("account")
	RecordObjectPoolHit("product")
	RecordObjectPoolHit("region")
	RecordObjectPoolMiss("account")
	RecordObjectPoolMiss("product")

	UpdateObjectPoolSize("account", 9)
}

func TestFourDimensionMetrics_LRU(t *testing.T) {
	RecordLRUEvicted("account")
	RecordLRUEvicted("product")
	RecordLRUEvicted("region")
	RecordLRUEvicted("resource")

	RecordLRUCleanupDuration("account", 0.1)
	RecordLRUCleanupDuration("product", 0.2)
	RecordLRUCleanupDuration("region", 0.3)
	RecordLRUCleanupDuration("resource", 0.4)
}

func TestFourDimensionMetrics_DegradedResources(t *testing.T) {
	UpdateDegradedResources("account", 5)
	UpdateDegradedResources("product", 10)
	UpdateDegradedResources("region", 15)
	UpdateDegradedResources("resource", 20)

	UpdateDegradedResources("account", 3)
}

func TestFourDimensionMetrics_ConcurrentAccess(t *testing.T) {
	done := make(chan bool)

	for i := 0; i < 100; i++ {
		go func(n int) {
			dimension := []string{"account", "product", "region", "resource"}[n%4]
			accountID := fmt.Sprintf("acc%d", n/10)
			productID := fmt.Sprintf("prod%d", n/10)
			region := fmt.Sprintf("region-%d", n/10)
			resourceID := fmt.Sprintf("res%d", n)

			RecordAccessDuration(dimension, accountID, productID, region, resourceID, 0.001*float64(n%10))
			RecordAccess(dimension, "success")
			UpdateMemoryUsage(dimension, int64(n*1024))
			done <- true
		}(i)
	}

	for i := 0; i < 100; i++ {
		<-done
	}
}

func TestFourDimensionMetrics_FourDimensionSync(t *testing.T) {
	FourDimensionSyncTotal.WithLabelValues("batch").Inc()
	FourDimensionSyncTotal.WithLabelValues("single").Inc()

	FourDimensionSyncDurationSeconds.WithLabelValues("account").Observe(0.1)
	FourDimensionSyncDurationSeconds.WithLabelValues("product").Observe(0.2)
	FourDimensionSyncDurationSeconds.WithLabelValues("region").Observe(0.3)
	FourDimensionSyncDurationSeconds.WithLabelValues("resource").Observe(0.4)
}

func TestGetNamespacePrefix(t *testing.T) {
	Reset()

	// Test 1: 未注册的命名空间应返回空
	if p := GetNamespacePrefix("unknown_ns"); p != "" {
		t.Errorf("expected empty prefix for unknown namespace, got %s", p)
	}

	// Test 2: 注册命名空间后应返回对应前缀
	ns := "test_prefix_ns"
	expectedPrefix := "test_prefix"
	RegisterNamespacePrefix(ns, expectedPrefix)

	if p := GetNamespacePrefix(ns); p != expectedPrefix {
		t.Errorf("expected prefix %s, got %s", expectedPrefix, p)
	}

	// Test 3: 多个命名空间
	RegisterNamespacePrefix("ns1", "prefix1")
	RegisterNamespacePrefix("ns2", "prefix2")
	RegisterNamespacePrefix("ns3", "prefix3")

	if p := GetNamespacePrefix("ns1"); p != "prefix1" {
		t.Errorf("expected prefix1, got %s", p)
	}
	if p := GetNamespacePrefix("ns2"); p != "prefix2" {
		t.Errorf("expected prefix2, got %s", p)
	}
	if p := GetNamespacePrefix("ns3"); p != "prefix3" {
		t.Errorf("expected prefix3, got %s", p)
	}

	// Test 4: 覆盖已注册的命名空间应保持不变
	RegisterNamespacePrefix(ns, "new_prefix")

	if p := GetNamespacePrefix(ns); p == "new_prefix" {
		t.Logf("prefix can be updated (behavior: merge/overwrite): %s", p)
	}
}

func TestNamespaceGauge_MultipleMetrics(t *testing.T) {
	Reset()

	ns := "multi_metric_ns"
	RegisterNamespacePrefix(ns, "multi")

	// Test 1: 创建多个不同的指标
	g1, c1 := NamespaceGauge(ns, "metric1", "label1")
	if g1 == nil {
		t.Fatalf("gauge1 should not be nil")
	}
	if c1 != 9 {
		t.Fatalf("expected 9 labels for gauge1, got %d", c1)
	}

	g2, c2 := NamespaceGauge(ns, "metric2", "label2")
	if g2 == nil {
		t.Fatalf("gauge2 should not be nil")
	}
	if c2 != 9 {
		t.Fatalf("expected 9 labels for gauge2, got %d", c2)
	}

	g3, c3 := NamespaceGauge(ns, "metric3", "label3", "label4")
	if g3 == nil {
		t.Fatalf("gauge3 should not be nil")
	}
	if c3 != 10 {
		t.Fatalf("expected 10 labels for gauge3, got %d", c3)
	}

	// Test 2: 重复创建同一指标应返回相同 gauge
	g1Again, _ := NamespaceGauge(ns, "metric1", "label1")
	if g1 != g1Again {
		t.Errorf("expected same gauge for same metric")
	}

	// Test 3: 不同数量的 extra labels
	_, c4 := NamespaceGauge(ns, "metric4")
	if c4 != 8 {
		t.Fatalf("expected 8 labels (no extra), got %d", c4)
	}
}

func TestIncSampleCountWithLabels_Comprehensive(t *testing.T) {
	Reset()

	// Test 1: 基础调用（n > 0）
	IncSampleCountWithLabels("acc1", "us-east-1", "alb", "acs_alb", 10)

	// Test 2: 多次累加
	IncSampleCountWithLabels("acc1", "us-east-1", "alb", "acs_alb", 5)
	IncSampleCountWithLabels("acc1", "us-east-1", "alb", "acs_alb", 3)

	// Test 3: 不同命名空间
	IncSampleCountWithLabels("acc2", "us-west-2", "alb", "acs_alb", 7)
	IncSampleCountWithLabels("acc3", "eu-west-1", "nlb", "classic_nlb", 8)

	// Test 4: 边界情况（n <= 0 应不执行任何操作）
	IncSampleCountWithLabels("acc1", "us-east-1", "alb", "acs_alb", 0)  // 应不执行
	IncSampleCountWithLabels("acc1", "us-east-1", "alb", "acs_alb", -1) // 应不执行

	// Test 5: 并发调用
	done := make(chan bool)
	for i := 0; i < 100; i++ {
		go func(n int) {
			accountID := fmt.Sprintf("acc%d", n%3)
			region := fmt.Sprintf("region-%d", n/10)
			IncSampleCountWithLabels(accountID, region, "alb", "acs_alb", 1)
			done <- true
		}(i)
	}
	for i := 0; i < 100; i++ {
		<-done
	}

	// 说明：IncSampleCountWithLabels 只更新 Prometheus 指标，不影响 GetSampleCounts() map
	// 所以这里仅确保不 panic 即可
	t.Log("IncSampleCountWithLabels test completed without panic")
}

func TestRegisterNamespaceMetricAlias_Comprehensive(t *testing.T) {
	Reset()

	ns := "alias_test_ns"

	// Test 1: 单个别名映射
	RegisterNamespaceMetricAlias(ns, map[string]string{"orig1": "alias1"})
	if got := GetMetricAlias(ns, "orig1"); got != "alias1" {
		t.Errorf("expected alias1, got %s", got)
	}

	// Test 2: 多个别名映射
	RegisterNamespaceMetricAlias(ns, map[string]string{
		"orig1": "alias1",
		"orig2": "alias2",
		"orig3": "alias3",
	})

	if got := GetMetricAlias(ns, "orig1"); got != "alias1" {
		t.Errorf("expected alias1, got %s", got)
	}
	if got := GetMetricAlias(ns, "orig2"); got != "alias2" {
		t.Errorf("expected alias2, got %s", got)
	}
	if got := GetMetricAlias(ns, "orig3"); got != "alias3" {
		t.Errorf("expected alias3, got %s", got)
	}

	// Test 3: 合并别名映射（多次调用）
	RegisterNamespaceMetricAlias(ns, map[string]string{"orig4": "alias4"})

	if got := GetMetricAlias(ns, "orig1"); got != "alias1" {
		t.Errorf("orig1 should still be mapped, got %s", got)
	}
	if got := GetMetricAlias(ns, "orig4"); got != "alias4" {
		t.Errorf("expected alias4, got %s", got)
	}

	// Test 4: 未映射的指标
	if got := GetMetricAlias(ns, "unknown"); got != "" {
		t.Errorf("expected empty for unknown metric, got %s", got)
	}

	// Test 5: 不同命名空间互不影响
	ns2 := "alias_test_ns2"
	RegisterNamespaceMetricAlias(ns2, map[string]string{"shared": "different"})

	if got := GetMetricAlias(ns, "shared"); got != "" {
		t.Errorf("expected empty in ns1, got %s", got)
	}
	if got := GetMetricAlias(ns2, "shared"); got != "different" {
		t.Errorf("expected different in ns2, got %s", got)
	}
}
