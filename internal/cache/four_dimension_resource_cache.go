package cache

import (
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"multicloud-exporter/internal/metrics"
)

// ResourceCacheEntry 资源缓存条目
type ResourceCacheEntry struct {
	resources    []string
	createdAt    time.Time
	lastAccessed int64 // 存储时间戳纳秒数，使用 atomic 操作
	accessCount  int64 // 使用 atomic 操作
	mu           sync.RWMutex
}

// NewResourceCacheEntry 创建资源缓存条目
func NewResourceCacheEntry(resources []string) *ResourceCacheEntry {
	now := time.Now()
	return &ResourceCacheEntry{
		resources:    resources,
		createdAt:    now,
		lastAccessed: now.UnixNano(),
		accessCount:  1,
	}
}

// GetResources 获取资源 ID 列表
func (e *ResourceCacheEntry) GetResources() []string {
	// 使用 atomic 操作更新访问时间和访问计数
	atomic.StoreInt64(&e.lastAccessed, time.Now().UnixNano())
	atomic.AddInt64(&e.accessCount, 1)
	e.mu.RLock()
	defer e.mu.RUnlock()
	// 返回资源列表的副本，避免外部修改
	resources := make([]string, len(e.resources))
	copy(resources, e.resources)
	return resources
}

// IsExpired 检查是否过期
func (e *ResourceCacheEntry) IsExpired(ttl time.Duration) bool {
	return time.Since(e.createdAt) > ttl
}

// GetAccessCount 获取访问次数
func (e *ResourceCacheEntry) GetAccessCount() int {
	return int(atomic.LoadInt64(&e.accessCount))
}

// GetResourceCount 获取资源数量
func (e *ResourceCacheEntry) GetResourceCount() int {
	return len(e.resources)
}

// ResourceCacheConfig 资源缓存配置
type ResourceCacheConfig struct {
	TTL             time.Duration // 缓存 TTL，默认 10 分钟
	MaxEntries      int           // 最大缓存条目数，默认 5000
	CleanupInterval time.Duration // 清理间隔，默认 5 分钟
}

// DefaultResourceCacheConfig 默认资源缓存配置
func DefaultResourceCacheConfig() ResourceCacheConfig {
	return ResourceCacheConfig{
		TTL:             10 * time.Minute,
		MaxEntries:      5000,
		CleanupInterval: 5 * time.Minute,
	}
}

// FourDimensionResourceCache 四维资源缓存
type FourDimensionResourceCache struct {
	mu       sync.RWMutex
	logger   *zap.Logger
	config   ResourceCacheConfig
	entries  map[string]*ResourceCacheEntry
	stopChan chan struct{}
}

// NewFourDimensionResourceCache 创建四维资源缓存
func NewFourDimensionResourceCache(logger *zap.Logger, config ResourceCacheConfig) *FourDimensionResourceCache {
	cache := &FourDimensionResourceCache{
		logger:   logger,
		config:   config,
		entries:  make(map[string]*ResourceCacheEntry),
		stopChan: make(chan struct{}),
	}

	go cache.cleanupLoop()

	return cache
}

// GenerateCacheKey 生成缓存键 (account:product:region)
func (c *FourDimensionResourceCache) GenerateCacheKey(accountID, productID, regionID string) string {
	return accountID + ":" + productID + ":" + regionID
}

// GetResourceIDs 获取资源 ID 列表
func (c *FourDimensionResourceCache) GetResourceIDs(accountID, productID, regionID string) ([]string, bool) {
	key := c.GenerateCacheKey(accountID, productID, regionID)

	c.mu.RLock()
	entry, exists := c.entries[key]
	c.mu.RUnlock()

	if !exists {
		metrics.RecordCacheMiss("four_dimension_resource")
		return nil, false
	}

	// 检查是否过期
	if entry.IsExpired(c.config.TTL) {
		metrics.RecordCacheMiss("four_dimension_resource")
		c.logger.Debug("resource cache entry expired",
			zap.String("account_id", accountID),
			zap.String("product_id", productID),
			zap.String("region_id", regionID),
		)
		return nil, false
	}

	metrics.RecordCacheHit("four_dimension_resource")
	return entry.GetResources(), true
}

// SetResourceIDs 设置资源 ID 列表
func (c *FourDimensionResourceCache) SetResourceIDs(accountID, productID, regionID string, resources []string) {
	key := c.GenerateCacheKey(accountID, productID, regionID)

	c.mu.Lock()
	defer c.mu.Unlock()

	// 检查是否超过最大条目数
	if len(c.entries) >= c.config.MaxEntries {
		c.evictLRU()
	}

	c.entries[key] = NewResourceCacheEntry(resources)
	c.updateCacheSizeMetrics()
}

// IsCacheExpired 检查缓存是否过期
func (c *FourDimensionResourceCache) IsCacheExpired(accountID, productID, regionID string) bool {
	key := c.GenerateCacheKey(accountID, productID, regionID)

	c.mu.RLock()
	entry, exists := c.entries[key]
	c.mu.RUnlock()

	if !exists {
		return true
	}

	return entry.IsExpired(c.config.TTL)
}

// Invalidate 使缓存失效
func (c *FourDimensionResourceCache) Invalidate(accountID, productID, regionID string) {
	key := c.GenerateCacheKey(accountID, productID, regionID)

	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.entries, key)
	c.updateCacheSizeMetrics()
}

