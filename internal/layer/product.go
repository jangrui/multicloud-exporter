package layer

import (
	"sync"
	"sync/atomic"

	"multicloud-exporter/internal/infrastructure/lock_free"
	"multicloud-exporter/internal/infrastructure/lru_cleanup"

	"go.uber.org/zap"
)

// ========== 产品层 ==========

// ProductStatus 产品状态
type ProductStatus string

const (
	ProductStatusActive   ProductStatus = "active"   // 活跃
	ProductStatusDegraded ProductStatus = "degraded" // 降级
	ProductStatusDisabled ProductStatus = "disabled" // 禁用
)

// ProductManagerConfig 产品管理器配置
type ProductManagerConfig struct {
	AccountID string
	ProductID string
	Provider  string
	Region    string
	Logger    *zap.Logger
}

// ProductManager 产品管理器
type ProductManager struct {
	accountID string
	productID string
	provider  string
	region    string

	stateManager *lock_free.LockFreeManager
	status       ProductStatus

	regionManagers map[string]*RegionManager
	regionCount    atomic.Uint32

	lruCache *lru_cleanup.GlobalLRUCache
	stats    *lock_free.GlobalStats
	mu       sync.RWMutex
}

// NewProductManager 创建产品管理器
func NewProductManager(cfg ProductManagerConfig) *ProductManager {
	pm := &ProductManager{
		accountID:      cfg.AccountID,
		productID:      cfg.ProductID,
		provider:       cfg.Provider,
		region:         cfg.Region,
		stateManager:   lock_free.NewLockFreeManager(),
		status:         ProductStatusActive,
		regionManagers: make(map[string]*RegionManager),
		lruCache:       lru_cleanup.NewGlobalLRUCache(1000, 5*60*1000*1000*1000),
		stats:          lock_free.NewGlobalStats(),
	}

	if cfg.Logger != nil {
		cfg.Logger.Info("product manager created",
			zap.String("account_id", cfg.AccountID),
			zap.String("product_id", cfg.ProductID),
		)
	}

	return pm
}

// GetRegionManager 获取或创建区域管理器
func (pm *ProductManager) GetRegionManager(region string) (*RegionManager, error) {
	pm.mu.RLock()
	rm, exists := pm.regionManagers[region]
	if exists {
		pm.mu.RUnlock()
		return rm, nil
	}
	pm.mu.RUnlock()

	pm.mu.Lock()
	defer pm.mu.Unlock()

	rm, exists = pm.regionManagers[region]
	if exists {
		return rm, nil
	}

	rm = NewRegionManager(RegionManagerConfig{
		AccountID: pm.accountID,
		ProductID: pm.productID,
		Region:    region,
		Provider:  pm.provider,
		Logger:    zap.L(),
	})

	pm.regionManagers[region] = rm
	pm.regionCount.Add(1)

	return rm, nil
}

// GetProductStatus 获取产品状态
func (pm *ProductManager) GetProductStatus() ProductStatus {
	return pm.status
}

// ShouldSkipProduct 检查是否应该跳过该产品
func (pm *ProductManager) ShouldSkipProduct() bool {
	if pm.status == ProductStatusDisabled || pm.status == ProductStatusDegraded {
		return true
	}
	return false
}

// UpdateProductStatus 更新产品状态
func (pm *ProductManager) UpdateProductStatus(status ProductStatus, reason string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pm.status = status
	zap.L().Info("product status changed",
		zap.String("account_id", pm.accountID),
		zap.String("product_id", pm.productID),
		zap.String("status", string(status)),
		zap.String("reason", reason),
	)
}

// Cleanup 清理资源
func (pm *ProductManager) Cleanup() {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	for _, rm := range pm.regionManagers {
		rm.Cleanup()
	}

	pm.regionManagers = make(map[string]*RegionManager)
	pm.regionCount.Store(0)
}

// GetRegionCount 获取区域数量
func (pm *ProductManager) GetRegionCount() uint32 {
	return pm.regionCount.Load()
}
