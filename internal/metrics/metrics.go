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
	RateLimitTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "multicloud_rate_limit_total",
			Help: " - 云 API 限流次数统计",
		},
		[]string{"cloud_provider", "api"},
	)
	CollectionDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "multicloud_collection_duration_seconds",
			Help:    " - 采集周期总耗时（秒）",
			Buckets: prometheus.DefBuckets,
		},
	)
)

var (
	nsGaugesMu sync.Mutex
	nsGauges   = make(map[string]*prometheus.GaugeVec)
)

var (
	prefixByNamespace = make(map[string]string)
	aliasByNamespace  = make(map[string]map[string]string)
	helpByNamespace   = make(map[string]func(string) string)
	aliasFuncByNS     = make(map[string]func(string) string)
	scaleByNamespace  = make(map[string]map[string]float64)
)

func RegisterNamespacePrefix(namespace, prefix string) {
	prefixByNamespace[namespace] = prefix
}

func RegisterNamespaceMetricAlias(namespace string, aliases map[string]string) {
	aliasByNamespace[namespace] = aliases
}

func RegisterNamespaceMetricScale(namespace string, scales map[string]float64) {
	scaleByNamespace[namespace] = scales
}

func RegisterNamespaceHelp(namespace string, help func(string) string) {
	helpByNamespace[namespace] = help
}

func RegisterNamespaceAliasFunc(namespace string, fn func(string) string) {
	aliasFuncByNS[namespace] = fn
}

func GetMetricScale(namespace, metric string) float64 {
	if scales, ok := scaleByNamespace[namespace]; ok {
		if s, ok := scales[metric]; ok {
			return s
		}
	}
	return 1.0
}

func aliasPrefixForNamespace(namespace string) string {
	if p, ok := prefixByNamespace[namespace]; ok {
		return p
	}
	return ""
}

func sanitizeName(name string) string {
	n := strings.ToLower(name)
	n = strings.ReplaceAll(n, "-", "_")
	n = strings.ReplaceAll(n, ".", "_")
	return n
}

func NamespaceGauge(namespace, metric string, extraLabels ...string) *prometheus.GaugeVec {
	nsGaugesMu.Lock()
	defer nsGaugesMu.Unlock()
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
	key := name
	if g, ok := nsGauges[key]; ok {
		return g
	}
	help := metricHelpForNamespace(namespace, useMetric)
	// 统一命名空间指标的标签集合：
	// cloud_provider, account_id, region, resource_type, resource_id, namespace, metric_name, code_name
	// 加上动态维度标签
	labels := []string{"cloud_provider", "account_id", "region", "resource_type", "resource_id", "namespace", "metric_name", "code_name"}
	labels = append(labels, extraLabels...)

	g := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: name,
			Help: help,
		},
		labels,
	)
	prometheus.MustRegister(g)
	nsGauges[key] = g
	return g
}

func aliasMetricForNamespace(namespace, metric string) string {
	if fn, ok := aliasFuncByNS[namespace]; ok {
		return fn(metric)
	}
	if m, ok := aliasByNamespace[namespace]; ok {
		if a, ok2 := m[metric]; ok2 {
			return a
		}
	}
	return ""
}

func metricHelpForNamespace(namespace, metric string) string {
	if h, ok := helpByNamespace[namespace]; ok {
		return h(metric)
	}
	return " - 云产品指标"
}

// Reset 重置所有 Gauge 指标，用于清理过期 Series
func Reset() {
	ResourceMetric.Reset()
	NamespaceMetric.Reset()
	nsGaugesMu.Lock()
	defer nsGaugesMu.Unlock()
	for _, g := range nsGauges {
		g.Reset()
	}
}
