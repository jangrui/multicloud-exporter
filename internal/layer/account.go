package layer

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"time"

	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/infrastructure/lock_free"
	"multicloud-exporter/internal/infrastructure/lru_cleanup"
	"multicloud-exporter/internal/metrics"

	"go.uber.org/zap"
)

// ========== 账户层 ==========

// AccountStatus 账户状态
type AccountStatus string

const (
	AccountStatusActive   AccountStatus = "active"   // 活跃
	AccountStatusDegraded AccountStatus = "degraded" // 降级
	AccountStatusDisabled AccountStatus = "disabled" // 禁用
)

// AccountManagerConfig 账户管理器配置
type AccountManagerConfig struct {
	AccountID   string
	AccountName string
	Provider    string
	Region      string
	Logger      *zap.Logger
}

// AccountManager 账户管理器
type AccountManager struct {
	accountID   string
	accountName string
	provider    string
	region      string

	// 无锁状态管理
	stateManager *lock_free.LockFreeManager
	status       AccountStatus

	// 子管理器引用
	productManagers map[string]*ProductManager

	// 性能优化
	lruCache     *lru_cleanup.GlobalLRUCache
	stats        *lock_free.GlobalStats
	productCount uint32

	// 停止通道
	stopChan chan struct{}
	mu       sync.RWMutex
}

// NewAccountManager 创建账户管理器
func NewAccountManager(cfg AccountManagerConfig) *AccountManager {
	am := &AccountManager{
		accountID:       cfg.AccountID,
		accountName:     cfg.AccountName,
		provider:        cfg.Provider,
		region:          cfg.Region,
		stateManager:    lock_free.NewLockFreeManager(),
		status:          AccountStatusActive,
		productManagers: make(map[string]*ProductManager),
		lruCache:        lru_cleanup.NewGlobalLRUCache(1000, 5*60*1000*1000*1000),
		stats:           lock_free.NewGlobalStats(),
		stopChan:        make(chan struct{}),
	}

	// 启动 LRU 清理循环
	go am.lruCache.CleanupLoop(1*time.Minute, am.stopChan)

	if cfg.Logger != nil {
		cfg.Logger.Info("account manager created",
			zap.String("account_id", cfg.AccountID),
			zap.String("account_name", cfg.AccountName),
		)
	}

	// 初始化指标
	metrics.AccountStatusTotal.WithLabelValues(cfg.AccountID, cfg.Provider, string(AccountStatusActive)).Inc()

	return am
}

// GetProductManager 获取或创建产品管理器
func (am *AccountManager) GetProductManager(productID string) (*ProductManager, error) {
	am.mu.RLock()
	// 检查是否禁用
	if am.status == AccountStatusDisabled {
		am.mu.RUnlock()
		return nil, fmt.Errorf("account %s is disabled", am.accountID)
	}

	// 检查降级状态
	if am.status == AccountStatusDegraded {
		am.mu.RUnlock()
		metrics.AccountSkipTotal.WithLabelValues(am.accountID, am.provider, "degraded").Inc()
		return nil, fmt.Errorf("account %s is degraded", am.accountID)
	}

	am.lruCache.TrackAccess(am.accountID, am)
	pm, exists := am.productManagers[productID]
	if exists {
		am.mu.RUnlock()
		return pm, nil
	}
	am.mu.RUnlock()

	am.mu.Lock()
	defer am.mu.Unlock()

	// 双重检查
	pm, exists = am.productManagers[productID]
	if exists {
		return pm, nil
	}

	pm = NewProductManager(ProductManagerConfig{
		AccountID: am.accountID,
		ProductID: productID,
		Provider:  am.provider,
		Region:    am.region,
		Logger:    zap.L(),
	})

	am.productManagers[productID] = pm
	am.productCount++

	am.stats.IncTotalRequests()

	return pm, nil
}

// GetAccountStatus 获取账户状态
func (am *AccountManager) GetAccountStatus() AccountStatus {
	am.mu.RLock()
	defer am.mu.RUnlock()
	am.lruCache.TrackAccess(am.accountID, am)
	return am.status
}

// ShouldSkipAccount 检查是否应该跳过该账户
func (am *AccountManager) ShouldSkipAccount() bool {
	am.mu.RLock()
	defer am.mu.RUnlock()
	am.lruCache.TrackAccess(am.accountID, am)

	if am.status == AccountStatusDisabled {
		metrics.AccountSkipTotal.WithLabelValues(am.accountID, am.provider, "disabled").Inc()
		return true
	}

	if am.status == AccountStatusDegraded {
		metrics.AccountSkipTotal.WithLabelValues(am.accountID, am.provider, "degraded").Inc()
		return true
	}

	return false
}

