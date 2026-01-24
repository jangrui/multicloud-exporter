package collector

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"multicloud-exporter/internal/cache"
	"multicloud-exporter/internal/cluster/four_dimension_sync"
	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/degradation"
	"multicloud-exporter/internal/discovery"
	"multicloud-exporter/internal/fault_tolerance"
	"multicloud-exporter/internal/layer"
	"multicloud-exporter/internal/logger"
	"multicloud-exporter/internal/memory"
	"multicloud-exporter/internal/metrics"
	"multicloud-exporter/internal/providers"

	"go.uber.org/zap"
)

// FourDimensionCollector 四维采集器
type FourDimensionCollector struct {
	cfg             *config.Config
	syncMgr         *four_dimension_sync.FourDimensionSync
	discoveryMgr    *discovery.Manager
	accountManagers map[string]*layer.AccountManager
	accountMgrMu    sync.RWMutex

	tagCache       *cache.FourDimensionTagCache
	degradeMgr     *degradation.FourDimensionDegradationManager
	memoryMgr      *memory.FourDimensionMemoryManager
	faultTolerance *fault_tolerance.FourDimensionFaultTolerance

	cloudProviders map[string]providers.Provider
	adapters       map[string]providers.FourDimensionAdapter
	providerMu     sync.RWMutex
	adaptMu        sync.RWMutex
	collectionMode string

	zapLogger *zap.Logger
	stopped   int32

	maxConcurrency int // 最大并发度

	status     FourDimensionStatus
	statusLock sync.RWMutex
}

// FourDimensionStatus 四维采集器状态
type FourDimensionStatus struct {
	LastStart    time.Time     `json:"last_start"`
	LastEnd      time.Time     `json:"last_end"`
	LastDuration time.Duration `json:"last_duration"`
}

// FourDimensionCollectorConfig 四维采集器配置
type FourDimensionCollectorConfig struct {
	Config         *config.Config
	SyncManager    *four_dimension_sync.FourDimensionSync
	DiscoveryMgr   *discovery.Manager
	TagCacheTTL    time.Duration
	MaxConcurrency int
	CollectionMode string
	Logger         *zap.Logger
}

// NewFourDimensionCollector 创建四维采集器
func NewFourDimensionCollector(cfg FourDimensionCollectorConfig) *FourDimensionCollector {
	ctxLog := logger.NewContextLogger("FourDimensionCollector", "resource_type", "Init")

	tagCacheCfg := cache.CacheConfig{
		TTL:             cfg.TagCacheTTL,
		MaxEntries:      10000,
		CleanupInterval: 5 * time.Minute,
	}
	tagCache := cache.NewFourDimensionTagCache(cfg.Logger, tagCacheCfg)

	degradeMgr := degradation.NewFourDimensionDegradationManager(cfg.Logger, degradation.DefaultDegradationConfig())
	memoryMgr := memory.NewFourDimensionMemoryManager(memory.DefaultMemoryManagerConfig())
	faultTolerance := fault_tolerance.NewFourDimensionFaultTolerance(fault_tolerance.DefaultIsolationConfig())

	mode := "account"
	if cfg.CollectionMode != "" {
		mode = cfg.CollectionMode
	}

	collector := &FourDimensionCollector{
		cfg:             cfg.Config,
		syncMgr:         cfg.SyncManager,
		discoveryMgr:    cfg.DiscoveryMgr,
		accountManagers: make(map[string]*layer.AccountManager),
		tagCache:        tagCache,
		degradeMgr:      degradeMgr,
		memoryMgr:       memoryMgr,
		faultTolerance:  faultTolerance,
		cloudProviders:  make(map[string]providers.Provider),
		adapters:        make(map[string]providers.FourDimensionAdapter),
		collectionMode:  mode,
		zapLogger:       cfg.Logger,
	}

	go collector.degradeMgr.StartRecoveryScheduler()
	go collector.faultTolerance.StartRecoveryScheduler()

	collector.initializeProviders()
	collector.initializeAdapters()

	ctxLog.Info("四维采集器初始化完成")

	return collector
}

