package metrics

import (
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
