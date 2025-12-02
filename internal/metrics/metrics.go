// 指标包：统一定义并暴露多云资源的 GaugeVec 指标
package metrics

import (
	"strings"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

// ResourceMetric 统一的资源指标，标签包含云、账号、区域、资源、ID、指标名
var (
	ResourceMetric = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "multicloud_resource_metric",
			Help: "Multi-cloud resource metrics",
		},
		[]string{"cloud_provider", "account_id", "region", "resource_type", "resource_id", "metric_name"},
	)
	RequestTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "multicloud_request_total",
			Help: "Total number of cloud API requests",
		},
		[]string{"cloud_provider", "api", "status"},
	)
	RequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "multicloud_request_duration_seconds",
			Help:    "Cloud API request duration",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"cloud_provider", "api"},
	)
	NamespaceMetric = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "multicloud_namespace_metric",
			Help: "Namespace-scoped metrics for cloud products",
		},
		[]string{"cloud_provider", "account_id", "region", "namespace", "resource_type", "resource_id", "metric_name"},
	)
)

var (
	nsGaugesMu sync.Mutex
	nsGauges   = make(map[string]*prometheus.GaugeVec)
)

func sanitizeName(name string) string {
	n := strings.ToLower(name)
	n = strings.ReplaceAll(n, "-", "_")
	n = strings.ReplaceAll(n, ".", "_")
	return n
}

func NamespaceGauge(namespace, metric string) *prometheus.GaugeVec {
	key := namespace + "|" + metric
	nsGaugesMu.Lock()
	defer nsGaugesMu.Unlock()
	if g, ok := nsGauges[key]; ok {
		return g
	}
	name := sanitizeName(namespace + "_" + metric)
	g := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: name,
			Help: "Cloud product metric",
		},
		[]string{"cloud_provider", "account_id", "region", "resource_type", "resource_id"},
	)
	prometheus.MustRegister(g)
	nsGauges[key] = g
	return g
}