func (c *FourDimensionCollector) initializeProviders() {
	ctxLog := logger.NewContextLogger("FourDimensionCollector", "resource_type", "Init")
	for _, name := range providers.GetAllProviders() {
		if factory, ok := providers.GetFactory(name); ok {
			provider := factory(c.cfg, c.discoveryMgr, nil)
			c.providerMu.Lock()
			c.cloudProviders[name] = provider
			c.providerMu.Unlock()
			ctxLog.Infof("FourDimensionCollector 初始化 Provider: %s", name)
		}
	}
}

func (c *FourDimensionCollector) initializeAdapters() {
	ctxLog := logger.NewContextLogger("FourDimensionCollector", "resource_type", "Init")
	c.providerMu.RLock()
	defer c.providerMu.RUnlock()

	for providerName, provider := range c.cloudProviders {
		if factory, ok := providers.GetFourDimensionAdapter(providerName); ok {
			adapter := factory(provider, c.zapLogger)
			if adapter != nil {
				c.adaptMu.Lock()
				c.adapters[providerName] = adapter
				c.adaptMu.Unlock()
				ctxLog.Infof("FourDimensionCollector 初始化适配器: %s", providerName)
			}
		} else {
			ctxLog.Warnf("四维适配器未注册: %s", providerName)
		}
	}
}

func layerStatusToString(status layer.AccountStatus) string {
	switch status {
	case layer.AccountStatusActive:
		return "active"
	case layer.AccountStatusDegraded:
		return "degraded"
	case layer.AccountStatusDisabled:
		return "disabled"
	default:
		return "unknown"
	}
}

func productStatusToString(status layer.ProductStatus) string {
	switch status {
	case layer.ProductStatusActive:
		return "active"
	case layer.ProductStatusDegraded:
		return "degraded"
	case layer.ProductStatusDisabled:
		return "disabled"
	default:
		return "unknown"
	}
}

func regionStatusToString(status layer.RegionStatus) string {
	switch status {
	case layer.RegionStatusActive:
		return "active"
	case layer.RegionStatusDegraded:
		return "degraded"
	case layer.RegionStatusDisabled:
		return "disabled"
	default:
		return "unknown"
	}
}

func (c *FourDimensionCollector) getOrCreateAccountManager(account config.CloudAccount) *layer.AccountManager {
	accountKey := fmt.Sprintf("%s:%s", account.Provider, account.AccountID)

	c.accountMgrMu.RLock()
	mgr, ok := c.accountManagers[accountKey]
	c.accountMgrMu.RUnlock()

	if ok {
		return mgr
	}

	c.accountMgrMu.Lock()
	defer c.accountMgrMu.Unlock()

	if mgr, ok = c.accountManagers[accountKey]; ok {
		return mgr
	}

	cfg := layer.AccountManagerConfig{
		AccountID: account.AccountID,
		Provider:  account.Provider,
		Logger:    c.zapLogger,
	}

	mgr = layer.NewAccountManager(cfg)
	c.accountManagers[accountKey] = mgr

	return mgr
}

func (c *FourDimensionCollector) getAdapter(provider string) (providers.FourDimensionAdapter, bool) {
	c.adaptMu.RLock()
	defer c.adaptMu.RUnlock()
	adapter, ok := c.adapters[provider]
	return adapter, ok
}

func (c *FourDimensionCollector) getAccounts() []config.CloudAccount {
	if c.cfg == nil {
		return nil
	}

	c.cfg.Mu.RLock()
	defer c.cfg.Mu.RUnlock()

	var accounts []config.CloudAccount
	for provider, list := range c.cfg.AccountsByProvider {
		for _, acc := range list {
			acc.Provider = provider
			accounts = append(accounts, acc)
		}
	}
	return accounts
}

