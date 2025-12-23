package config

import (
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
	if m.Canonical["new_connection"].AWS.Metric != "NewConnectionCount" {
		t.Fatalf("ALB new_connection aws metric mismatch: %q", m.Canonical["new_connection"].AWS.Metric)
	}
	if m.Canonical["active_connection"].AWS.Metric != "ActiveConnectionCount" {
		t.Fatalf("ALB active_connection aws metric mismatch: %q", m.Canonical["active_connection"].AWS.Metric)
	}
}
