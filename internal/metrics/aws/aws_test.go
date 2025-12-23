package aws

import (
	"testing"

	"multicloud-exporter/internal/metrics"
)

func TestNamespacePrefixes(t *testing.T) {
	if metrics.GetNamespacePrefix("AWS/ELB") != "clb" {
		t.Fatalf("prefix mismatch for AWS/ELB")
	}
	if metrics.GetNamespacePrefix("AWS/NetworkELB") != "nlb" {
		t.Fatalf("prefix mismatch for AWS/NetworkELB")
	}
	if metrics.GetNamespacePrefix("AWS/ApplicationELB") != "alb" {
		t.Fatalf("prefix mismatch for AWS/ApplicationELB")
	}
	if metrics.GetNamespacePrefix("AWS/GatewayELB") != "gwlb" {
		t.Fatalf("prefix mismatch for AWS/GatewayELB")
	}
}

func TestNamespaceGaugeLabels(t *testing.T) {
	_, count := metrics.NamespaceGauge("AWS/ELB", "RequestCount", "InstanceId", "Zone")
	if count < 8 {
		t.Fatalf("labels count too small: %d", count)
	}
	_, count2 := metrics.NamespaceGauge("AWS/NetworkELB", "ProcessedBytes", "TargetGroup", "Zone")
	if count2 < 8 {
		t.Fatalf("labels count too small: %d", count2)
	}
}
