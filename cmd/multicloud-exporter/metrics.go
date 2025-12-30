package main

import (
	"github.com/prometheus/client_golang/prometheus"
	"multicloud-exporter/internal/metrics"
)

// registerPrometheusMetrics 注册所有 Prometheus 指标
func registerPrometheusMetrics() {
	prometheus.MustRegister(metrics.ResourceMetric)
	prometheus.MustRegister(metrics.RequestTotal)
	prometheus.MustRegister(metrics.RequestDuration)
	prometheus.MustRegister(metrics.NamespaceMetric)
	prometheus.MustRegister(metrics.RateLimitTotal)
	prometheus.MustRegister(metrics.CollectionDuration)
	prometheus.MustRegister(metrics.CacheSizeBytes)
	prometheus.MustRegister(metrics.CacheEntriesTotal)
}