func (c *FourDimensionCollector) updateAccountStatus(update four_dimension_sync.FourDimensionUpdate) {
	c.accountMgrMu.RLock()
	defer c.accountMgrMu.RUnlock()

	if mgr, ok := c.accountManagers[update.AccountID]; ok {
		switch update.Status {
		case four_dimension_sync.StatusDisabled:
			mgr.DisableAccount("集群同步禁用")
			metrics.RecordFourDimensionAccountStatusChange(update.AccountID, "", layerStatusToString(mgr.GetAccountStatus()), "disabled", "cluster_sync")
		case four_dimension_sync.StatusDegraded:
			mgr.DegradateAccount("集群同步降级")
			metrics.RecordFourDimensionAccountStatusChange(update.AccountID, "", layerStatusToString(mgr.GetAccountStatus()), "degraded", "cluster_sync")
			metrics.RecordFourDimensionAccountDegraded(update.AccountID, "", "cluster_sync")
		case four_dimension_sync.StatusActive:
			mgr.RecoverAccount("集群同步恢复")
			metrics.RecordFourDimensionAccountStatusChange(update.AccountID, "", layerStatusToString(mgr.GetAccountStatus()), "active", "cluster_sync")
		}
	}
}

func (c *FourDimensionCollector) updateProductStatus(update four_dimension_sync.FourDimensionUpdate) {
	accountKey := update.AccountID

	c.accountMgrMu.RLock()
	defer c.accountMgrMu.RUnlock()

	if mgr, ok := c.accountManagers[accountKey]; ok {
		productMgr, err := mgr.GetProductManager(update.ProductID)
		if err == nil && productMgr != nil {
			reason := ""
			status := layer.ProductStatusActive
			switch update.Status {
			case four_dimension_sync.StatusDisabled:
				reason = "集群同步禁用"
				status = layer.ProductStatusDisabled
			case four_dimension_sync.StatusDegraded:
				reason = "集群同步降级"
				status = layer.ProductStatusDegraded
			case four_dimension_sync.StatusActive:
				reason = "集群同步恢复"
				status = layer.ProductStatusActive
			}
			productMgr.UpdateProductStatus(status, reason)
			metrics.RecordFourDimensionProductStatus(update.AccountID, update.ProductID, productStatusToString(status))
		}
	}
}

func (c *FourDimensionCollector) updateRegionStatus(update four_dimension_sync.FourDimensionUpdate) {
	accountKey := update.AccountID

	c.accountMgrMu.RLock()
	defer c.accountMgrMu.RUnlock()

	if mgr, ok := c.accountManagers[accountKey]; ok {
		productMgr, err := mgr.GetProductManager(update.ProductID)
		if err == nil && productMgr != nil {
			regionMgr, err := productMgr.GetRegionManager(update.Region)
			if err == nil && regionMgr != nil {
				reason := ""
				status := layer.RegionStatusActive
				switch update.Status {
				case four_dimension_sync.StatusDisabled:
					reason = "集群同步禁用"
					status = layer.RegionStatusDisabled
				case four_dimension_sync.StatusDegraded:
					reason = "集群同步降级"
					status = layer.RegionStatusDegraded
				case four_dimension_sync.StatusActive:
					reason = "集群同步恢复"
					status = layer.RegionStatusActive
				}
				regionMgr.UpdateRegionStatus(status, reason)
				metrics.RecordFourDimensionRegionStatus(update.AccountID, update.ProductID, update.Region, regionStatusToString(status))
			}
		}
	}
}

