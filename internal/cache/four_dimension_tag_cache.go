package cache

import (
	"sync"
	"time"

	"go.uber.org/zap"

	"multicloud-exporter/internal/metrics"
)

// CacheEntry 缓存条目
type CacheEntry struct {
	tags         map[string]string
	createdAt    time.Time
	lastAccessed time.Time
	accessCount  int
}

// NewCacheEntry 创建缓存条目
func NewCacheEntry(tags map[string]string) *CacheEntry {
	now := time.Now()
	return &CacheEntry{
		tags:         tags,
		createdAt:    now,
		lastAccessed: now,
		accessCount:  1,
	}
}

// GetTags 获取标签
func (e *CacheEntry) GetTags() map[string]string {
	e.lastAccessed = time.Now()
	e.accessCount++
	return e.tags
}

// IsExpired 检查是否过期
func (e *CacheEntry) IsExpired(ttl time.Duration) bool {
	return time.Since(e.createdAt) > ttl
}

// GetAccessCount 获取访问次数
func (e *CacheEntry) GetAccessCount() int {
	return e.accessCount
}

// CacheConfig 缓存配置
type CacheConfig struct {
	TTL             time.Duration // 缓存 TTL，默认 30 分钟
	MaxEntries      int           // 最大缓存条目数，默认 10000
	CleanupInterval time.Duration // 清理间隔，默认 5 分钟
}

// DefaultCacheConfig 默认缓存配置
func DefaultCacheConfig() CacheConfig {
	return CacheConfig{
		TTL:             30 * time.Minute,
		MaxEntries:      10000,
		CleanupInterval: 5 * time.Minute,
	}
}

// FourDimensionTagCache 四维标签缓存
type FourDimensionTagCache struct {
	mu       sync.RWMutex
	logger   *zap.Logger
	config   CacheConfig
	entries  map[string]*CacheEntry
	stopChan chan struct{}
}

// NewFourDimensionTagCache 创建四维标签缓存
func NewFourDimensionTagCache(logger *zap.Logger, config CacheConfig) *FourDimensionTagCache {
	cache := &FourDimensionTagCache{
		logger:   logger,
		config:   config,
		entries:  make(map[string]*CacheEntry),
		stopChan: make(chan struct{}),
	}

	go cache.cleanupLoop()

	return cache
}

// GenerateCacheKey 生成缓存键
func (c *FourDimensionTagCache) GenerateCacheKey(accountID, productID, regionID, resourceID string) string {
	return accountID + ":" + productID + ":" + regionID + ":" + resourceID
}

// GetTags 获取资源标签
func (c *FourDimensionTagCache) GetTags(accountID, productID, regionID, resourceID string) (map[string]string, bool) {
	key := c.GenerateCacheKey(accountID, productID, regionID, resourceID)

	c.mu.RLock()
	entry, exists := c.entries[key]
	c.mu.RUnlock()

	if !exists {
		metrics.RecordCacheMiss("four_dimension_tag")
		return nil, false
	}

	// 检查是否过期
	if entry.IsExpired(c.config.TTL) {
		metrics.RecordCacheMiss("four_dimension_tag")
		c.logger.Debug("cache entry expired",
			zap.String("account_id", accountID),
			zap.String("product_id", productID),
			zap.String("region_id", regionID),
			zap.String("resource_id", resourceID),
		)
		return nil, false
	}

	metrics.RecordCacheHit("four_dimension_tag")
	return entry.GetTags(), true
}

// SetTags 设置资源标签
func (c *FourDimensionTagCache) SetTags(accountID, productID, regionID, resourceID string, tags map[string]string) {
	key := c.GenerateCacheKey(accountID, productID, regionID, resourceID)

	c.mu.Lock()
	defer c.mu.Unlock()

	// 检查是否超过最大条目数
	if len(c.entries) >= c.config.MaxEntries {
		c.evictLRU()
	}

	c.entries[key] = NewCacheEntry(tags)
	c.updateCacheSizeMetrics()
}

