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
	CacheSizeBytes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "multicloud_cache_size_bytes",
			Help: " - 缓存大小（字节）",
		},
		[]string{"cache_type"},
	)
	CacheEntriesTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "multicloud_cache_entries_total",
			Help: " - 缓存条目总数",
		},
		[]string{"cache_type"},
	)
	// RegionDiscovery 区域发现状态统计（扩展支持产品维度）
	RegionDiscoveryStatus = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "multicloud_region_status_total",
			Help: " - 区域状态统计（active/empty/unknown）",
		},
		[]string{"cloud_provider", "product", "status"},
	)
	// RegionDiscoveryDuration 区域发现耗时
	RegionDiscoveryDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "multicloud_region_discovery_duration_seconds",
			Help:    " - 区域发现耗时（秒）",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"cloud_provider"},
	)
	// RegionSkippedTotal 跳过的空区域次数（扩展支持产品维度）
	RegionSkippedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "multicloud_region_skip_total",
			Help: " - 跳过的空区域次数",
		},
		[]string{"cloud_provider", "product"},
	)
	// RegionRediscoveryTotal 区域重新发现触发次数
	RegionRediscoveryTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "multicloud_region_rediscovery_total",
			Help: " - 区域重新发现触发次数",
		},
		[]string{"reason"},
	)
	// RegionRediscoveryDuration 区域重新发现耗时
	RegionRediscoveryDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "multicloud_region_rediscovery_duration_seconds",
			Help:    " - 区域重新发现耗时（秒）",
			Buckets: prometheus.DefBuckets,
		},
	)
	// RegionRediscoveryMarkedTotal 区域重新发现标记的区域总数
	RegionRediscoveryMarkedTotal = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "multicloud_region_rediscovery_marked_total",
			Help: " - 最近一次重新发现标记的区域总数",
		},
	)
	// ClusterConfigRefreshTotal 集群配置刷新次数
	ClusterConfigRefreshTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "multicloud_cluster_config_refresh_total",
			Help: " - 集群配置刷新次数统计",
		},
	)
	// ClusterConfigRefreshDuration 集群配置刷新耗时
	ClusterConfigRefreshDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "multicloud_cluster_config_refresh_duration_seconds",
			Help:    " - 集群配置刷新耗时（秒）",
			Buckets: prometheus.DefBuckets,
		},
	)
	// ClusterConfigTotal 当前集群总 Pod 数
	ClusterConfigTotal = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "multicloud_cluster_config_total",
			Help: " - 当前集群总 Pod 数",
		},
	)
	// ClusterConfigIndex 当前 Pod 索引
	ClusterConfigIndex = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "multicloud_cluster_config_index",
			Help: " - 当前 Pod 在集群中的索引",
		},
	)
	// FirstRunDelaySeconds 首次采集延迟时间
	FirstRunDelaySeconds = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "multicloud_first_run_delay_seconds",
			Help: " - 首次采集延迟时间（秒）",
		},
		[]string{"pod_index", "strategy"},
	)
	// CacheHitRatio 缓存命中率
	CacheHitRatio = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "multicloud_cache_hit_ratio",
			Help: " - 缓存命中率（0-1）",
		},
		[]string{"cache_type"},
	)
	// SampleCountTotal 样本计数指标（维度化，支持多副本并发）
	SampleCountTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "multicloud_sample_count_total",
			Help: " - 采集样本总数（按账号、区域、资源类型、命名空间维度统计）",
		},
		[]string{"account_id", "region", "resource_type", "namespace"},
	)
	// CacheHitTotal 缓存命中次数统计
	CacheHitTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "multicloud_cache_hit_total",
			Help: " - 缓存命中次数统计",
		},
		[]string{"cache_type"},
	)
	// CacheMissTotal 缓存未命中次数统计
	CacheMissTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "multicloud_cache_miss_total",
			Help: " - 缓存未命中次数统计",
		},
		[]string{"cache_type"},
	)
	// BroadcastFailedTotal 集群广播失败次数统计
	BroadcastFailedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "multicloud_broadcast_failed_total",
			Help: " - 集群广播失败次数统计",
		},
		[]string{"peer"},
	)
	// RegionManagerMemoryBytes 区域管理器内存占用（按云厂商和产品维度）
	RegionManagerMemoryBytes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "multicloud_region_manager_memory_bytes",
			Help: " - 区域管理器内存占用（字节），按云厂商和产品维度统计",
		},
		[]string{"cloud_provider", "product"},
	)
	// RegionManagerProductsTotal 每个云厂商的产品数量
	RegionManagerProductsTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "multicloud_region_manager_products_total",
			Help: " - 每个云厂商的产品 RegionManager 数量",
		},
		[]string{"cloud_provider"},
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
		// 如果已存在映射，记录警告（用于调试）
		// 注意：不能使用 logger，因为会导致循环导入（logger -> config -> metrics）
		if existing, exists := aliasByNamespace[namespace][k]; exists && existing != v {
			fmt.Printf("WARNING: Metric alias conflict for namespace=%s metric=%s: existing=%s new=%s (new will override)\n", namespace, k, existing, v)
		}
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

	// 第一次检查：是否已在缓存中（持锁）
	nsGaugesMu.Lock()
	if info, ok := nsGauges[key]; ok {
		nsGaugesMu.Unlock()
		return info.vec, info.count
	}
	nsGaugesMu.Unlock()

	// 构建指标（不持锁）
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

	// 注册到 Prometheus（不持锁，避免死锁）
	registerErr := prometheus.Register(g)
	if registerErr != nil {
		if are, ok := registerErr.(prometheus.AlreadyRegisteredError); ok {
			// 已注册，使用已存在的 collector
			if existingVec, ok := are.ExistingCollector.(*prometheus.GaugeVec); ok {
				// 缓存已存在的 collector
				nsGaugesMu.Lock()
				// 再次检查，避免被其他 goroutine 抢先注册
				if info, exists := nsGauges[key]; exists {
					nsGaugesMu.Unlock()
					return info.vec, info.count
				}
				nsGauges[key] = gaugeInfo{vec: existingVec, count: len(labels)}
				nsGaugesMu.Unlock()
				return existingVec, len(labels)
			}
		}
		// 其他错误：记录并返回未注册的 gauge
		// 注意：不能使用 logger，因为会导致循环导入（logger -> config -> metrics）
		// 使用 fmt.Printf 作为 fallback，这些错误通常只在初始化阶段出现
		fmt.Printf("Failed to register metric: name=%q labels=%v err=%v. Returning unregistered gauge.\n", name, labels, registerErr)
	}

	// 注册成功或失败，都缓存 gauge（避免重复尝试注册）
	nsGaugesMu.Lock()
	// 最后一次检查，避免重复写入
	if info, exists := nsGauges[key]; exists {
		nsGaugesMu.Unlock()
		return info.vec, info.count
	}
	nsGauges[key] = gaugeInfo{vec: g, count: len(labels)}
	nsGaugesMu.Unlock()
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

// IncSampleCountWithLabels 增加维度化的样本计数（推荐使用，支持多副本并发）
func IncSampleCountWithLabels(accountID, region, resourceType, namespace string, n int) {
	if n <= 0 {
		return
	}
	SampleCountTotal.WithLabelValues(accountID, region, resourceType, namespace).Add(float64(n))
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

// UpdateCacheMetrics 更新缓存监控指标
func UpdateCacheMetrics(cacheType string, sizeBytes int64, entries int) {
	CacheSizeBytes.WithLabelValues(cacheType).Set(float64(sizeBytes))
	CacheEntriesTotal.WithLabelValues(cacheType).Set(float64(entries))
}

// RecordCacheHit 记录缓存命中
func RecordCacheHit(cacheType string) {
	CacheHitTotal.WithLabelValues(cacheType).Inc()
}

// RecordCacheMiss 记录缓存未命中
func RecordCacheMiss(cacheType string) {
	CacheMissTotal.WithLabelValues(cacheType).Inc()
}
