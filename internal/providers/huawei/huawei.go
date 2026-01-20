// 华为云采集器：按配置采集 ELB、OBS 等资源的监控指标
package huawei

import (
	"strings"
	"sync"
	"time"

	"multicloud-exporter/internal/cluster"
	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/discovery"
	"multicloud-exporter/internal/logger"
	"multicloud-exporter/internal/metrics"
	providerscommon "multicloud-exporter/internal/providers/common"
	"multicloud-exporter/internal/utils"
)

// 华为云默认区域列表
var defaultHuaweiRegions = []string{
	"cn-north-4",     // 北京四
	"cn-north-1",     // 北京一
	"cn-east-3",      // 上海一
	"cn-east-2",      // 上海二
	"cn-south-1",     // 广州
	"cn-southwest-2", // 成都一
	"ap-southeast-1", // 香港
	"ap-southeast-2", // 雅谷
	"ap-southeast-3", // 新加坡
}

const (
	HuaweiProductELB = "elb"
	HuaweiProductOBS = "obs"
)

type Collector struct {
	cfg                   *config.Config
	disc                  *discovery.Manager
	resCache              map[string]resCacheEntry
	cacheMu               sync.RWMutex
	clientFactory         ClientFactory
	productRegionManagers map[string]providerscommon.RegionManager
	rmMu                  sync.RWMutex
	degradeMgr            *providerscommon.Manager
}

type resCacheEntry struct {
	IDs       []string
	UpdatedAt time.Time
}

var (
	productToNamespaceMapHuawei = map[string]string{
		HuaweiProductELB: "SYS.ELB",
		HuaweiProductOBS: "SYS.OBS",
	}
)

// NewCollector 创建华为云采集器实例
func NewCollector(cfg *config.Config, mgr *discovery.Manager, clusterMgr *cluster.SyncManager) *Collector {
	c := &Collector{
		cfg:                   cfg,
		disc:                  mgr,
		resCache:              make(map[string]resCacheEntry),
		clientFactory:         &defaultClientFactory{},
		productRegionManagers: make(map[string]providerscommon.RegionManager),
	}

	// 为每个产品创建独立的 RegionManager
	if cfg != nil && cfg.GetServer() != nil && cfg.GetServer().RegionDiscovery != nil {
		for product := range productToNamespaceMapHuawei {
			rm := providerscommon.NewRegionManager(providerscommon.RegionDiscoveryConfig{
				Enabled:           cfg.GetServer().RegionDiscovery.Enabled,
				DiscoveryInterval: parseDuration(cfg.GetServer().RegionDiscovery.DiscoveryInterval),
				EmptyThreshold:    cfg.GetServer().RegionDiscovery.EmptyThreshold,
			})

			if clusterMgr != nil {
				rm.SetBroadcaster(clusterMgr, "huawei", product)
				clusterMgr.RegisterProductRegionManager("huawei", product, rm)
			}

			rm.StartRediscoveryScheduler()
			c.productRegionManagers[product] = rm
		}
	}

	return c
}

// getProductRegionManager 获取指定产品的 RegionManager
func (h *Collector) getProductRegionManager(product string) providerscommon.RegionManager {
	h.rmMu.RLock()
	defer h.rmMu.RUnlock()
	return h.productRegionManagers[product]
}

// parseDuration 解析时长字符串为 time.Duration
func parseDuration(s string) time.Duration {
	if s == "" {
		return 0
	}
	if d, err := time.ParseDuration(s); err == nil {
		return d
	}
	return 0
}

// Collect 根据账号配置遍历区域与资源类型并采集
// 注意：分片逻辑已下沉到产品级（collectELB/collectOBS），此处不做区域级分片
// 这样可以避免双重分片导致的任务丢失问题
// 注意：产品级状态隔离已实施，区域过滤在各产品的 listXXXIDs 方法中通过专属 RegionManager 进行
func (h *Collector) Collect(account config.CloudAccount) {
	regions := account.Regions
	if len(regions) == 0 || (len(regions) == 1 && regions[0] == "*") {
		regions = defaultHuaweiRegions
	}

	var wg sync.WaitGroup
	for _, region := range regions {
		wg.Add(1)
		go func(r string) {
			defer wg.Done()
			h.collectRegion(account, r)
		}(region)
	}
	wg.Wait()
}

// collectRegion 采集指定区域的资源
func (h *Collector) collectRegion(account config.CloudAccount, region string) {
	ctxLog := logger.NewContextLogger("Huawei", "account_id", account.AccountID, "region", region, "resource_type", "RegionCollector")
	ctxLog.Debugf("开始采集区域")
	for _, resource := range account.Resources {
		r := strings.ToLower(resource)
		if resource == "*" {
			h.collectELB(account, region)
			h.collectOBS(account, region)
		} else {
			switch r {
			case "clb", "elb":
				h.collectELB(account, region)
			case "s3", "obs":
				h.collectOBS(account, region)
			default:
				ctxLog := logger.NewContextLogger("Huawei", "account_id", account.AccountID, "region", region, "resource_type", resource)
				ctxLog.Warnf("资源类型尚未实现")
			}
		}
	}
}

// cacheKey 生成缓存键
func (h *Collector) cacheKey(account config.CloudAccount, region, namespace, rtype string) string {
	return account.AccountID + "|" + region + "|" + namespace + "|" + rtype
}

// getCachedIDs 获取缓存的资源 ID 列表
func (h *Collector) getCachedIDs(account config.CloudAccount, region, namespace, rtype string) ([]string, bool) {
	h.cacheMu.RLock()
	entry, ok := h.resCache[h.cacheKey(account, region, namespace, rtype)]
	h.cacheMu.RUnlock()
	if !ok || len(entry.IDs) == 0 {
		metrics.RecordCacheMiss("resource_discovery")
		return nil, false
	}
	ttlDur := time.Hour
	if server := h.cfg.GetServer(); server != nil {
		if server.DiscoveryTTL != "" {
			if d, err := utils.ParseDuration(server.DiscoveryTTL); err == nil {
				ttlDur = d
			}
		}
	} else if h.cfg != nil && h.cfg.Server != nil {
		if h.cfg.Server.DiscoveryTTL != "" {
			if d, err := utils.ParseDuration(h.cfg.Server.DiscoveryTTL); err == nil {
				ttlDur = d
			}
		}
	}
	if time.Since(entry.UpdatedAt) > ttlDur {
		metrics.RecordCacheMiss("resource_discovery")
		return nil, false
	}
	metrics.RecordCacheHit("resource_discovery")
	return entry.IDs, true
}

// setCachedIDs 设置缓存的资源 ID 列表
func (h *Collector) setCachedIDs(account config.CloudAccount, region, namespace, rtype string, ids []string) {
	// 不缓存空结果，避免 API 临时故障导致资源永久不可见
	if len(ids) == 0 {
		logger.NewContextLogger("Huawei", "account_id", account.AccountID, "region", region, "namespace", namespace, "rtype", rtype).Debugf("资源列表为空，跳过缓存（允许下次重新尝试）")
		return
	}
	h.cacheMu.Lock()
	h.resCache[h.cacheKey(account, region, namespace, rtype)] = resCacheEntry{IDs: ids, UpdatedAt: time.Now()}
	h.cacheMu.Unlock()
}
