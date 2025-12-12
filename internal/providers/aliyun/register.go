package aliyun

import (
	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/discovery"
	"multicloud-exporter/internal/providers"
)

// GetDefaultResources 返回阿里云默认采集的资源类型
func (a *Collector) GetDefaultResources() []string {
	return []string{"bwp", "clb", "s3", "alb", "nlb", "gwlb"}
}

func init() {
	providers.Register("aliyun", func(cfg *config.Config, mgr *discovery.Manager) providers.Provider {
		return NewCollector(cfg, mgr)
	})
}
