package tencent

import (
	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/discovery"
	"multicloud-exporter/internal/providers"
)

// GetDefaultResources 返回腾讯云默认采集的资源类型
func (t *Collector) GetDefaultResources() []string {
	return []string{"lb", "bwp"}
}

func init() {
	providers.Register("tencent", func(cfg *config.Config, mgr *discovery.Manager) providers.Provider {
		return NewCollector(cfg, mgr)
	})
}
