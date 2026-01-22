package memory

import (
	"runtime"
	"sync"
	"time"

	"multicloud-exporter/internal/metrics"
)

// MemoryStatus 内存状态
type MemoryStatus int

const (
	MemoryStatusNormal   MemoryStatus = iota // 正常
	MemoryStatusWarning                      // 警告（超过 70%）
	MemoryStatusCritical                     // 严重（超过 90%）
)

func (s MemoryStatus) String() string {
	return [...]string{"normal", "warning", "critical"}[s]
}

// MemoryEntry 内存条目
type MemoryEntry struct {
	Dimension   string    `json:"dimension"`    // 维度（account/product/region/resource）
	Key         string    `json:"key"`          // 唯一标识
	Bytes       int64     `json:"bytes"`        // 占用字节数
	CreatedAt   time.Time `json:"created_at"`   // 创建时间
	LastUsedAt  time.Time `json:"last_used_at"` // 最后使用时间
	AccessCount int       `json:"access_count"` // 访问次数
	IsExpired   bool      `json:"is_expired"`   // 是否过期
}

// MemoryStats 内存统计
type MemoryStats struct {
	TotalBytes      int64     `json:"total_bytes"`       // 总字节数
	EntryCount      int       `json:"entry_count"`       // 条目数量
	ExpiredCount    int       `json:"expired_count"`     // 过期条目数
	EvictedCount    int       `json:"evicted_count"`     // 驱逐条目数
	CleanupCount    int       `json:"cleanup_count"`     // 清理次数
	LastCleanupTime time.Time `json:"last_cleanup_time"` // 最后清理时间
}

// AccountMemory 账号级内存统计
type AccountMemory struct {
	AccountID string                  `json:"account_id"`
	Stats     MemoryStats             `json:"stats"`
	Entries   map[string]*MemoryEntry `json:"entries"`
}

// ProductMemory 产品级内存统计
type ProductMemory struct {
	ProductID string                  `json:"product_id"`
	Stats     MemoryStats             `json:"stats"`
	Entries   map[string]*MemoryEntry `json:"entries"`
}

// RegionMemory 区域级内存统计
type RegionMemory struct {
	RegionID string                  `json:"region_id"`
	Stats    MemoryStats             `json:"stats"`
	Entries  map[string]*MemoryEntry `json:"entries"`
}

// ResourceMemory 资源级内存统计
type ResourceMemory struct {
	ResourceID string                  `json:"resource_id"`
	Stats      MemoryStats             `json:"stats"`
	Entries    map[string]*MemoryEntry `json:"entries"`
}

// MemoryLimitConfig 内存限制配置
type MemoryLimitConfig struct {
	AccountLimit  int64         `json:"account_limit"`  // 账号级内存限制（字节）
	ProductLimit  int64         `json:"product_limit"`  // 产品级内存限制（字节）
	RegionLimit   int64         `json:"region_limit"`   // 区域级内存限制（字节）
	ResourceLimit int64         `json:"resource_limit"` // 资源级内存限制（字节）
	TTL           time.Duration `json:"ttl"`            // 条目生存时间
}

// DefaultMemoryLimitConfig 默认内存限制配置
func DefaultMemoryLimitConfig() MemoryLimitConfig {
	return MemoryLimitConfig{
		AccountLimit:  100 * 1024 * 1024, // 100 MB
		ProductLimit:  50 * 1024 * 1024,  // 50 MB
		RegionLimit:   20 * 1024 * 1024,  // 20 MB
		ResourceLimit: 10 * 1024 * 1024,  // 10 MB
		TTL:           30 * time.Minute,  // 30 分钟
	}
}

// CleanupStrategy 清理策略
type CleanupStrategy int

const (
	CleanupStrategyLRU   CleanupStrategy = iota // LRU 清理
	CleanupStrategyTTL                          // TTL 清理
	CleanupStrategyForce                        // 强制清理
)

func (s CleanupStrategy) String() string {
	return [...]string{"lru", "ttl", "force"}[s]
}

// MemoryCleanupConfig 内存清理配置
type MemoryCleanupConfig struct {
	WarningThreshold  float64       `json:"warning_threshold"`   // 警告阈值（0-1）
	CriticalThreshold float64       `json:"critical_threshold"`  // 严重阈值（0-1）
	CleanupInterval   time.Duration `json:"cleanup_interval"`    // 清理间隔
	ForceCleanupRatio float64       `json:"force_cleanup_ratio"` // 强制清理比例（0-1）
}

