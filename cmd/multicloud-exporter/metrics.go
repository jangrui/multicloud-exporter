package main

import (
	"fmt"
	"strings"

	"multicloud-exporter/internal/metrics"

	"github.com/prometheus/client_golang/prometheus"
)

// registerPrometheusMetrics 注册所有 Prometheus 指标
func registerPrometheusMetrics(reg prometheus.Registerer) error {
	collectors := []prometheus.Collector{
		metrics.ResourceMetric,
		metrics.RequestTotal,
		metrics.RequestDuration,
		metrics.NamespaceMetric,
		metrics.RateLimitTotal,
		metrics.CollectionDuration,
		metrics.CacheSizeBytes,
		metrics.CacheEntriesTotal,
		metrics.RegionDiscoveryStatus,
		metrics.RegionDiscoveryDuration,
		metrics.RegionSkippedTotal,
		metrics.RegionRediscoveryTotal,
		metrics.RegionRediscoveryDuration,
		metrics.RegionRediscoveryMarkedTotal,
		metrics.AccountStatusTotal,
		metrics.AccountSkipTotal,
		metrics.AccountDegradedTotal,
		metrics.AccountStatusChange,
		metrics.ProductStatusTotal,
		metrics.ProductSkipTotal,
		metrics.ProductDegradedTotal,
		metrics.RegionStatusTotal,
		metrics.RegionSkipTotal,
		metrics.RegionDegradedTotal,
		metrics.ResourceStatusTotal,
		metrics.ResourceSkipTotal,
		metrics.ResourceDegradedTotal,
		metrics.FourDimensionSyncTotal,
		metrics.FourDimensionSyncDurationSeconds,
		metrics.ClusterConfigRefreshTotal,
		metrics.ClusterConfigRefreshDuration,
		metrics.ClusterConfigTotal,
		metrics.ClusterConfigIndex,
		metrics.FirstRunDelaySeconds,
		metrics.ClusterStabilityCheckTotal,
		metrics.ClusterHeadlessJoined,
		metrics.CollectionSkippedTotal,
		metrics.CacheHitRatio,
	}

	for _, c := range collectors {
		if err := reg.Register(c); err != nil {
			if _, ok := err.(prometheus.AlreadyRegisteredError); ok {
				continue
			}
			return fmt.Errorf("register prometheus collector: %w", err)
		}
	}

	metrics.AccountStatusTotal.WithLabelValues("", "", "active").Set(0)
	metrics.ProductStatusTotal.WithLabelValues("", "", "active").Set(0)
	metrics.RegionStatusTotal.WithLabelValues("", "", "", "active").Set(0)
	metrics.ResourceStatusTotal.WithLabelValues("", "", "", "", "active").Set(0)
	metrics.RegionDiscoveryStatus.WithLabelValues("", "", "unknown").Set(0)

	return nil
}

func verifyCriticalMetricFamilies(g prometheus.Gatherer) error {
	mfs, err := g.Gather()
	if err != nil {
		return fmt.Errorf("gather prometheus metrics: %w", err)
	}

	required := []string{
		"multicloud_account_status_total",
		"multicloud_product_status_total",
		"multicloud_region_status_total",
		"multicloud_resource_status_total",
		"multicloud_region_discovery_status_total",
	}

	present := make(map[string]bool, len(mfs))
	for _, mf := range mfs {
		if mf != nil && mf.Name != nil {
			present[*mf.Name] = true
		}
	}

	var missing []string
	for _, name := range required {
		if !present[name] {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing metric families: %s", strings.Join(missing, ", "))
	}

	return nil
}
