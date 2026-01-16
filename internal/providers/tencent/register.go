package tencent

import (
	"multicloud-exporter/internal/cluster"
	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/discovery"
	"multicloud-exporter/internal/providers"
)

// GetDefaultResources 返回腾讯云默认采集的资源类型
func (t *Collector) GetDefaultResources() []string {
	return []string{"clb", "bwp", "s3", "gwlb"}
}

func init() {
	providers.Register("tencent", func(cfg *config.Config, mgr *discovery.Manager, clusterMgr *cluster.SyncManager) providers.Provider {
		return NewCollector(cfg, mgr, clusterMgr)
	})
}
