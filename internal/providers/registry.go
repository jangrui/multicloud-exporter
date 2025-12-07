package providers

import (
	"sync"

	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/discovery"
)

// Provider 定义云厂商采集接口
type Provider interface {
	Collect(account config.CloudAccount)
	GetDefaultResources() []string
}

// Factory 创建 Provider 实例的工厂函数
type Factory func(cfg *config.Config, mgr *discovery.Manager) Provider

var (
	registry = make(map[string]Factory)
	mu       sync.RWMutex
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
