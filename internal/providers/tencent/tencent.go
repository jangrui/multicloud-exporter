// 腾讯云采集器：按配置采集 CVM 等资源的静态属性
package tencent

import (
	"log"
	"sync"
	"time"

	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/discovery"
	"multicloud-exporter/internal/metrics"
	"multicloud-exporter/internal/utils"

	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	cvm "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/cvm/v20170312"
)

type Collector struct {
	cfg      *config.Config
	disc     *discovery.Manager
	resCache map[string]resCacheEntry
	cacheMu  sync.RWMutex
}

type resCacheEntry struct {
	IDs       []string
	UpdatedAt time.Time
}

func NewCollector(cfg *config.Config, mgr *discovery.Manager) *Collector {
	return &Collector{
		cfg:      cfg,
		disc:     mgr,
		resCache: make(map[string]resCacheEntry),
	}
}

// Collect 根据账号配置遍历区域与资源类型并采集
func (t *Collector) Collect(account config.CloudAccount) {
	regions := account.Regions
	if len(regions) == 0 || (len(regions) == 1 && regions[0] == "*") {
		regions = []string{"ap-guangzhou", "ap-shanghai", "ap-beijing", "ap-chengdu", "ap-chongqing", "ap-nanjing", "ap-hongkong", "ap-singapore", "ap-tokyo", "ap-seoul"}
	}

	for _, region := range regions {
		for _, resource := range account.Resources {
			switch resource {
			case "cvm":
				t.collectCVM(account, region)
			case "cdb":
				t.collectCDB(account, region)
			case "redis":
				t.collectRedis(account, region)
			case "clb":
				t.collectCLB(account, region)
			case "slb":
				t.collectCLB(account, region)
			case "eip":
				t.collectEIP(account, region)
			case "bwp":
				t.collectBWP(account, region)
			default:
				log.Printf("Tencent resource type %s not implemented yet", resource)
			}
		}
	}
}

// collectCVM 采集 CVM 的 CPU、内存等基础信息
func (t *Collector) collectCVM(account config.CloudAccount, region string) {
	// Check cache for CVM to avoid frequent DescribeInstances
	if _, hit := t.getCachedIDs(account, region, "cvm", "cvm"); hit {
		return
	}

	credential := common.NewCredential(account.AccessKeyID, account.AccessKeySecret)
	cpf := profile.NewClientProfile()
	client, err := cvm.NewClient(credential, region, cpf)
	if err != nil {
		log.Printf("Tencent CVM client error: %v", err)
		return
	}

	request := cvm.NewDescribeInstancesRequest()
	response, err := client.DescribeInstances(request)
	if err != nil {
		log.Printf("Tencent CVM describe error: %v", err)
		return
	}

	var ids []string
	if response.Response.InstanceSet != nil {
		for _, instance := range response.Response.InstanceSet {
			if instance.InstanceId != nil {
				ids = append(ids, *instance.InstanceId)
			}
			metrics.ResourceMetric.WithLabelValues(
				"tencent",
				account.AccountID,
				region,
				"cvm",
				*instance.InstanceId,
				"cpu_cores",
			).Set(float64(*instance.CPU))

			metrics.ResourceMetric.WithLabelValues(
				"tencent",
				account.AccountID,
				region,
				"cvm",
				*instance.InstanceId,
				"memory_gb",
			).Set(float64(*instance.Memory))
		}
	}
	// Cache the discovery result
	t.setCachedIDs(account, region, "cvm", "cvm", ids)
	log.Printf("Tencent CVM enumerated account_id=%s region=%s count=%d", account.AccountID, region, len(ids))
}

func (t *Collector) collectCDB(account config.CloudAccount, region string) {
	log.Printf("Collecting Tencent CDB in region %s (not implemented)", region)
}

func (t *Collector) collectRedis(account config.CloudAccount, region string) {
	log.Printf("Collecting Tencent Redis in region %s (not implemented)", region)
}

func (t *Collector) collectCLB(account config.CloudAccount, region string) {
	if t.cfg == nil {
		return
	}
	var prods []config.Product
	if t.disc != nil {
		if ps, ok := t.disc.Get()["tencent"]; ok && len(ps) > 0 {
			prods = ps
		}
	}
	if len(prods) == 0 {
		return
	}
	for _, p := range prods {
		if p.Namespace != "QCE/CLB" {
			continue
		}
		vips := t.listCLBVips(account, region)
		if len(vips) == 0 {
			continue
		}
		t.fetchCLBMonitor(account, region, p, vips)
	}
}

func (t *Collector) collectEIP(account config.CloudAccount, region string) {
	log.Printf("Collecting Tencent EIP in region %s (not implemented)", region)
}

func (t *Collector) collectBWP(account config.CloudAccount, region string) {
	if t.cfg == nil {
		return
	}
	var prods []config.Product
	if t.disc != nil {
		if ps, ok := t.disc.Get()["tencent"]; ok && len(ps) > 0 {
			prods = ps
		}
	}
	if len(prods) == 0 {
		return
	}
	for _, p := range prods {
		if p.Namespace != "QCE/BWP" {
			continue
		}
		ids := t.listBWPIDs(account, region)
		if len(ids) == 0 {
			return
		}
		t.fetchBWPMonitor(account, region, p, ids)
	}
}

func (t *Collector) cacheKey(account config.CloudAccount, region, namespace, rtype string) string {
	return account.AccountID + "|" + region + "|" + namespace + "|" + rtype
}

func (t *Collector) getCachedIDs(account config.CloudAccount, region, namespace, rtype string) ([]string, bool) {
	t.cacheMu.RLock()
	entry, ok := t.resCache[t.cacheKey(account, region, namespace, rtype)]
	t.cacheMu.RUnlock()
	if !ok || len(entry.IDs) == 0 {
		return nil, false
	}
	ttlDur := time.Hour
	if t.cfg != nil && t.cfg.ServerConf != nil {
		if t.cfg.ServerConf.DiscoveryTTL != "" {
			if d, err := utils.ParseDuration(t.cfg.ServerConf.DiscoveryTTL); err == nil {
				ttlDur = d
			}
		}
	} else if t.cfg != nil && t.cfg.Server != nil {
		if t.cfg.Server.DiscoveryTTL != "" {
			if d, err := utils.ParseDuration(t.cfg.Server.DiscoveryTTL); err == nil {
				ttlDur = d
			}
		}
	}
	if time.Since(entry.UpdatedAt) > ttlDur {
		return nil, false
	}
	return entry.IDs, true
}

func (t *Collector) setCachedIDs(account config.CloudAccount, region, namespace, rtype string, ids []string) {
	t.cacheMu.Lock()
	t.resCache[t.cacheKey(account, region, namespace, rtype)] = resCacheEntry{IDs: ids, UpdatedAt: time.Now()}
	t.cacheMu.Unlock()
}
