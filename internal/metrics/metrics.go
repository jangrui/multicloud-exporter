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
			Help: " - 多云资源通用指标",
		},
		[]string{"cloud_provider", "account_id", "region", "resource_type", "resource_id", "metric_name"},
	)
	RequestTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "multicloud_request_total",
			Help: " - 云 API 请求次数统计",
		},
		[]string{"cloud_provider", "api", "status"},
	)
	RequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "multicloud_request_duration_seconds",
			Help:    " - 云 API 请求耗时（秒）",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"cloud_provider", "api"},
	)
	NamespaceMetric = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "multicloud_namespace_metric",
			Help: " - 云产品命名空间指标（统一命名）",
		},
		[]string{"cloud_provider", "account_id", "region", "namespace", "resource_type", "resource_id", "metric_name"},
	)
)

var (
	nsGaugesMu sync.Mutex
	nsGauges   = make(map[string]*prometheus.GaugeVec)
)

func aliasPrefixForNamespace(namespace string) string {
	switch namespace {
	case "acs_bandwidth_package":
		return "bwp"
	default:
		return ""
	}
}

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
	alias := aliasPrefixForNamespace(namespace)
	metricAlias := aliasMetricForNamespace(namespace, metric)
	useMetric := metric
	if metricAlias != "" {
		useMetric = metricAlias
	}
	var name string
	if alias != "" {
		name = sanitizeName(alias + "_" + useMetric)
	} else {
		name = sanitizeName(namespace + "_" + useMetric)
	}
	help := metricHelpForNamespace(namespace, useMetric)
	g := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: name,
			Help: help,
		},
		[]string{"cloud_provider", "account_id", "region", "resource_type", "resource_id", "namespace", "metric_name"},
	)
	prometheus.MustRegister(g)
	nsGauges[key] = g
	return g
}

func aliasMetricForNamespace(namespace, metric string) string {
	switch namespace {
	case "acs_bandwidth_package":
		switch metric {
		case "in_bandwidth_utilization":
			return "in_utilization_pct"
		case "out_bandwidth_utilization":
			return "out_utilization_pct"
		case "net_rx.rate":
			return "in_bps"
		case "net_tx.rate":
			return "out_bps"
		case "net_rx.Pkgs":
			return "in_pps"
		case "net_tx.Pkgs":
			return "out_pps"
		case "in_ratelimit_drop_pps":
			return "in_drop_pps"
		case "out_ratelimit_drop_pps":
			return "out_drop_pps"
		}
	}
	return ""
}

func metricHelpForNamespace(namespace, metric string) string {
	switch namespace {
	case "acs_bandwidth_package":
		switch metric {
		case "in_utilization_pct":
			return " - 共享带宽入方向带宽利用率（百分比）"
		case "out_utilization_pct":
			return " - 共享带宽出方向带宽利用率（百分比）"
		case "in_bps":
			return " - 共享带宽入方向带宽速率（bit/s）"
		case "out_bps":
			return " - 共享带宽出方向带宽速率（bit/s）"
		case "in_pps":
			return " - 共享带宽入方向包速率（包/秒）"
		case "out_pps":
			return " - 共享带宽出方向包速率（包/秒）"
		case "in_drop_pps":
			return " - 共享带宽入方向因限速丢弃包速率（包/秒）"
		case "out_drop_pps":
			return " - 共享带宽出方向因限速丢弃包速率（包/秒）"
		}
	}
	return " - 云产品指标"
}
