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

	// ========== 四层架构指标 ==========

	// AccountStatusTotal 账户状态总数
	AccountStatusTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "multicloud_account_status_total",
			Help: " - 账户状态统计（active/degraded/disabled）",
		},
		[]string{"account_id", "cloud_provider", "status"},
	)

	// AccountSkipTotal 跳过的账户次数
	AccountSkipTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "multicloud_account_skip_total",
			Help: " - 跳过的账户次数统计",
		},
		[]string{"account_id", "cloud_provider", "reason"},
	)

	// AccountDegradedTotal 降级的账户次数
	AccountDegradedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "multicloud_account_degraded_total",
			Help: " - 降级的账户次数统计",
		},
		[]string{"account_id", "cloud_provider", "reason"},
	)

	// AccountStatusChange 账户状态变更次数
	AccountStatusChange = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "multicloud_account_status_change",
			Help: " - 账户状态变更次数统计",
		},
		[]string{"account_id", "cloud_provider", "old_status", "new_status", "reason"},
	)

	// ProductStatusTotal 产品状态总数
	ProductStatusTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "multicloud_product_status_total",
			Help: " - 产品状态统计（active/degraded/disabled）",
		},
		[]string{"account_id", "product_id", "status"},
	)

	// ProductSkipTotal 跳过的产品次数
	ProductSkipTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "multicloud_product_skip_total",
			Help: " - 跳过的产品次数统计",
		},
		[]string{"account_id", "product_id", "reason"},
	)

	// ProductDegradedTotal 降级的产品次数
	ProductDegradedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "multicloud_product_degraded_total",
			Help: " - 降级的产品次数统计",
		},
		[]string{"account_id", "product_id", "reason"},
	)

	// RegionStatusTotal 区域状态总数
	RegionStatusTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "multicloud_region_status_total",
			Help: " - 区域状态统计（active/degraded/disabled）",
		},
		[]string{"account_id", "product_id", "region", "status"},
	)

	// RegionSkipTotal 跳过的区域次数
	RegionSkipTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "multicloud_region_skip_total",
			Help: " - 跳过的区域次数统计",
		},
		[]string{"account_id", "product_id", "region", "reason"},
	)

	// RegionDegradedTotal 降级的区域次数
	RegionDegradedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "multicloud_region_degraded_total",
			Help: " - 降级的区域次数统计",
		},
		[]string{"account_id", "product_id", "region", "reason"},
	)

	// ResourceStatusTotal 资源状态总数
	ResourceStatusTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "multicloud_resource_status_total",
			Help: " - 资源状态统计（active/degraded/disabled）",
		},
		[]string{"account_id", "product_id", "region", "resource_id", "status"},
	)

	// ResourceSkipTotal 跳过的资源次数
	ResourceSkipTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "multicloud_resource_skip_total",
			Help: " - 跳过的资源次数统计",
		},
		[]string{"account_id", "product_id", "region", "resource_id", "reason"},
	)

	// ResourceDegradedTotal 降级的资源次数
	ResourceDegradedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "multicloud_resource_degraded_total",
			Help: " - 降级的资源次数统计",
		},
		[]string{"account_id", "product_id", "region", "resource_id", "reason"},
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

// RecordAccessDuration 记录四维访问延迟
func RecordAccessDuration(dimension, accountID, productID, region, resourceID string, durationSeconds float64) {
	AccessDurationSeconds.WithLabelValues(dimension, accountID, productID, region, resourceID).Observe(durationSeconds)
}

// RecordAccess 记录四维访问次数
func RecordAccess(dimension, status string) {
	AccessTotal.WithLabelValues(dimension, status).Inc()
}

// RecordLockContention 记录锁竞争
func RecordLockContention(dimension string) {
	LockContentionTotal.WithLabelValues(dimension).Inc()
}

// UpdateMemoryUsage 更新内存占用
func UpdateMemoryUsage(dimension string, bytes int64) {
	MemoryUsageBytes.WithLabelValues(dimension).Set(float64(bytes))
}

// UpdateObjectPoolSize 更新对象池大小
func UpdateObjectPoolSize(poolType string, size float64) {
	ObjectPoolSize.WithLabelValues(poolType).Set(size)
}

// RecordObjectPoolHit 记录对象池命中
func RecordObjectPoolHit(poolType string) {
	ObjectPoolHitsTotal.WithLabelValues(poolType).Inc()
}

// RecordObjectPoolMiss 记录对象池未命中
func RecordObjectPoolMiss(poolType string) {
	ObjectPoolMissesTotal.WithLabelValues(poolType).Inc()
}

// RecordLRUEvicted 记录 LRU 驱逐
func RecordLRUEvicted(dimension string) {
	LRUEvictedTotal.WithLabelValues(dimension).Inc()
}

// RecordLRUCleanupDuration 记录 LRU 清理耗时
func RecordLRUCleanupDuration(dimension string, durationSeconds float64) {
	LRUCleanupDurationSeconds.WithLabelValues(dimension).Observe(durationSeconds)
}

