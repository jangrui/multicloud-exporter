package main

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func TestRegisterPrometheusMetrics_NoConflicts(t *testing.T) {
	reg := prometheus.NewRegistry()
	if err := registerPrometheusMetrics(reg); err != nil {
		t.Fatalf("registerPrometheusMetrics: %v", err)
	}

	if err := verifyCriticalMetricFamilies(reg); err != nil {
		t.Fatalf("verifyCriticalMetricFamilies: %v", err)
	}
}
