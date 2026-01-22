package providers

import (
	"context"
	"sync"

	"go.uber.org/zap"

	"multicloud-exporter/internal/cluster"
	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/discovery"
	"multicloud-exporter/internal/providers/common"
)

// Provider 定义云厂商采集接口
type Provider interface {
	GetDefaultResources() []string
	SupportsInternalSharding() bool
}

// DegradableProvider 支持降级管理的 Provider 接口
type DegradableProvider interface {
	Provider
	SetDegradationManager(mgr *common.Manager)
}

// FourDimensionAdapter 四维架构适配器接口
type FourDimensionAdapter interface {
	CollectAccountMetrics(ctx context.Context, account config.CloudAccount) error
	CollectProductMetrics(ctx context.Context, account config.CloudAccount, productID string) error
	CollectRegionMetrics(ctx context.Context, account config.CloudAccount, productID, region string) error
	CollectResourceMetrics(ctx context.Context, account config.CloudAccount, productID, region, resourceID string) error
	DiscoverResources(ctx context.Context, account config.CloudAccount) (map[string]map[string][]string, error)
	GetRegions(ctx context.Context, account config.CloudAccount) ([]string, error)
}

// Factory 创建 Provider 实例的工厂函数
type Factory func(cfg *config.Config, mgr *discovery.Manager, clusterMgr *cluster.SyncManager) Provider

// FourDimensionFactory 创建四维架构适配器的工厂函数
type FourDimensionFactory func(collector Provider, logger *zap.Logger) FourDimensionAdapter

var (
	registry              = make(map[string]Factory)
	fourDimensionRegistry = make(map[string]FourDimensionFactory)
	mu                    sync.RWMutex
)

// Register 注册云厂商 Provider
func Register(name string, factory Factory) {
	mu.Lock()
	defer mu.Unlock()
	registry[name] = factory
}

// GetFactory 获取指定云厂商的 Factory
func GetFactory(name string) (Factory, bool) {
	mu.RLock()
	defer mu.RUnlock()
	f, ok := registry[name]
	return f, ok
}

// GetAllProviders 获取所有已注册的云厂商名称
func GetAllProviders() []string {
	mu.RLock()
	defer mu.RUnlock()
	keys := make([]string, 0, len(registry))
	for k := range registry {
		keys = append(keys, k)
	}
	return keys
}

// RegisterFourDimensionAdapter 注册四维架构适配器
func RegisterFourDimensionAdapter(provider string, factory FourDimensionFactory) {
	mu.Lock()
	defer mu.Unlock()
	fourDimensionRegistry[provider] = factory
}

// GetFourDimensionAdapter 获取指定云厂商的四维架构适配器工厂函数
func GetFourDimensionAdapter(provider string) (FourDimensionFactory, bool) {
	mu.RLock()
	defer mu.RUnlock()
	f, ok := fourDimensionRegistry[provider]
	return f, ok
}
