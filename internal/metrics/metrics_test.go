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