// DefaultMemoryCleanupConfig 默认内存清理配置
func DefaultMemoryCleanupConfig() MemoryCleanupConfig {
	return MemoryCleanupConfig{
		WarningThreshold:  0.7, // 70%
		CriticalThreshold: 0.9, // 90%
		CleanupInterval:   1 * time.Minute,
		ForceCleanupRatio: 0.5, // 清理 50%
	}
}

// MemoryManagerConfig 内存管理器配置
type MemoryManagerConfig struct {
	Limit   MemoryLimitConfig   `json:"limit"`
	Cleanup MemoryCleanupConfig `json:"cleanup"`
}

// DefaultMemoryManagerConfig 默认内存管理器配置
func DefaultMemoryManagerConfig() MemoryManagerConfig {
	return MemoryManagerConfig{
		Limit:   DefaultMemoryLimitConfig(),
		Cleanup: DefaultMemoryCleanupConfig(),
	}
}

// FourDimensionMemoryManager 四维内存管理器
type FourDimensionMemoryManager struct {
	mu sync.RWMutex

	config MemoryManagerConfig

	// 四维内存统计
	accounts  map[string]*AccountMemory
	products  map[string]*ProductMemory
	regions   map[string]*RegionMemory
	resources map[string]*ResourceMemory

	// 总内存统计
	totalStats MemoryStats

	// 状态
	status MemoryStatus

	// 停止信号
	stopChan chan struct{}
}

// NewFourDimensionMemoryManager 创建四维内存管理器
func NewFourDimensionMemoryManager(config MemoryManagerConfig) *FourDimensionMemoryManager {
	if config.Limit.TTL == 0 {
		config.Limit = DefaultMemoryLimitConfig()
	}
	if config.Cleanup.CleanupInterval == 0 {
		config.Cleanup = DefaultMemoryCleanupConfig()
	}

	return &FourDimensionMemoryManager{
		config:    config,
		accounts:  make(map[string]*AccountMemory),
		products:  make(map[string]*ProductMemory),
		regions:   make(map[string]*RegionMemory),
		resources: make(map[string]*ResourceMemory),
		status:    MemoryStatusNormal,
		stopChan:  make(chan struct{}),
	}
}

// StartMemoryManager 启动内存管理器（启动后台清理 goroutine）
func (m *FourDimensionMemoryManager) StartMemoryManager() {
	go m.cleanupLoop()
}

// StopMemoryManager 停止内存管理器
func (m *FourDimensionMemoryManager) StopMemoryManager() {
	close(m.stopChan)
}

// RecordMemoryUsage 记录内存使用
func (m *FourDimensionMemoryManager) RecordMemoryUsage(dimension, key string, bytes int64) {
	entry := &MemoryEntry{
		Dimension:   dimension,
		Key:         key,
		Bytes:       bytes,
		CreatedAt:   time.Now(),
		LastUsedAt:  time.Now(),
		AccessCount: 1,
		IsExpired:   false,
	}

	m.mu.Lock()

	switch dimension {
	case "account":
		// 账号维度：所有条目存储在 "account" 根目录下
		if _, ok := m.accounts["account"]; !ok {
			m.accounts["account"] = &AccountMemory{
				AccountID: "account",
				Entries:   make(map[string]*MemoryEntry),
			}
		}
		m.accounts["account"].Entries[key] = entry
		m.accounts["account"].Stats.TotalBytes += bytes
		m.accounts["account"].Stats.EntryCount++

	case "product":
		// 产品维度：所有条目存储在 "product" 根目录下
		if _, ok := m.products["product"]; !ok {
			m.products["product"] = &ProductMemory{
				ProductID: "product",
				Entries:   make(map[string]*MemoryEntry),
			}
		}
		m.products["product"].Entries[key] = entry
		m.products["product"].Stats.TotalBytes += bytes
		m.products["product"].Stats.EntryCount++

	case "region":
		// 区域维度：所有条目存储在 "region" 根目录下
		if _, ok := m.regions["region"]; !ok {
			m.regions["region"] = &RegionMemory{
				RegionID: "region",
				Entries:  make(map[string]*MemoryEntry),
			}
		}
		m.regions["region"].Entries[key] = entry
		m.regions["region"].Stats.TotalBytes += bytes
		m.regions["region"].Stats.EntryCount++

	case "resource":
		// 资源维度：所有条目存储在 "resource" 根目录下
		if _, ok := m.resources["resource"]; !ok {
			m.resources["resource"] = &ResourceMemory{
				ResourceID: "resource",
				Entries:    make(map[string]*MemoryEntry),
			}
		}
		m.resources["resource"].Entries[key] = entry
		m.resources["resource"].Stats.TotalBytes += bytes
		m.resources["resource"].Stats.EntryCount++
	}

	m.totalStats.TotalBytes += bytes
	m.totalStats.EntryCount++

	m.mu.Unlock()

	// 更新指标（在锁外调用以避免死锁）
	m.updateMetrics()
	m.checkMemoryStatus()
}

