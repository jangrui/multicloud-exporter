// 华为云 Provider 注册
package huawei

import (
	"multicloud-exporter/internal/cluster"
	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/discovery"
	"multicloud-exporter/internal/providers"
)

// GetDefaultResources 返回华为云默认采集的资源类型
func (h *Collector) GetDefaultResources() []string {
	return []string{"clb", "s3"}
}

// SupportsInternalSharding 返回是否支持内部分片
// 华为云在 collectELB 等方法中实现了产品级分片（使用 utils.ShouldProcess）
func (h *Collector) SupportsInternalSharding() bool {
	return true
}

func init() {
	providers.Register("huawei", func(cfg *config.Config, mgr *discovery.Manager, clusterMgr *cluster.SyncManager) providers.Provider {
		return NewCollector(cfg, mgr, clusterMgr)
	})
}
