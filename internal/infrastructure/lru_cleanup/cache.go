package lru_cleanup

import (
	"container/list"
	"sync"
	"time"
)

// GlobalLRUCache 全局 LRU 缓存，用于追踪管理器访问时间
type GlobalLRUCache struct {
	mu sync.RWMutex

	// hashmap 存储 key -> *list.Element
	entries map[string]*list.Element

	// 双向链表存储访问顺序（最近访问在前面）
	lruList *list.List

	// 配置
	capacity int           // 最大容量
	ttl      time.Duration // TTL 超时

	// 监控指标
	evictedTotal uint64 // 驱逐总数
	cleanupTotal uint64 // 清理总数
}

// lruEntry LRU 条目
type lruEntry struct {
	key        string
	value      interface{}
	accessTime time.Time
}

// NewGlobalLRUCache 创建新的全局 LRU 缓存
func NewGlobalLRUCache(capacity int, ttl time.Duration) *GlobalLRUCache {
	return &GlobalLRUCache{
		entries:      make(map[string]*list.Element),
		lruList:      list.New(),
		capacity:     capacity,
		ttl:          ttl,
		evictedTotal: 0,
		cleanupTotal: 0,
	}
}

// TrackAccess 记录管理器访问时间
func (c *GlobalLRUCache) TrackAccess(key string, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()

	// 检查是否已存在
	if elem, ok := c.entries[key]; ok {
		// 更新访问时间和值
		entry := elem.Value.(*lruEntry)
		entry.value = value
		entry.accessTime = now

		// 移动到链表前面（最近访问）
		c.lruList.MoveToFront(elem)
		return
	}

	// 新增条目
	entry := &lruEntry{
		key:        key,
		value:      value,
		accessTime: now,
	}
	elem := c.lruList.PushFront(entry)
	c.entries[key] = elem

	// 检查容量限制
	if c.lruList.Len() > c.capacity {
		c.evictOldest()
	}
}

// Get 获取条目（不更新访问时间）
func (c *GlobalLRUCache) Get(key string) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	elem, ok := c.entries[key]
	if !ok {
		return nil, false
	}

	entry := elem.Value.(*lruEntry)
	return entry.value, true
}

// GetAndUpdate 获取条目并更新访问时间
func (c *GlobalLRUCache) GetAndUpdate(key string) (interface{}, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	elem, ok := c.entries[key]
	if !ok {
		return nil, false
	}

	entry := elem.Value.(*lruEntry)
	entry.accessTime = time.Now()

	// 移动到链表前面（最近访问）
	c.lruList.MoveToFront(elem)

	return entry.value, true
}

// GetLeastRecentlyUsed 获取最少使用的条目
func (c *GlobalLRUCache) GetLeastRecentlyUsed() (string, interface{}, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.lruList.Len() == 0 {
		return "", nil, false
	}

	elem := c.lruList.Back()
	if elem == nil {
		return "", nil, false
	}

	entry := elem.Value.(*lruEntry)
	return entry.key, entry.value, true
}

// Evict 驱逐指定条目
func (c *GlobalLRUCache) Evict(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	elem, ok := c.entries[key]
	if !ok {
		return false
	}

	c.lruList.Remove(elem)
	delete(c.entries, key)
	c.evictedTotal++

	return true
}

// evictOldest 驱逐最旧的条目（内部方法，调用前必须持有锁）
func (c *GlobalLRUCache) evictOldest() {
	if c.lruList.Len() == 0 {
		return
	}

	elem := c.lruList.Back()
	if elem == nil {
		return
	}

	entry := elem.Value.(*lruEntry)
	c.lruList.Remove(elem)
	delete(c.entries, entry.key)
	c.evictedTotal++
}

// CleanupLoop 定期清理协程
func (c *GlobalLRUCache) CleanupLoop(interval time.Duration, stopChan <-chan struct{}) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.cleanup()
		case <-stopChan:
			return
		}
	}
}

// cleanup 执行清理逻辑（内部方法，调用前必须持有锁）
func (c *GlobalLRUCache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	cleanupCount := 0

	// 遍历链表，清理超时条目
	var next *list.Element
	for elem := c.lruList.Back(); elem != nil; elem = next {
		next = elem.Prev()

		entry := elem.Value.(*lruEntry)

		// 检查 TTL
		if now.Sub(entry.accessTime) > c.ttl {
			c.lruList.Remove(elem)
			delete(c.entries, entry.key)
			c.evictedTotal++
			cleanupCount++
		}
	}

	c.cleanupTotal += uint64(cleanupCount)
}

// GetStats 获取 LRU 缓存统计信息
func (c *GlobalLRUCache) GetStats() LRUCacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return LRUCacheStats{
		Capacity:     uint64(c.capacity),
		Size:         c.lruList.Len(),
		EvictedTotal: c.evictedTotal,
		CleanupTotal: c.cleanupTotal,
	}
}

// LRUCacheStats LRU 缓存统计信息
type LRUCacheStats struct {
	Capacity     uint64 // 最大容量
	Size         int    // 当前大小
	EvictedTotal uint64 // 驱逐总数
	CleanupTotal uint64 // 清理总数
}
