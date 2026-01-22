package layer

import (
	"sync"
	"sync/atomic"

	"multicloud-exporter/internal/infrastructure/lock_free"
	"multicloud-exporter/internal/infrastructure/lru_cleanup"

	"go.uber.org/zap"
)

// ========== 资源层 ==========

// ResourceStatus 资源状态
type ResourceStatus string

const (
	ResourceStatusActive   ResourceStatus = "active"   // 活跃
	ResourceStatusDegraded ResourceStatus = "degraded" // 降级
	ResourceStatusDisabled ResourceStatus = "disabled" // 禁用
)

// ResourceManagerConfig 资源管理器配置
type ResourceManagerConfig struct {
	AccountID  string
	ProductID  string
	Region     string
	ResourceID string
	Provider   string
	Logger     *zap.Logger
}

// ResourceManager 资源管理器
type ResourceManager struct {
	accountID  string
	productID  string
	region     string
	resourceID string
	provider   string

	stateManager *lock_free.LockFreeManager
	status       ResourceStatus

	mu sync.RWMutex
}

// NewResourceManager 创建资源管理器
func NewResourceManager(cfg ResourceManagerConfig) *ResourceManager {
	rm := &ResourceManager{
		accountID:    cfg.AccountID,
		productID:    cfg.ProductID,
		region:       cfg.Region,
		resourceID:   cfg.ResourceID,
		provider:     cfg.Provider,
		stateManager: lock_free.NewLockFreeManager(),
		status:       ResourceStatusActive,
	}

	if cfg.Logger != nil {
		cfg.Logger.Info("resource manager created",
			zap.String("account_id", cfg.AccountID),
			zap.String("product_id", cfg.ProductID),
			zap.String("region", cfg.Region),
			zap.String("resource_id", cfg.ResourceID),
		)
	}

	return rm
}

// GetResourceStatus 获取资源状态
func (rm *ResourceManager) GetResourceStatus() ResourceStatus {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return rm.status
}

// ShouldSkipResource 检查是否应该跳过该资源
func (rm *ResourceManager) ShouldSkipResource() bool {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	if rm.status == ResourceStatusDisabled || rm.status == ResourceStatusDegraded {
		return true
	}
	return false
}

// UpdateResourceStatus 更新资源状态
func (rm *ResourceManager) UpdateResourceStatus(status ResourceStatus, reason string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	rm.status = status
	zap.L().Info("resource status changed",
		zap.String("account_id", rm.accountID),
		zap.String("product_id", rm.productID),
		zap.String("region", rm.region),
		zap.String("resource_id", rm.resourceID),
		zap.String("status", string(status)),
		zap.String("reason", reason),
	)
}

// Cleanup 清理资源
func (rm *ResourceManager) Cleanup() {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.status = ResourceStatusDisabled
}

// ========== 区域层 ==========

// RegionStatus 区域状态
type RegionStatus string

const (
	RegionStatusActive   RegionStatus = "active"   // 活跃
	RegionStatusDegraded RegionStatus = "degraded" // 降级
	RegionStatusDisabled RegionStatus = "disabled" // 禁用
)

// RegionManagerConfig 区域管理器配置
type RegionManagerConfig struct {
	AccountID string
	ProductID string
	Region    string
	Provider  string
	Logger    *zap.Logger
}

// RegionManager 区域管理器
type RegionManager struct {
	accountID string
	productID string
	region    string
	provider  string

	stateManager *lock_free.LockFreeManager
	status       RegionStatus

	resourceManagers map[string]*ResourceManager
	resourceCount    atomic.Uint32

	lruCache *lru_cleanup.GlobalLRUCache
	stats    *lock_free.GlobalStats
	mu       sync.RWMutex
}

// NewRegionManager 创建区域管理器
func NewRegionManager(cfg RegionManagerConfig) *RegionManager {
	rm := &RegionManager{
		accountID:        cfg.AccountID,
		productID:        cfg.ProductID,
		region:           cfg.Region,
		provider:         cfg.Provider,
		stateManager:     lock_free.NewLockFreeManager(),
		status:           RegionStatusActive,
		resourceManagers: make(map[string]*ResourceManager),
		lruCache:         lru_cleanup.NewGlobalLRUCache(1000, 5*60*1000*1000*1000),
		stats:            lock_free.NewGlobalStats(),
	}

	if cfg.Logger != nil {
		cfg.Logger.Info("region manager created",
			zap.String("account_id", cfg.AccountID),
			zap.String("product_id", cfg.ProductID),
			zap.String("region", cfg.Region),
		)
	}

	return rm
}

// GetResourceManager 获取或创建资源管理器
func (rm *RegionManager) GetResourceManager(resourceID string) (*ResourceManager, error) {
	rm.mu.RLock()
	resMgr, exists := rm.resourceManagers[resourceID]
	if exists {
		rm.mu.RUnlock()
		return resMgr, nil
	}
	rm.mu.RUnlock()

	rm.mu.Lock()
	defer rm.mu.Unlock()

	resMgr, exists = rm.resourceManagers[resourceID]
	if exists {
		return resMgr, nil
	}

	resMgr = NewResourceManager(ResourceManagerConfig{
		AccountID:  rm.accountID,
		ProductID:  rm.productID,
		Region:     rm.region,
		ResourceID: resourceID,
		Provider:   rm.provider,
		Logger:     zap.L(),
	})

	rm.resourceManagers[resourceID] = resMgr
	rm.resourceCount.Add(1)

	return resMgr, nil
}

// GetRegionStatus 获取区域状态
func (rm *RegionManager) GetRegionStatus() RegionStatus {
	return rm.status
}

// ShouldSkipRegion 检查是否应该跳过该区域
func (rm *RegionManager) ShouldSkipRegion() bool {
	if rm.status == RegionStatusDisabled || rm.status == RegionStatusDegraded {
		return true
	}
	return false
}

// UpdateRegionStatus 更新区域状态
func (rm *RegionManager) UpdateRegionStatus(status RegionStatus, reason string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	rm.status = status
	zap.L().Info("region status changed",
		zap.String("account_id", rm.accountID),
		zap.String("product_id", rm.productID),
		zap.String("region", rm.region),
		zap.String("status", string(status)),
		zap.String("reason", reason),
	)
}

// Cleanup 清理资源
func (rm *RegionManager) Cleanup() {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	for _, resMgr := range rm.resourceManagers {
		resMgr.Cleanup()
	}

	rm.resourceManagers = make(map[string]*ResourceManager)
	rm.resourceCount.Store(0)
}

// GetResourceCount 获取资源数量
func (rm *RegionManager) GetResourceCount() uint32 {
	return rm.resourceCount.Load()
}
