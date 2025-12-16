// 指标包：统一定义并暴露多云资源的 GaugeVec 指标
package metrics

import (
	"fmt"
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
	nsGauges   = make(map[string]gaugeInfo)
)

type gaugeInfo struct {
	vec   *prometheus.GaugeVec
	count int
}

var (
	prefixByNamespace = make(map[string]string)
	aliasByNamespace  = make(map[string]map[string]string)
	helpByNamespace   = make(map[string]func(string) string)
	aliasFuncByNS     = make(map[string]func(string) string)
	scaleByNamespace  = make(map[string]map[string]float64)
)

var (
	sampleCountsMu sync.Mutex
	sampleCounts   = make(map[string]int)
)

func RegisterNamespacePrefix(namespace, prefix string) {
	prefixByNamespace[namespace] = prefix
}

func RegisterNamespaceMetricAlias(namespace string, aliases map[string]string) {
	if aliasByNamespace[namespace] == nil {
		aliasByNamespace[namespace] = make(map[string]string)
	}
	// 合并映射而不是覆盖，允许后续注册补充新的映射
	for k, v := range aliases {
		aliasByNamespace[namespace][k] = v
	}
}

func RegisterNamespaceMetricScale(namespace string, scales map[string]float64) {
	if scaleByNamespace[namespace] == nil {
		scaleByNamespace[namespace] = make(map[string]float64)
	}
	// 合并缩放因子而不是覆盖，允许后续注册补充新的缩放因子
	for k, v := range scales {
		scaleByNamespace[namespace][k] = v
	}
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

func GetMetricAlias(namespace, metric string) string {
	return aliasMetricForNamespace(namespace, metric)
}

// GetNamespacePrefix 返回命名空间的统一前缀（用于 resource_type）
func GetNamespacePrefix(namespace string) string {
	return aliasPrefixForNamespace(namespace)
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
	n = strings.ReplaceAll(n, "/", "_") // Replace slash with underscore
	return n
}

func NamespaceGauge(namespace, metric string, extraLabels ...string) (*prometheus.GaugeVec, int) {
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
	if info, ok := nsGauges[key]; ok {
		return info.vec, info.count
	}
	help := metricHelpForNamespace(namespace, useMetric)
	// 统一命名空间指标的标签集合：
	// cloud_provider, account_id, region, resource_type, resource_id, namespace, metric_name, code_name
	// 加上动态维度标签
	labels := []string{"cloud_provider", "account_id", "region", "resource_type", "resource_id", "namespace", "metric_name", "code_name"}
	seen := make(map[string]bool)
	for _, l := range labels {
		seen[l] = true
	}
	for _, l := range extraLabels {
		sanitized := sanitizeName(l)
		base := sanitized
		idx := 2
		for seen[sanitized] {
			sanitized = fmt.Sprintf("%s_%d", base, idx)
			idx++
		}
		seen[sanitized] = true
		labels = append(labels, sanitized)
	}

	g := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: name,
			Help: help,
		},
		labels,
	)
	if err := prometheus.Register(g); err != nil {
		if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
			// If already registered, use the existing one
			if existingVec, ok := are.ExistingCollector.(*prometheus.GaugeVec); ok {
				nsGauges[key] = gaugeInfo{vec: existingVec, count: len(labels)}
				return existingVec, len(labels)
			}
		}
		// Log error and return the unregistered gauge (it won't be scraped but won't crash)
		fmt.Printf("Failed to register metric: name=%q labels=%v err=%v. Returning unregistered gauge.\n", name, labels, err)
		// We still return g, but it's not registered.
		// We also cache it so we don't try to register again and log error every time.
		nsGauges[key] = gaugeInfo{vec: g, count: len(labels)}
		return g, len(labels)
	}
	nsGauges[key] = gaugeInfo{vec: g, count: len(labels)}
	return g, len(labels)
}

func IncSampleCount(namespace string, n int) {
	if n <= 0 {
		return
	}
	sampleCountsMu.Lock()
	sampleCounts[namespace] += n
	sampleCountsMu.Unlock()
}

func ResetSampleCounts() {
	sampleCountsMu.Lock()
	sampleCounts = make(map[string]int)
	sampleCountsMu.Unlock()
}

func GetSampleCounts() map[string]int {
	sampleCountsMu.Lock()
	defer sampleCountsMu.Unlock()
	out := make(map[string]int, len(sampleCounts))
	for k, v := range sampleCounts {
		out[k] = v
	}
	return out
}

func aliasMetricForNamespace(namespace, metric string) string {
	if m, ok := aliasByNamespace[namespace]; ok {
		if a, ok2 := m[metric]; ok2 {
			return a
		}
	}
	if fn, ok := aliasFuncByNS[namespace]; ok {
		return fn(metric)
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
	for _, info := range nsGauges {
		info.vec.Reset()
	}
}