// ScanMemory 扫描内存使用情况
func (m *FourDimensionMemoryManager) ScanMemory() MemoryStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := m.totalStats
	stats.LastCleanupTime = time.Now()

	return stats
}

// GetMemoryUsage 获取当前内存占用
func (m *FourDimensionMemoryManager) GetMemoryUsage(dimension string) MemoryStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// 直接返回维度的统计信息
	switch dimension {
	case "account":
		if len(m.accounts) == 0 {
			return MemoryStats{}
		}
		var total MemoryStats
		for _, acc := range m.accounts {
			total.TotalBytes += acc.Stats.TotalBytes
			total.EntryCount += acc.Stats.EntryCount
			total.ExpiredCount += acc.Stats.ExpiredCount
			total.EvictedCount += acc.Stats.EvictedCount
			total.CleanupCount += acc.Stats.CleanupCount
		}
		return total

	case "product":
		if len(m.products) == 0 {
			return MemoryStats{}
		}
		var total MemoryStats
		for _, prod := range m.products {
			total.TotalBytes += prod.Stats.TotalBytes
			total.EntryCount += prod.Stats.EntryCount
			total.ExpiredCount += prod.Stats.ExpiredCount
			total.EvictedCount += prod.Stats.EvictedCount
			total.CleanupCount += prod.Stats.CleanupCount
		}
		return total

	case "region":
		if len(m.regions) == 0 {
			return MemoryStats{}
		}
		var total MemoryStats
		for _, reg := range m.regions {
			total.TotalBytes += reg.Stats.TotalBytes
			total.EntryCount += reg.Stats.EntryCount
			total.ExpiredCount += reg.Stats.ExpiredCount
			total.EvictedCount += reg.Stats.EvictedCount
			total.CleanupCount += reg.Stats.CleanupCount
		}
		return total

	case "resource":
		if len(m.resources) == 0 {
			return MemoryStats{}
		}
		var total MemoryStats
		for _, res := range m.resources {
			total.TotalBytes += res.Stats.TotalBytes
			total.EntryCount += res.Stats.EntryCount
			total.ExpiredCount += res.Stats.ExpiredCount
			total.EvictedCount += res.Stats.EvictedCount
			total.CleanupCount += res.Stats.CleanupCount
		}
		return total

	default:
		return m.totalStats
	}
}

// SetMemoryLimit 设置内存限制
func (m *FourDimensionMemoryManager) SetMemoryLimit(dimension string, limit int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	switch dimension {
	case "account":
		m.config.Limit.AccountLimit = limit
	case "product":
		m.config.Limit.ProductLimit = limit
	case "region":
		m.config.Limit.RegionLimit = limit
	case "resource":
		m.config.Limit.ResourceLimit = limit
	}
}

// TriggerCleanup 触发内存清理
func (m *FourDimensionMemoryManager) TriggerCleanup(strategy CleanupStrategy) int {
	var cleaned int

	m.mu.Lock()

	switch strategy {
	case CleanupStrategyLRU:
		cleaned = m.cleanupLRU()
	case CleanupStrategyTTL:
		cleaned = m.cleanupTTL()
	case CleanupStrategyForce:
		cleaned = m.cleanupForce()
	default:
		cleaned = m.cleanupLRU()
	}

	m.totalStats.CleanupCount++
	m.totalStats.LastCleanupTime = time.Now()

	m.mu.Unlock()

	// 更新指标（在锁外调用以避免死锁）
	m.updateMetrics()
	m.checkMemoryStatus()

	return cleaned
}

// cleanupLoop 后台清理循环
func (m *FourDimensionMemoryManager) cleanupLoop() {
	ticker := time.NewTicker(m.config.Cleanup.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.autoCleanup()
		case <-m.stopChan:
			return
		}
	}
}