// UpdateDegradedResources 更新降级资源数
func UpdateDegradedResources(dimension string, count float64) {
	DegradedResourcesTotal.WithLabelValues(dimension).Set(count)
}

// FourDimensionSyncTotal 四维集群同步总数
var FourDimensionSyncTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "multicloud_four_dimension_sync_total",
		Help: " - 四维集群同步次数统计（batch/single）",
	},
	[]string{"sync_type"},
)

// FourDimensionSyncDurationSeconds 四维集群同步耗时
var FourDimensionSyncDurationSeconds = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    "multicloud_four_dimension_sync_duration_seconds",
		Help:    " - 四维集群同步耗时（秒）",
		Buckets: prometheus.DefBuckets,
	},
	[]string{"dimension"},
)

// AccessDurationSeconds 四维访问延迟指标
var AccessDurationSeconds = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    "multicloud_access_duration_seconds",
		Help:    " - 四维访问延迟（秒）",
		Buckets: []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 5},
	},
	[]string{"dimension", "account_id", "product_id", "region", "resource_id"},
)

// AccessTotal 四维吞吐量指标
var AccessTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "multicloud_access_total",
		Help: " - 四维访问次数统计",
	},
	[]string{"dimension", "status"},
)

// LockContentionTotal 四维锁竞争指标
var LockContentionTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "multicloud_lock_contention_total",
		Help: " - 四维锁竞争次数统计",
	},
	[]string{"dimension"},
)

// MemoryUsageBytes 四维内存占用指标
var MemoryUsageBytes = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "multicloud_memory_usage_bytes",
		Help: " - 四维内存占用（字节）",
	},
	[]string{"dimension"},
)

// ObjectPoolSize 对象池大小指标
var ObjectPoolSize = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "multicloud_pool_size",
		Help: " - 对象池大小",
	},
	[]string{"pool_type"},
)

// ObjectPoolHitsTotal 对象池命中次数
var ObjectPoolHitsTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "multicloud_pool_hits_total",
		Help: " - 对象池命中次数统计",
	},
	[]string{"pool_type"},
)

// ObjectPoolMissesTotal 对象池未命中次数
var ObjectPoolMissesTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "multicloud_pool_misses_total",
		Help: " - 对象池未命中次数统计",
	},
	[]string{"pool_type"},
)

// LRUEvictedTotal LRU 清理驱逐次数
var LRUEvictedTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "multicloud_lru_evicted_total",
		Help: " - LRU 清理驱逐次数统计",
	},
	[]string{"dimension"},
)

// LRUCleanupDurationSeconds LRU 清理耗时
var LRUCleanupDurationSeconds = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    "multicloud_lru_duration_seconds",
		Help:    " - LRU 清理耗时（秒）",
		Buckets: prometheus.DefBuckets,
	},
	[]string{"dimension"},
)

// DegradedResourcesTotal 四维降级资源数
var DegradedResourcesTotal = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "multicloud_degraded_resources_total",
		Help: " - 降级资源总数",
	},
	[]string{"dimension"},
)

// DegradationTotal 降级总数
var DegradationTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "multicloud_degradation_total",
		Help: " - 降级次数统计（按维度和原因）",
	},
	[]string{"dimension", "reason"},
)

// DegradationRecoveredTotal 降级恢复总数
var DegradationRecoveredTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "multicloud_degradation_recovered_total",
		Help: " - 降级恢复次数统计（按维度）",
	},
	[]string{"dimension"},
)

// DegradationDurationSeconds 降级持续时间
var DegradationDurationSeconds = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    "multicloud_degradation_duration_seconds",
		Help:    " - 降级持续时间（秒）",
		Buckets: prometheus.DefBuckets,
	},
	[]string{"dimension"},
)

// MemoryAlertTotal 内存告警总数
var MemoryAlertTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "multicloud_memory_alert_total",
		Help: " - 内存告警次数统计（按级别）",
	},
	[]string{"level"},
)

// IsolatedResourcesTotal 隔离资源总数
var IsolatedResourcesTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "multicloud_isolated_resources_total",
		Help: " - 隔离资源总数统计（按维度和原因）",
	},
	[]string{"dimension", "reason"},
)

// RecoveredResourcesTotal 恢复资源总数
var RecoveredResourcesTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "multicloud_recovered_resources_total",
		Help: " - 恢复资源总数统计（按维度）",
	},
	[]string{"dimension"},
)

// CollectionStartTotal 采集启动总数
var CollectionStartTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "multicloud_collection_start_total",
		Help: " - 采集启动次数统计（按维度）",
	},
	[]string{"dimension", "key"},
)

// CollectionDurationSeconds 采集持续时间
var CollectionDurationSeconds = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    "multicloud_collection_duration_seconds",
		Help:    " - 采集持续时间（秒）",
		Buckets: prometheus.DefBuckets,
	},
	[]string{"dimension", "key"},
)