// IsExpired 检查缓存是否过期
func (c *FourDimensionTagCache) IsExpired(accountID, productID, regionID, resourceID string) bool {
	key := c.GenerateCacheKey(accountID, productID, regionID, resourceID)

	c.mu.RLock()
	entry, exists := c.entries[key]
	c.mu.RUnlock()

	if !exists {
		return true
	}

	return entry.IsExpired(c.config.TTL)
}

// Invalidate 使缓存失效
func (c *FourDimensionTagCache) Invalidate(accountID, productID, regionID, resourceID string) {
	key := c.GenerateCacheKey(accountID, productID, regionID, resourceID)

	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.entries, key)
	c.updateCacheSizeMetrics()
}

// InvalidateByAccount 使账号下所有缓存失效
func (c *FourDimensionTagCache) InvalidateByAccount(accountID string) int {
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
func (c *FourDimensionTagCache) InvalidateByProduct(accountID, productID string) int {
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
func (c *FourDimensionTagCache) InvalidateByRegion(accountID, productID, regionID string) int {
	prefix := accountID + ":" + productID + ":" + regionID + ":"

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

// Clear 清空所有缓存
func (c *FourDimensionTagCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries = make(map[string]*CacheEntry)
	c.updateCacheSizeMetrics()
}

// GetStats 获取缓存统计信息
func (c *FourDimensionTagCache) GetStats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	stats := CacheStats{
		TotalEntries:     len(c.entries),
		ExpiredEntries:   0,
		TotalAccessCount: 0,
	}

	for _, entry := range c.entries {
		stats.TotalAccessCount += entry.GetAccessCount()
		if entry.IsExpired(c.config.TTL) {
			stats.ExpiredEntries++
		}
	}

	return stats
}

// GetSize 获取缓存大小
func (c *FourDimensionTagCache) GetSize() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

// Stop 停止缓存清理
func (c *FourDimensionTagCache) Stop() {
	close(c.stopChan)
}

// cleanupLoop 清理循环
func (c *FourDimensionTagCache) cleanupLoop() {
	ticker := time.NewTicker(c.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopChan:
			c.logger.Info("tag cache cleanup stopped")
			return
		case <-ticker.C:
			c.cleanup()
		}
	}
}

// cleanup 清理过期缓存
func (c *FourDimensionTagCache) cleanup() {
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
		c.logger.Debug("cleaned expired cache entries",
			zap.Int("expired_count", expiredCount),
			zap.Int("remaining_count", len(c.entries)),
		)
		metrics.RecordLRUEvicted("tag_cache")
	}

	c.updateCacheSizeMetrics()
}

// evictLRU 驱逐最少使用的缓存
func (c *FourDimensionTagCache) evictLRU() {
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
		c.logger.Debug("evicted LRU cache entry",
			zap.String("key", lruKey),
			zap.Int("access_count", minAccessCount),
			zap.Int("remaining_count", len(c.entries)),
		)
		metrics.RecordLRUEvicted("tag_cache")
	}

	c.updateCacheSizeMetrics()
}

// updateCacheSizeMetrics 更新缓存大小指标
func (c *FourDimensionTagCache) updateCacheSizeMetrics() {
	metrics.UpdateCacheMetrics("four_dimension_tag", int64(len(c.entries)), len(c.entries))
}

// startsWith 检查字符串是否以指定前缀开头
func startsWith(s, prefix string) bool {
	if len(s) < len(prefix) {
		return false
	}
	return s[:len(prefix)] == prefix
}

// CacheStats 缓存统计信息
type CacheStats struct {
	TotalEntries     int
	ExpiredEntries   int
	TotalAccessCount int
}

// GetCacheSizeInBytes 获取缓存大小（字节）
func (c *FourDimensionTagCache) GetCacheSizeInBytes() int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var size int64
	for _, entry := range c.entries {
		// 估算每个标签条目的大小（key + tags）
		size += int64(len(entry.tags) * 100) // 假设每个标签 100 字节
	}

	return size
}