// autoCleanup 自动清理（基于内存状态）
func (m *FourDimensionMemoryManager) autoCleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()

	switch m.status {
	case MemoryStatusNormal:
		// 仅清理过期条目
		m.cleanupTTL()
	case MemoryStatusWarning:
		// 清理过期 + 部分 LRU
		m.cleanupTTL()
		m.cleanupLRU()
	case MemoryStatusCritical:
		// 强制清理
		m.cleanupForce()
	}

	m.totalStats.CleanupCount++
	m.totalStats.LastCleanupTime = time.Now()
	m.updateMetrics()
}

// cleanupLRU LRU 清理（驱逐访问次数最少的条目）
func (m *FourDimensionMemoryManager) cleanupLRU() int {
	cleaned := 0

	for _, acc := range m.accounts {
		cleaned += m.cleanupEntriesLRU(acc.Entries, m.config.Limit.AccountLimit, &acc.Stats)
	}

	for _, prod := range m.products {
		cleaned += m.cleanupEntriesLRU(prod.Entries, m.config.Limit.ProductLimit, &prod.Stats)
	}

	for _, reg := range m.regions {
		cleaned += m.cleanupEntriesLRU(reg.Entries, m.config.Limit.RegionLimit, &reg.Stats)
	}

	for _, res := range m.resources {
		cleaned += m.cleanupEntriesLRU(res.Entries, m.config.Limit.ResourceLimit, &res.Stats)
	}

	return cleaned
}

// cleanupEntriesLRU 清理特定维度的条目（LRU）
func (m *FourDimensionMemoryManager) cleanupEntriesLRU(entries map[string]*MemoryEntry, limit int64, stats *MemoryStats) int {
	if stats.TotalBytes <= limit {
		return 0
	}

	needToFree := stats.TotalBytes - limit
	cleaned := 0
	freed := int64(0)

	// 收集所有条目并按访问次数排序（LRU）
	type entryWithKey struct {
		key   string
		entry *MemoryEntry
	}

	entriesSlice := make([]entryWithKey, 0, len(entries))
	for key, entry := range entries {
		entriesSlice = append(entriesSlice, entryWithKey{key, entry})
	}

	// 按访问次数排序（从小到大），如果访问次数相同则按最后访问时间排序
	for i := 0; i < len(entriesSlice)-1; i++ {
		for j := i + 1; j < len(entriesSlice); j++ {
			// 先比较访问次数
			if entriesSlice[i].entry.AccessCount > entriesSlice[j].entry.AccessCount {
				entriesSlice[i], entriesSlice[j] = entriesSlice[j], entriesSlice[i]
			} else if entriesSlice[i].entry.AccessCount == entriesSlice[j].entry.AccessCount {
				// 访问次数相同，按最后访问时间排序（旧的在前面）
				if entriesSlice[i].entry.LastUsedAt.After(entriesSlice[j].entry.LastUsedAt) {
					entriesSlice[i], entriesSlice[j] = entriesSlice[j], entriesSlice[i]
				}
			}
		}
	}

	// 逐个删除，直到释放足够的内存
	for _, ewk := range entriesSlice {
		if freed >= needToFree {
			break
		}

		delete(entries, ewk.key)
		stats.TotalBytes -= ewk.entry.Bytes
		stats.EntryCount--
		stats.EvictedCount++
		m.totalStats.TotalBytes -= ewk.entry.Bytes
		m.totalStats.EntryCount--
		m.totalStats.EvictedCount++
		freed += ewk.entry.Bytes
		cleaned++
	}

	return cleaned
}

// cleanupTTL TTL 清理（删除过期条目）
func (m *FourDimensionMemoryManager) cleanupTTL() int {
	cleaned := 0
	now := time.Now()

	for _, acc := range m.accounts {
		cleaned += m.cleanupEntriesTTL(acc.Entries, now, m.config.Limit.TTL, &acc.Stats)
	}

	for _, prod := range m.products {
		cleaned += m.cleanupEntriesTTL(prod.Entries, now, m.config.Limit.TTL, &prod.Stats)
	}

	for _, reg := range m.regions {
		cleaned += m.cleanupEntriesTTL(reg.Entries, now, m.config.Limit.TTL, &reg.Stats)
	}

	for _, res := range m.resources {
		cleaned += m.cleanupEntriesTTL(res.Entries, now, m.config.Limit.TTL, &res.Stats)
	}

	return cleaned
}