func (c *FourDimensionCollector) updateResourceStatus(update four_dimension_sync.FourDimensionUpdate) {
	accountKey := update.AccountID

	c.accountMgrMu.RLock()
	defer c.accountMgrMu.RUnlock()

	if mgr, ok := c.accountManagers[accountKey]; ok {
		productMgr, err := mgr.GetProductManager(update.ProductID)
		if err == nil && productMgr != nil {
			regionMgr, err := productMgr.GetRegionManager(update.Region)
			if err == nil && regionMgr != nil {
				resourceMgr, err := regionMgr.GetResourceManager(update.ResourceID)
				if err == nil && resourceMgr != nil {
					switch update.Status {
					case four_dimension_sync.StatusDisabled:
						resourceMgr.UpdateResourceStatus(layer.ResourceStatusDisabled, "集群同步禁用")
						metrics.RecordFourDimensionResourceStatus(update.AccountID, update.ProductID, update.Region, update.ResourceID, "disabled")
					case four_dimension_sync.StatusDegraded:
						resourceMgr.UpdateResourceStatus(layer.ResourceStatusDegraded, "集群同步降级")
						metrics.RecordFourDimensionResourceStatus(update.AccountID, update.ProductID, update.Region, update.ResourceID, "degraded")
						metrics.RecordFourDimensionResourceDegraded(update.AccountID, update.ProductID, update.Region, update.ResourceID, "cluster_sync")
					case four_dimension_sync.StatusActive:
						resourceMgr.UpdateResourceStatus(layer.ResourceStatusActive, "集群同步恢复")
						metrics.RecordFourDimensionResourceStatus(update.AccountID, update.ProductID, update.Region, update.ResourceID, "active")
					}
				}
			}
		}
	}
}

// Collect 执行采集（根据配置的采集模式）
func (c *FourDimensionCollector) Collect() {
	if atomic.LoadInt32(&c.stopped) == 1 {
		return
	}

	c.statusLock.Lock()
	c.status.LastStart = time.Now()
	c.statusLock.Unlock()

	accounts := c.getAccounts()
	var wg sync.WaitGroup
	// 账号级并发控制（使用全局 maxConcurrency）
	concurrency := c.maxConcurrency
	if concurrency <= 0 {
		concurrency = 20 // 默认值
	}
	sem := make(chan struct{}, concurrency)

	for _, account := range accounts {
		wg.Add(1)
		// 限制并发数：必须在 go func 外部获取信号量
		sem <- struct{}{}
		go func(acc config.CloudAccount) {
			defer wg.Done()
			defer func() { <-sem }()

			defer func() {
				if r := recover(); r != nil {
					c.zapLogger.Error("采集账号 panic",
						zap.String("account_id", acc.AccountID),
						zap.String("provider", acc.Provider),
						zap.Any("panic", r),
					)
				}
			}()

			if err := c.CollectAccount(acc); err != nil {
				c.zapLogger.Error("采集账号失败",
					zap.String("account_id", acc.AccountID),
					zap.String("provider", acc.Provider),
					zap.Error(err),
				)
			}
		}(account)
	}
	wg.Wait()

	c.statusLock.Lock()
	c.status.LastEnd = time.Now()
	c.status.LastDuration = c.status.LastEnd.Sub(c.status.LastStart)
	c.statusLock.Unlock()
}

