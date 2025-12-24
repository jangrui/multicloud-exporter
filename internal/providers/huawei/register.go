// 华为云 Provider 注册
package huawei

import (
	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/discovery"
	"multicloud-exporter/internal/providers"
)

// GetDefaultResources 返回华为云默认采集的资源类型
func (h *Collector) GetDefaultResources() []string {
	return []string{"clb", "s3"}
}

func init() {
	providers.Register("huawei", func(cfg *config.Config, mgr *discovery.Manager) providers.Provider {
		return NewCollector(cfg, mgr)
	})
}