// cleanupEntriesTTL 清理特定维度的条目（TTL）
func (m *FourDimensionMemoryManager) cleanupEntriesTTL(entries map[string]*MemoryEntry, now time.Time, ttl time.Duration, stats *MemoryStats) int {
	cleaned := 0

	for key, entry := range entries {
		if now.Sub(entry.LastUsedAt) > ttl {
			delete(entries, key)
			stats.TotalBytes -= entry.Bytes
			stats.EntryCount--
			stats.ExpiredCount++
			m.totalStats.TotalBytes -= entry.Bytes
			m.totalStats.EntryCount--
			m.totalStats.ExpiredCount++
			cleaned++
		}
	}

	return cleaned
}

// cleanupForce 强制清理（清理指定比例的条目）
func (m *FourDimensionMemoryManager) cleanupForce() int {
	cleaned := 0

	for _, acc := range m.accounts {
		cleaned += m.cleanupEntriesForce(acc.Entries, m.config.Cleanup.ForceCleanupRatio, &acc.Stats)
	}

	for _, prod := range m.products {
		cleaned += m.cleanupEntriesForce(prod.Entries, m.config.Cleanup.ForceCleanupRatio, &prod.Stats)
	}

	for _, reg := range m.regions {
		cleaned += m.cleanupEntriesForce(reg.Entries, m.config.Cleanup.ForceCleanupRatio, &reg.Stats)
	}

	for _, res := range m.resources {
		cleaned += m.cleanupEntriesForce(res.Entries, m.config.Cleanup.ForceCleanupRatio, &res.Stats)
	}

	return cleaned
}

// cleanupEntriesForce 清理特定维度的条目（强制）
func (m *FourDimensionMemoryManager) cleanupEntriesForce(entries map[string]*MemoryEntry, ratio float64, stats *MemoryStats) int {
	if ratio <= 0 || ratio > 1 {
		ratio = 0.5
	}

	targetCount := int(float64(stats.EntryCount) * ratio)
	if targetCount == 0 {
		return 0
	}

	cleaned := 0
	count := 0

	for key, entry := range entries {
		if count >= targetCount {
			break
		}

		delete(entries, key)
		stats.TotalBytes -= entry.Bytes
		stats.EntryCount--
		stats.EvictedCount++
		m.totalStats.TotalBytes -= entry.Bytes
		m.totalStats.EntryCount--
		m.totalStats.EvictedCount++
		count++
		cleaned++
	}

	return cleaned
}

// checkMemoryStatus 检查内存状态
func (m *FourDimensionMemoryManager) checkMemoryStatus() {
	m.mu.RLock()
	totalBytes := m.totalStats.TotalBytes
	totalMem := m.config.Limit.AccountLimit + m.config.Limit.ProductLimit + m.config.Limit.RegionLimit + m.config.Limit.ResourceLimit
	warningThreshold := m.config.Cleanup.WarningThreshold
	criticalThreshold := m.config.Cleanup.CriticalThreshold
	m.mu.RUnlock()

	if totalMem == 0 {
		totalMem = 1 // 避免除以零
	}

	ratio := float64(totalBytes) / float64(totalMem)

	m.mu.Lock()
	switch {
	case ratio >= criticalThreshold:
		m.status = MemoryStatusCritical
		metrics.MemoryAlertTotal.WithLabelValues("critical").Inc()
	case ratio >= warningThreshold:
		m.status = MemoryStatusWarning
		metrics.MemoryAlertTotal.WithLabelValues("warning").Inc()
	default:
		m.status = MemoryStatusNormal
	}
	m.mu.Unlock()
}

// GetStatus 获取内存状态
func (m *FourDimensionMemoryManager) GetStatus() MemoryStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.status
}

// GetMemoryManagerStats 获取内存管理器统计
func (m *FourDimensionMemoryManager) GetMemoryManagerStats() MemoryStats {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.totalStats
}

// updateMetrics 更新指标
func (m *FourDimensionMemoryManager) updateMetrics() {
	// 更新内存使用指标
	for _, dim := range []string{"account", "product", "region", "resource"} {
		usage := m.GetMemoryUsage(dim)
		metrics.MemoryUsageBytes.WithLabelValues(dim).Set(float64(usage.TotalBytes))
	}

	// 更新系统内存使用（runtime）
	var mstats runtime.MemStats
	runtime.ReadMemStats(&mstats)
	metrics.MemoryUsageBytes.WithLabelValues("system").Set(float64(mstats.Alloc))
}
