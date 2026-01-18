package tencent

import (
	"multicloud-exporter/internal/cluster"
	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/discovery"
	"multicloud-exporter/internal/providers"
	providerscommon "multicloud-exporter/internal/providers/common"
)

// GetDefaultResources 返回腾讯云默认采集的资源类型
func (t *Collector) GetDefaultResources() []string {
	return []string{"clb", "bwp", "s3", "gwlb"}
}

// SupportsInternalSharding 返回是否支持内部分片
// 腾讯云在 collectCLB/collectBWP 等方法中实现了产品级分片（使用 utils.ShouldProcess）
func (t *Collector) SupportsInternalSharding() bool {
	return true
}

// SetDegradationManager 设置降级管理器
func (t *Collector) SetDegradationManager(mgr *providerscommon.Manager) {
	t.degradeMgr = mgr
}

func init() {
	providers.Register("tencent", func(cfg *config.Config, mgr *discovery.Manager, clusterMgr *cluster.SyncManager) providers.Provider {
		return NewCollector(cfg, mgr, clusterMgr)
	})
}