// UpdateAccountStatus 更新账户状态
func (am *AccountManager) UpdateAccountStatus(status AccountStatus, reason string) {
	am.mu.Lock()
	defer am.mu.Unlock()

	oldStatus := am.status
	if oldStatus == status {
		return
	}

	am.status = status

	metrics.AccountStatusChange.WithLabelValues(
		am.accountID,
		am.provider,
		string(oldStatus),
		string(status),
		reason,
	).Inc()

	zap.L().Info("account status changed",
		zap.String("account_id", am.accountID),
		zap.String("old_status", string(oldStatus)),
		zap.String("new_status", string(status)),
		zap.String("reason", reason),
	)
}

// DegradateAccount 降级账户
func (am *AccountManager) DegradateAccount(reason string) {
	am.UpdateAccountStatus(AccountStatusDegraded, reason)
	metrics.AccountDegradedTotal.WithLabelValues(am.accountID, am.provider, reason).Inc()
}

// RecoverAccount 恢复账户
func (am *AccountManager) RecoverAccount(reason string) {
	am.UpdateAccountStatus(AccountStatusActive, reason)
}

// DisableAccount 禁用账户
func (am *AccountManager) DisableAccount(reason string) {
	am.UpdateAccountStatus(AccountStatusDisabled, reason)
}

// GetStats 获取统计信息
func (am *AccountManager) GetStats() lock_free.GlobalStatsSnapshot {
	return am.stats.GetGlobalSnapshot()
}

// GetLRUCacheStats 获取 LRU 缓存统计信息
func (am *AccountManager) GetLRUCacheStats() lru_cleanup.LRUCacheStats {
	return am.lruCache.GetStats()
}

// Cleanup 清理资源
func (am *AccountManager) Cleanup() {
	am.mu.Lock()
	defer am.mu.Unlock()

	for _, pm := range am.productManagers {
		pm.Cleanup()
	}

	am.productManagers = make(map[string]*ProductManager)
	am.productCount = 0
}

// Stop 停止 AccountManager
func (am *AccountManager) Stop() {
	close(am.stopChan)
}

// GetProductCount 获取产品数量
func (am *AccountManager) GetProductCount() uint32 {
	am.mu.RLock()
	defer am.mu.RUnlock()
	return am.productCount
}

// GetConcurrency 获取并发度
func (am *AccountManager) GetConcurrency(cfg *config.FourDimensionConfig) int {
	// 使用 CalculateConcurrency 计算各维度并发度
	accountConc, _, _, _ := config.CalculateConcurrency(*cfg, 1)

	if cfg.MaxConcurrency > 0 && accountConc > cfg.MaxConcurrency {
		accountConc = cfg.MaxConcurrency
	}

	return accountConc
}

// OptimizeConcurrency 优化并发度（基于当前系统负载）
func (am *AccountManager) OptimizeConcurrency(cfg *config.FourDimensionConfig) int {
	baseConcurrency := am.GetConcurrency(cfg)

	if !cfg.PerformanceTuning {
		return baseConcurrency
	}

	currentLoad := float64(runtime.NumGoroutine()) / float64(10000)

	if currentLoad > 0.8 {
		return max(1, baseConcurrency/2)
	} else if currentLoad < 0.3 {
		return min(baseConcurrency*2, cfg.MaxConcurrency)
	}

	return baseConcurrency
}

// CreateWorkerPool 创建工作池
func (am *AccountManager) CreateWorkerPool(cfg *config.FourDimensionConfig) *WorkerPool {
	concurrency := am.GetConcurrency(cfg)
	return NewWorkerPool(concurrency)
}

// WorkerPool 工作池
type WorkerPool struct {
	tasks chan func()
	quit  chan struct{}
	wg    sync.WaitGroup
}

// NewWorkerPool 创建工作池
func NewWorkerPool(size int) *WorkerPool {
	return &WorkerPool{
		tasks: make(chan func(), size*2),
		quit:  make(chan struct{}),
	}
}

// Start 启动工作池
func (wp *WorkerPool) Start(ctx context.Context) {
	for i := 0; i < cap(wp.tasks)/2; i++ {
		wp.wg.Add(1)
		go wp.worker(ctx)
	}
}

// Stop 停止工作池
func (wp *WorkerPool) Stop() {
	close(wp.quit)
	wp.wg.Wait()
}

// Submit 提交任务
func (wp *WorkerPool) Submit(task func()) {
	wp.tasks <- task
}

// worker 工作协程
func (wp *WorkerPool) worker(ctx context.Context) {
	defer wp.wg.Done()

	for {
		select {
		case task := <-wp.tasks:
			task()
		case <-wp.quit:
			return
		case <-ctx.Done():
			return
		}
	}
}
