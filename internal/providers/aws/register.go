package aws

import (
	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/discovery"
	"multicloud-exporter/internal/providers"
)

// GetDefaultResources 返回 AWS 默认采集的资源类型
func (c *Collector) GetDefaultResources() []string {
	return []string{"s3"}
}

func init() {
	providers.Register("aws", func(cfg *config.Config, mgr *discovery.Manager) providers.Provider {
		return NewCollector(cfg, mgr)
	})
}
