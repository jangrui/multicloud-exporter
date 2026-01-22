package object_pool

import (
	"sync"
	"sync/atomic"
	"time"
)

// ObjectPool 对象池基础结构，基于 sync.Pool 实现
type ObjectPool struct {
	pool sync.Pool

	// 监控指标
	hits   uint64 // 池命中次数
	misses uint64 // 池未命中次数
	puts   uint64 // 归还次数
}

// NewObjectPool 创建新的对象池
// newFunc 是创建新对象的函数
func NewObjectPool(newFunc func() interface{}) *ObjectPool {
	return &ObjectPool{
		pool: sync.Pool{
			New: newFunc,
		},
	}
}

// Get 从池中获取对象
func (p *ObjectPool) Get() interface{} {
	obj := p.pool.Get()
	if obj != nil {
		atomic.AddUint64(&p.hits, 1)
	} else {
		atomic.AddUint64(&p.misses, 1)
	}
	return obj
}

// Put 归还对象到池
func (p *ObjectPool) Put(obj interface{}) {
	if obj == nil {
		return
	}
	p.pool.Put(obj)
	atomic.AddUint64(&p.puts, 1)
}

// GetStats 获取对象池统计信息
func (p *ObjectPool) GetStats() PoolStats {
	return PoolStats{
		Hits:   atomic.LoadUint64(&p.hits),
		Misses: atomic.LoadUint64(&p.misses),
		Puts:   atomic.LoadUint64(&p.puts),
	}
}

// Reset 重置对象池统计信息
func (p *ObjectPool) Reset() {
	atomic.StoreUint64(&p.hits, 0)
	atomic.StoreUint64(&p.misses, 0)
	atomic.StoreUint64(&p.puts, 0)
}

// PoolStats 对象池统计信息
type PoolStats struct {
	Hits   uint64 // 池命中次数
	Misses uint64 // 池未命中次数
	Puts   uint64 // 归还次数
}

// GetHitRatio 获取命中率
func (s *PoolStats) GetHitRatio() float64 {
	total := s.Hits + s.Misses
	if total == 0 {
		return 0
	}
	return float64(s.Hits) / float64(total) * 100
}

// PoolManager 对象池管理器，负责管理多个对象池
type PoolManager struct {
	pools map[string]*ObjectPool
	mu    sync.RWMutex
}

// NewPoolManager 创建对象池管理器
func NewPoolManager() *PoolManager {
	return &PoolManager{
		pools: make(map[string]*ObjectPool),
	}
}

// Register 注册对象池
func (pm *PoolManager) Register(name string, pool *ObjectPool) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.pools[name] = pool
}

// GetPool 获取对象池
func (pm *PoolManager) GetPool(name string) (*ObjectPool, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	pool, ok := pm.pools[name]
	return pool, ok
}

// GetAllStats 获取所有对象池的统计信息
func (pm *PoolManager) GetAllStats() map[string]PoolStats {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	stats := make(map[string]PoolStats, len(pm.pools))
	for name, pool := range pm.pools {
		stats[name] = pool.GetStats()
	}
	return stats
}

// CleanupManager 自动清理管理器，定期清理闲置对象
type CleanupManager struct {
	pools           map[string]*ObjectPool
	cleanupInterval time.Duration
	idleTimeout     time.Duration
	stopChan        chan struct{}
	mu              sync.RWMutex
}

// NewCleanupManager 创建清理管理器
func NewCleanupManager(cleanupInterval, idleTimeout time.Duration) *CleanupManager {
	return &CleanupManager{
		pools:           make(map[string]*ObjectPool),
		cleanupInterval: cleanupInterval,
		idleTimeout:     idleTimeout,
		stopChan:        make(chan struct{}),
	}
}

// Register 注册对象池
func (cm *CleanupManager) Register(name string, pool *ObjectPool) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.pools[name] = pool
}

// Start 启动自动清理协程
func (cm *CleanupManager) Start() {
	ticker := time.NewTicker(cm.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			cm.cleanup()
		case <-cm.stopChan:
			return
		}
	}
}

// Stop 停止自动清理协程
func (cm *CleanupManager) Stop() {
	close(cm.stopChan)
}

// cleanup 执行清理逻辑
func (cm *CleanupManager) cleanup() {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	// 获取所有池的统计信息
	for _, pool := range cm.pools {
		stats := pool.GetStats()
		hitRatio := stats.GetHitRatio()

		// 如果命中率过低（< 10%），重置池统计
		if hitRatio < 10.0 && stats.Hits > 100 {
			// 注意：sync.Pool 本身有自动清理机制
			// 这里主要统计重置，避免统计信息无限增长
			pool.Reset()
		}
	}
}
