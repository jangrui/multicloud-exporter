// 华为云采集器：按配置采集 ELB、OBS 等资源的监控指标
package huawei

import (
	"strings"
	"sync"
	"time"

	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/discovery"
	"multicloud-exporter/internal/logger"
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
	"cn-southwest-2", // 贵阳一
	"ap-southeast-1", // 香港
	"ap-southeast-2", // 曼谷
	"ap-southeast-3", // 新加坡
}

// Collector 封装华为云资源采集逻辑
type Collector struct {
	cfg           *config.Config
	disc          *discovery.Manager
	resCache      map[string]resCacheEntry
	cacheMu       sync.RWMutex
	clientFactory ClientFactory
	regionManager providerscommon.RegionManager
}

type resCacheEntry struct {
	IDs       []string
	UpdatedAt time.Time
}

// NewCollector 创建华为云采集器实例
func NewCollector(cfg *config.Config, mgr *discovery.Manager) *Collector {
	return &Collector{
		cfg:           cfg,
		disc:          mgr,
		resCache:      make(map[string]resCacheEntry),
		clientFactory: &defaultClientFactory{},
	}
}

// Collect 根据账号配置遍历区域与资源类型并采集
// 注意：分片逻辑已下沉到产品级（collectELB/collectOBS），此处不做区域级分片
// 这样可以避免双重分片导致的任务丢失问题
func (h *Collector) Collect(account config.CloudAccount) {
	regions := account.Regions
	if len(regions) == 0 || (len(regions) == 1 && regions[0] == "*") {
		regions = defaultHuaweiRegions
	}

	// 使用区域管理器进行智能过滤
	if h.regionManager != nil {
		activeRegions := h.regionManager.GetActiveRegions(account.AccountID, regions)
		logger.Log.Infof("华为云智能区域选择: 总=%d 活跃=%d 账号ID=%s",
			len(regions), len(activeRegions), account.AccountID)
		regions = activeRegions
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
	logger.Log.Debugf("开始采集 Huawei 区域 %s", region)
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
				logger.Log.Warnf("Huawei 资源类型 %s 尚未实现", resource)
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
		return nil, false
	}
	return entry.IDs, true
}

// setCachedIDs 设置缓存的资源 ID 列表
func (h *Collector) setCachedIDs(account config.CloudAccount, region, namespace, rtype string, ids []string) {
	h.cacheMu.Lock()
	h.resCache[h.cacheKey(account, region, namespace, rtype)] = resCacheEntry{IDs: ids, UpdatedAt: time.Now()}
	h.cacheMu.Unlock()
}