// InvalidateByAccount 使账号下所有缓存失效
func (c *FourDimensionResourceCache) InvalidateByAccount(accountID string) int {
	c.mu.Lock()
	defer c.mu.Unlock()

	count := 0
	for key := range c.entries {
		if startsWith(key, accountID+":") {
			delete(c.entries, key)
			count++
		}
	}

	c.updateCacheSizeMetrics()
	return count
}

// InvalidateByProduct 使产品下所有缓存失效
func (c *FourDimensionResourceCache) InvalidateByProduct(accountID, productID string) int {
	prefix := accountID + ":" + productID + ":"

	c.mu.Lock()
	defer c.mu.Unlock()

	count := 0
	for key := range c.entries {
		if startsWith(key, prefix) {
			delete(c.entries, key)
			count++
		}
	}

	c.updateCacheSizeMetrics()
	return count
}

// InvalidateByRegion 使区域下所有缓存失效
func (c *FourDimensionResourceCache) InvalidateByRegion(accountID, productID, regionID string) int {
	// 资源缓存的键格式为 account:product:region，直接匹配完整键
	key := c.GenerateCacheKey(accountID, productID, regionID)

	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.entries[key]; exists {
		delete(c.entries, key)
		c.updateCacheSizeMetrics()
		return 1
	}

	return 0
}

// Clear 清空所有缓存
func (c *FourDimensionResourceCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries = make(map[string]*ResourceCacheEntry)
	c.updateCacheSizeMetrics()
}

// GetStats 获取缓存统计信息
func (c *FourDimensionResourceCache) GetStats() ResourceCacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	stats := ResourceCacheStats{
		TotalEntries:     len(c.entries),
		ExpiredEntries:   0,
		TotalAccessCount: 0,
		TotalResources:   0,
	}

	for _, entry := range c.entries {
		stats.TotalAccessCount += entry.GetAccessCount()
		stats.TotalResources += entry.GetResourceCount()
		if entry.IsExpired(c.config.TTL) {
			stats.ExpiredEntries++
		}
	}

	return stats
}

// GetSize 获取缓存大小
func (c *FourDimensionResourceCache) GetSize() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

// Stop 停止缓存清理
func (c *FourDimensionResourceCache) Stop() {
	close(c.stopChan)
}

// cleanupLoop 清理循环
func (c *FourDimensionResourceCache) cleanupLoop() {
	ticker := time.NewTicker(c.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopChan:
			c.logger.Info("resource cache cleanup stopped")
			return
		case <-ticker.C:
			c.cleanup()
		}
	}
}

// cleanup 清理过期缓存
func (c *FourDimensionResourceCache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	expiredCount := 0
	keysToRemove := make([]string, 0)

	// 收集过期的缓存键
	for key, entry := range c.entries {
		if entry.IsExpired(c.config.TTL) {
			keysToRemove = append(keysToRemove, key)
		}
	}

	// 删除过期缓存
	for _, key := range keysToRemove {
		delete(c.entries, key)
		expiredCount++
	}

	if expiredCount > 0 {
		c.logger.Debug("cleaned expired resource cache entries",
			zap.Int("expired_count", expiredCount),
			zap.Int("remaining_count", len(c.entries)),
		)
		metrics.RecordLRUEvicted("resource_cache")
	}

	c.updateCacheSizeMetrics()
}

// evictLRU 驱逐最少使用的缓存
func (c *FourDimensionResourceCache) evictLRU() {
	if len(c.entries) == 0 {
		return
	}

	// 查找最少使用的缓存
	var lruKey string
	minAccessCount := int(^uint(0) >> 1) // Max int

	for key, entry := range c.entries {
		if entry.GetAccessCount() < minAccessCount {
			minAccessCount = entry.GetAccessCount()
			lruKey = key
		}
	}

	// 删除 LRU 缓存
	if lruKey != "" {
		delete(c.entries, lruKey)
		c.logger.Debug("evicted LRU resource cache entry",
			zap.String("key", lruKey),
			zap.Int("access_count", minAccessCount),
			zap.Int("remaining_count", len(c.entries)),
		)
		metrics.RecordLRUEvicted("resource_cache")
	}

	c.updateCacheSizeMetrics()
}

// updateCacheSizeMetrics 更新缓存大小指标
func (c *FourDimensionResourceCache) updateCacheSizeMetrics() {
	metrics.UpdateCacheMetrics("four_dimension_resource", int64(len(c.entries)), len(c.entries))
}

// ResourceCacheStats 资源缓存统计信息
type ResourceCacheStats struct {
	TotalEntries     int // 总条目数
	ExpiredEntries   int // 过期条目数
	TotalAccessCount int // 总访问次数
	TotalResources   int // 总资源数量
}

// GetCacheSizeInBytes 获取缓存大小（字节）
func (c *FourDimensionResourceCache) GetCacheSizeInBytes() int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var size int64
	for _, entry := range c.entries {
		// 估算每个资源 ID 的大小
		for _, resourceID := range entry.GetResources() {
			size += int64(len(resourceID)) // 资源 ID 字节
		}
		// 加上条目元数据开销
		size += 100
	}

	return size
}