// CollectFiltered 按条件采集
func (c *FourDimensionCollector) CollectFiltered(provider, resource string) {
	if atomic.LoadInt32(&c.stopped) == 1 {
		return
	}

	c.statusLock.Lock()
	c.status.LastStart = time.Now()
	c.statusLock.Unlock()

	accounts := c.getAccounts()
	var wg sync.WaitGroup
	sem := make(chan struct{}, c.maxConcurrency)

	for _, account := range accounts {
		if provider != "" && account.Provider != provider {
			continue
		}
		if resource != "" && len(account.Resources) > 0 {
			found := false
			for _, r := range account.Resources {
				if r == resource {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		wg.Add(1)
		go func(acc config.CloudAccount) {
			defer wg.Done()
			sem <- struct{}{}        // 获取令牌
			defer func() { <-sem }() // 释放令牌

			if err := c.CollectAccount(acc); err != nil {
				c.zapLogger.Error("采集账号失败",
					zap.String("account_id", acc.AccountID),
					zap.String("provider", acc.Provider),
					zap.Error(err),
				)
			}
		}(account)
	}

	wg.Wait()

	c.statusLock.Lock()
	c.status.LastEnd = time.Now()
	c.status.LastDuration = c.status.LastEnd.Sub(c.status.LastStart)
	c.statusLock.Unlock()
}

// CollectAccount 采集账号级指标
func (c *FourDimensionCollector) CollectAccount(account config.CloudAccount) error {
	if atomic.LoadInt32(&c.stopped) == 1 {
		return fmt.Errorf("collector已停止")
	}

	mgr := c.getOrCreateAccountManager(account)

	if mgr.ShouldSkipAccount() {
		metrics.RecordFourDimensionAccountSkip(account.AccountID, account.Provider, "account_disabled")
		return nil
	}

	// 检查 adapter 是否存在
	if _, ok := c.getAdapter(account.Provider); !ok {
		metrics.RecordFourDimensionAccountSkip(account.AccountID, account.Provider, "adapter_not_found")
		return nil
	}

	ctxLog := logger.NewContextLogger("FourDimensionCollector", "provider", account.Provider, "account_id", account.AccountID)
	ctxLog.Infof("FourDimensionCollector 开始采集账号级指标")

	// 1. 解析需要采集的产品（资源类型）
	resources := account.Resources
	if len(resources) == 0 || (len(resources) == 1 && resources[0] == "*") {
		if p, ok := c.cloudProviders[account.Provider]; ok {
			resources = p.GetDefaultResources()
		}
	}

	// 2. 遍历产品进行采集
	var wg sync.WaitGroup
	for _, resource := range resources {
		wg.Add(1)
		go func(productID string) {
			defer wg.Done()
			if err := c.CollectProduct(account, productID); err != nil {
				ctxLog.Warnf("采集产品失败: %s, %v", productID, err)
			}
		}(resource)
	}
	wg.Wait()

	metrics.RecordFourDimensionAccountStatus(account.AccountID, account.Provider, "active")

	return nil
}

// CollectProduct 采集产品级指标
func (c *FourDimensionCollector) CollectProduct(account config.CloudAccount, productID string) error {
	ctx := context.Background()

	if atomic.LoadInt32(&c.stopped) == 1 {
		return fmt.Errorf("collector已停止")
	}

	mgr := c.getOrCreateAccountManager(account)
	productMgr, err := mgr.GetProductManager(productID)
	if err != nil {
		return err
	}

	if productMgr.ShouldSkipProduct() {
		metrics.RecordFourDimensionProductSkip(account.AccountID, productID, "product_disabled")
		return nil
	}

	adapter, ok := c.getAdapter(account.Provider)
	if !ok {
		metrics.RecordFourDimensionProductSkip(account.AccountID, productID, "adapter_not_found")
		return nil
	}

	// 获取区域列表
	regions, err := adapter.GetRegions(ctx, account)
	if err != nil {
		productMgr.UpdateProductStatus(layer.ProductStatusDegraded, err.Error())
		metrics.RecordFourDimensionProductStatus(account.AccountID, productID, "degraded")
		return err
	}

	// 遍历区域进行采集
	var wg sync.WaitGroup
	// 区域级并发控制
	regConc := 1
	if c.cfg != nil && c.cfg.Server != nil && c.cfg.Server.RegionConcurrency > 0 {
		regConc = c.cfg.Server.RegionConcurrency
	}
	sem := make(chan struct{}, regConc)

	for _, region := range regions {
		wg.Add(1)
		go func(r string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			defer func() {
				if r := recover(); r != nil {
					c.zapLogger.Error("采集区域 panic", zap.String("region", r.(string)), zap.Any("panic", r))
				}
			}()

			if err := c.CollectRegion(account, productID, r); err != nil {
				// 区域级错误不直接导致产品级降级，除非所有区域都失败（这里暂不实现复杂的聚合逻辑）
				c.zapLogger.Warn("采集区域失败", zap.String("region", r), zap.Error(err))
			}
		}(region)
	}
	wg.Wait()

	productMgr.UpdateProductStatus(layer.ProductStatusActive, "")
	metrics.RecordFourDimensionProductStatus(account.AccountID, productID, "active")

	return nil
}

// CollectRegion 采集区域级指标
func (c *FourDimensionCollector) CollectRegion(account config.CloudAccount, productID, region string) error {
	ctx := context.Background()

	if atomic.LoadInt32(&c.stopped) == 1 {
		return fmt.Errorf("collector已停止")
	}

	mgr := c.getOrCreateAccountManager(account)
	productMgr, err := mgr.GetProductManager(productID)
	if err != nil {
		return err
	}

	regionMgr, err := productMgr.GetRegionManager(region)
	if err != nil {
		return err
	}

	if regionMgr.ShouldSkipRegion() {
		metrics.RecordFourDimensionRegionSkip(account.AccountID, productID, region, "region_disabled")
		return nil
	}

	adapter, ok := c.getAdapter(account.Provider)
	if !ok {
		metrics.RecordFourDimensionRegionSkip(account.AccountID, productID, region, "adapter_not_found")
		return nil
	}

	if err := adapter.CollectRegionMetrics(ctx, account, productID, region); err != nil {
		regionMgr.UpdateRegionStatus(layer.RegionStatusDegraded, err.Error())
		metrics.RecordFourDimensionRegionStatus(account.AccountID, productID, region, "degraded")
		return err
	}

	regionMgr.UpdateRegionStatus(layer.RegionStatusActive, "")
	metrics.RecordFourDimensionRegionStatus(account.AccountID, productID, region, "active")

	return nil
}

// CollectResource 采集资源级指标
func (c *FourDimensionCollector) CollectResource(account config.CloudAccount, productID, region, resourceID string) error {
	ctx := context.Background()

	if atomic.LoadInt32(&c.stopped) == 1 {
		return fmt.Errorf("collector已停止")
	}

	mgr := c.getOrCreateAccountManager(account)
	productMgr, err := mgr.GetProductManager(productID)
	if err != nil {
		return err
	}

	regionMgr, err := productMgr.GetRegionManager(region)
	if err != nil {
		return err
	}

	resourceMgr, err := regionMgr.GetResourceManager(resourceID)
	if err != nil {
		return err
	}

	if resourceMgr.ShouldSkipResource() {
		metrics.RecordFourDimensionResourceSkip(account.AccountID, productID, region, resourceID, "resource_disabled")
		return nil
	}

	adapter, ok := c.getAdapter(account.Provider)
	if !ok {
		metrics.RecordFourDimensionResourceSkip(account.AccountID, productID, region, resourceID, "adapter_not_found")
		return nil
	}

	if err := adapter.CollectResourceMetrics(ctx, account, productID, region, resourceID); err != nil {
		resourceMgr.UpdateResourceStatus(layer.ResourceStatusDegraded, err.Error())
		metrics.RecordFourDimensionResourceStatus(account.AccountID, productID, region, resourceID, "degraded")
		return err
	}

	resourceMgr.UpdateResourceStatus(layer.ResourceStatusActive, "")
	metrics.RecordFourDimensionResourceStatus(account.AccountID, productID, region, resourceID, "active")

	return nil
}

// GetStatus 获取采集器状态
func (c *FourDimensionCollector) GetStatus() FourDimensionStatus {
	c.statusLock.RLock()
	defer c.statusLock.Unlock()
	return FourDimensionStatus{
		LastStart:    c.status.LastStart,
		LastEnd:      c.status.LastEnd,
		LastDuration: c.status.LastDuration,
	}
}

// UpdateFromPeer 从集群同步更新状态
func (c *FourDimensionCollector) UpdateFromPeer(update four_dimension_sync.FourDimensionUpdate) {
	switch update.Dimension {
	case four_dimension_sync.DimensionAccount:
		c.updateAccountStatus(update)
	case four_dimension_sync.DimensionProduct:
		c.updateProductStatus(update)
	case four_dimension_sync.DimensionRegion:
		c.updateRegionStatus(update)
	case four_dimension_sync.DimensionResource:
		c.updateResourceStatus(update)
	}
}

// Stop 停止采集器
func (c *FourDimensionCollector) Stop() {
	atomic.StoreInt32(&c.stopped, 1)
}

// GetDegradationManager 获取降级管理器
func (c *FourDimensionCollector) GetDegradationManager() *degradation.FourDimensionDegradationManager {
	return c.degradeMgr
}
