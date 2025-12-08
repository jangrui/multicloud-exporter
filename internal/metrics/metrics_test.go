package metrics

import "testing"

func TestSanitizeName(t *testing.T) {
    if sanitizeName("A-B.C") != "a_b_c" { t.Fatalf("sanitize") }
}

func TestNamespaceGauge(t *testing.T) {
    g := NamespaceGauge("ns", "met", "extra")
    if g == nil { t.Fatalf("gauge") }
    g = NamespaceGauge("ns", "met", "extra")
    if g == nil { t.Fatalf("reuse") }
}
