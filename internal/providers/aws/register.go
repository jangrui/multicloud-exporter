package aws

import (
	"multicloud-exporter/internal/cluster"
	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/discovery"
	"multicloud-exporter/internal/providers"
	providerscommon "multicloud-exporter/internal/providers/common"
)

// GetDefaultResources 返回 AWS 默认采集的资源类型
func (c *Collector) GetDefaultResources() []string {
	return []string{"s3", "alb", "clb", "nlb", "gwlb"}
}

// SupportsInternalSharding 返回是否支持内部分片
// AWS 在 collectLBGeneric/collectS3 等方法中实现了产品级分片（使用 utils.ShouldProcess）
func (c *Collector) SupportsInternalSharding() bool {
	return true
}

// SetDegradationManager 设置降级管理器
func (c *Collector) SetDegradationManager(mgr *providerscommon.Manager) {
	c.degradeMgr = mgr
}

func init() {
	providers.Register("aws", func(cfg *config.Config, mgr *discovery.Manager, clusterMgr *cluster.SyncManager) providers.Provider {
		return NewCollector(cfg, mgr, clusterMgr)
	})
}
