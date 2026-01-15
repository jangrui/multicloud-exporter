package main

import (
	"multicloud-exporter/internal/metrics"

	"github.com/prometheus/client_golang/prometheus"
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
	prometheus.MustRegister(metrics.RegionDiscoveryStatus)
	prometheus.MustRegister(metrics.RegionDiscoveryDuration)
	prometheus.MustRegister(metrics.RegionSkippedTotal)
	prometheus.MustRegister(metrics.RegionRediscoveryTotal)
	prometheus.MustRegister(metrics.RegionRediscoveryDuration)
	prometheus.MustRegister(metrics.RegionRediscoveryMarkedTotal)
	// 集群配置相关指标
	prometheus.MustRegister(metrics.ClusterConfigRefreshTotal)
	prometheus.MustRegister(metrics.ClusterConfigRefreshDuration)
	prometheus.MustRegister(metrics.ClusterConfigTotal)
	prometheus.MustRegister(metrics.ClusterConfigIndex)
	// 首次采集延迟指标
	prometheus.MustRegister(metrics.FirstRunDelaySeconds)
	// 缓存命中率指标
	prometheus.MustRegister(metrics.CacheHitRatio)
}
