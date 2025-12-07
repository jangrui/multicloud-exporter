package tencent

import (
	"strings"
	"sync"
	"time"

	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/discovery"
	"multicloud-exporter/internal/logger"
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

func (t *Collector) Collect(account config.CloudAccount) {
	regions := account.Regions
	if len(regions) == 0 {
		regions = []string{"ap-guangzhou"} // Default region
	}

	var wg sync.WaitGroup
	for _, region := range regions {
		wg.Add(1)
		go func(r string) {
			defer wg.Done()
			t.collectRegion(account, r)
		}(region)
	}
	wg.Wait()
}

func (t *Collector) collectRegion(account config.CloudAccount, region string) {
	logger.Log.Debugf("Start collecting Tencent region %s", region)
	for _, resource := range account.Resources {
		if resource == "*" {
			// Collect all supported resources
			t.collectCVM(account, region)
			t.collectCDB(account, region)
			t.collectRedis(account, region)
			t.collectCLB(account, region)
			t.collectEIP(account, region)
			t.collectBWP(account, region)
		} else {
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
				logger.Log.Warnf("Tencent resource type %s not implemented yet", resource)
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
		logger.Log.Errorf("Tencent CVM client error: %v", err)
		return
	}

	request := cvm.NewDescribeInstancesRequest()
	start := time.Now()
	response, err := client.DescribeInstances(request)
	if err != nil {
		status := classifyTencentError(err)
		metrics.RequestTotal.WithLabelValues("tencent", "DescribeInstances", status).Inc()
		logger.Log.Errorf("Tencent CVM describe error: %v", err)
		return
	}
	metrics.RequestTotal.WithLabelValues("tencent", "DescribeInstances", "success").Inc()
	metrics.RequestDuration.WithLabelValues("tencent", "DescribeInstances").Observe(time.Since(start).Seconds())

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
	logger.Log.Debugf("Tencent CVM enumerated account_id=%s region=%s count=%d", account.AccountID, region, len(ids))
}

func (t *Collector) collectCDB(account config.CloudAccount, region string) {
	logger.Log.Warnf("Collecting Tencent CDB in region %s (not implemented)", region)
}

func (t *Collector) collectRedis(account config.CloudAccount, region string) {
	logger.Log.Warnf("Collecting Tencent Redis in region %s (not implemented)", region)
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
	logger.Log.Warnf("Collecting Tencent EIP in region %s (not implemented)", region)
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

func classifyTencentError(err error) string {
	msg := err.Error()
	if strings.Contains(msg, "AuthFailure") || strings.Contains(msg, "InvalidCredential") {
		return "auth_error"
	}
	if strings.Contains(msg, "RequestLimitExceeded") {
		return "limit_error"
	}
	if strings.Contains(msg, "timeout") || strings.Contains(msg, "network") {
		return "network_error"
	}
	return "error"
}
