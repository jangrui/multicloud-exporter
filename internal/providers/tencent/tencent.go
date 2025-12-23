package tencent

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/discovery"
	"multicloud-exporter/internal/logger"
	"multicloud-exporter/internal/metrics"
	providerscommon "multicloud-exporter/internal/providers/common"
	"multicloud-exporter/internal/utils"

	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	cvm "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/cvm/v20170312"
	monitor "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/monitor/v20180724"
)

type Collector struct {
	cfg           *config.Config
	disc          *discovery.Manager
	resCache      map[string]resCacheEntry
	cacheMu       sync.RWMutex
	clientFactory ClientFactory
}

type resCacheEntry struct {
	IDs       []string
	UpdatedAt time.Time
}

func NewCollector(cfg *config.Config, mgr *discovery.Manager) *Collector {
	return &Collector{
		cfg:           cfg,
		disc:          mgr,
		resCache:      make(map[string]resCacheEntry),
		clientFactory: &defaultClientFactory{},
	}
}

func (t *Collector) Collect(account config.CloudAccount) {
	regions := account.Regions
	if len(regions) == 0 || (len(regions) == 1 && regions[0] == "*") {
		regions = t.getAllRegions(account)
		if len(regions) == 0 {
			regions = []string{"ap-guangzhou"}
		}
	}

	var wg sync.WaitGroup
	wTotal, wIndex := utils.ClusterConfig()
	for _, region := range regions {
		key := account.AccountID + "|" + region
		if !utils.ShouldProcess(key, wTotal, wIndex) {
			continue
		}
		wg.Add(1)
		go func(r string) {
			defer wg.Done()
			t.collectRegion(account, r)
		}(region)
	}
	wg.Wait()
}

// getAllRegions 通过 CVM DescribeRegions 自动枚举腾讯云可用区域
func (t *Collector) getAllRegions(account config.CloudAccount) []string {
	client, err := t.clientFactory.NewCVMClient("ap-guangzhou", account.AccessKeyID, account.AccessKeySecret)
	if err != nil {
		return []string{"ap-guangzhou"}
	}
	req := cvm.NewDescribeRegionsRequest()
	start := time.Now()
	var resp *cvm.DescribeRegionsResponse
	var callErr error
	for attempt := 0; attempt < 3; attempt++ {
		resp, callErr = client.DescribeRegions(req)
		if callErr == nil && resp != nil && resp.Response != nil && resp.Response.RegionSet != nil {
			metrics.RequestTotal.WithLabelValues("tencent", "DescribeRegions", "success").Inc()
			metrics.RecordRequest("tencent", "DescribeRegions", "success")
			metrics.RequestDuration.WithLabelValues("tencent", "DescribeRegions").Observe(time.Since(start).Seconds())
			break
		}
		if callErr != nil {
			status := providerscommon.ClassifyTencentError(callErr)
			metrics.RequestTotal.WithLabelValues("tencent", "DescribeRegions", status).Inc()
			metrics.RecordRequest("tencent", "DescribeRegions", status)
			if status == "limit_error" {
				// 记录限流指标
				metrics.RateLimitTotal.WithLabelValues("tencent", "DescribeRegions").Inc()
			}
			if status == "auth_error" {
				break
			}
			time.Sleep(time.Duration(200*(attempt+1)) * time.Millisecond)
		} else {
			break
		}
	}
	if callErr != nil || resp == nil || resp.Response == nil || resp.Response.RegionSet == nil {
		def := os.Getenv("DEFAULT_REGIONS")
		if def != "" {
			parts := strings.Split(def, ",")
			var out []string
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if p != "" {
					out = append(out, p)
				}
			}
			if len(out) > 0 {
				return out
			}
		}
		return []string{"ap-guangzhou"}
	}
	var regions []string
	for _, r := range resp.Response.RegionSet {
		if r != nil && r.Region != nil {
			regions = append(regions, *r.Region)
		}
	}
	if len(regions) == 0 {
		regions = []string{"ap-guangzhou"}
	}
	return regions
}

func (t *Collector) collectRegion(account config.CloudAccount, region string) {
	logger.Log.Debugf("开始采集 Tencent 区域 %s", region)
	for _, resource := range account.Resources {
		r := strings.ToLower(resource)
		if resource == "*" {
			// Collect all supported resources
			t.collectCLB(account, region)
			t.collectBWP(account, region)
			t.collectCOS(account, region)
		} else {
			switch r {
			case "clb":
				t.collectCLB(account, region)
			case "bwp":
				t.collectBWP(account, region)
			case "s3":
				t.collectCOS(account, region)
			case "cos":
				t.collectCOS(account, region)
			case "gwlb":
				t.collectGWLB(account, region)
			default:
				logger.Log.Warnf("Tencent 资源类型 %s 尚未实现", resource)
			}
		}
	}
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
	// 产品级分片：获取集群配置用于产品级分片判断
	wTotal, wIndex := utils.ClusterConfig()
	for _, p := range prods {
		if p.Namespace != "QCE/LB" {
			continue
		}
		// 产品级分片判断：只有当前 Pod 应该处理的产品才进行采集
		// 分片键格式：AccountID|Region|Namespace
		productKey := account.AccountID + "|" + region + "|" + p.Namespace
		if !utils.ShouldProcess(productKey, wTotal, wIndex) {
			logger.Log.Debugf("Tencent CLB 产品跳过（分片不匹配）account=%s region=%s namespace=%s", account.AccountID, region, p.Namespace)
			continue
		}
		vips := t.listCLBVips(account, region)
		if len(vips) == 0 {
			continue
		}
		t.fetchCLBMonitor(account, region, p, vips)
	}
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
	// 产品级分片：获取集群配置用于产品级分片判断
	wTotal, wIndex := utils.ClusterConfig()
	for _, p := range prods {
		if p.Namespace != "QCE/BWP" {
			continue
		}
		// 产品级分片判断：只有当前 Pod 应该处理的产品才进行采集
		// 分片键格式：AccountID|Region|Namespace
		productKey := account.AccountID + "|" + region + "|" + p.Namespace
		if !utils.ShouldProcess(productKey, wTotal, wIndex) {
			logger.Log.Debugf("Tencent BWP 产品跳过（分片不匹配）account=%s region=%s namespace=%s", account.AccountID, region, p.Namespace)
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
	if server := t.cfg.GetServer(); server != nil {
		if server.DiscoveryTTL != "" {
			if d, err := utils.ParseDuration(server.DiscoveryTTL); err == nil {
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

var (
	periodMu                sync.RWMutex
	periodCache             = make(map[string]int64)
	describeBaseMetricsJSON = func(region, ak, sk, namespace string) ([]byte, error) {
		cred := common.NewCredential(ak, sk)
		client, err := monitor.NewClient(cred, region, profile.NewClientProfile())
		if err != nil {
			return nil, err
		}
		req := monitor.NewDescribeBaseMetricsRequest()
		req.Namespace = common.StringPtr(namespace)
		start := time.Now()
		var resp *monitor.DescribeBaseMetricsResponse
		var callErr error
		for attempt := 0; attempt < 3; attempt++ {
			resp, callErr = client.DescribeBaseMetrics(req)
			if callErr == nil && resp != nil && resp.Response != nil {
				metrics.RequestTotal.WithLabelValues("tencent", "DescribeBaseMetrics", "success").Inc()
				metrics.RequestDuration.WithLabelValues("tencent", "DescribeBaseMetrics").Observe(time.Since(start).Seconds())
				metrics.RecordRequest("tencent", "DescribeBaseMetrics", "success")
				break
			}
			if callErr != nil {
				status := providerscommon.ClassifyTencentError(callErr)
				metrics.RequestTotal.WithLabelValues("tencent", "DescribeBaseMetrics", status).Inc()
				metrics.RecordRequest("tencent", "DescribeBaseMetrics", status)
				if status == "limit_error" {
					// 记录限流指标
					metrics.RateLimitTotal.WithLabelValues("tencent", "DescribeBaseMetrics").Inc()
				}
				if status == "auth_error" {
					return nil, callErr
				}
				time.Sleep(time.Duration(200*(attempt+1)) * time.Millisecond)
			} else {
				return nil, callErr
			}
		}
		if callErr != nil || resp == nil || resp.Response == nil {
			return nil, callErr
		}
		return json.Marshal(resp.Response)
	}
)

func minPeriodForMetric(region string, account config.CloudAccount, namespace, metric string, periodFallback int64) int64 {
	key := namespace + "|" + metric
	periodMu.RLock()
	if v, ok := periodCache[key]; ok && v > 0 {
		periodMu.RUnlock()
		return v
	}
	periodMu.RUnlock()
	bs, err := describeBaseMetricsJSON(region, account.AccessKeyID, account.AccessKeySecret, namespace)
	if err != nil {
		status := providerscommon.ClassifyTencentError(err)
		metrics.RequestTotal.WithLabelValues("tencent", "DescribeBaseMetrics", status).Inc()
		return 60
	}
	var jr struct {
		MetricSet []struct {
			MetricName *string `json:"MetricName"`
			Periods    any     `json:"Periods"`
			Period     any     `json:"Period"`
		} `json:"MetricSet"`
	}
	_ = json.Unmarshal(bs, &jr)
	min := int64(0)
	for _, m := range jr.MetricSet {
		if m.MetricName == nil || *m.MetricName != metric {
			continue
		}
		switch v := m.Periods.(type) {
		case []any:
			for _, p := range v {
				switch pv := p.(type) {
				case float64:
					pi := int64(pv)
					if pi > 0 && (min == 0 || pi < min) {
						min = pi
					}
				case int64:
					pi := pv
					if pi > 0 && (min == 0 || pi < min) {
						min = pi
					}
				case string:
					if n, err := strconv.Atoi(strings.TrimSpace(pv)); err == nil {
						pi := int64(n)
						if pi > 0 && (min == 0 || pi < min) {
							min = pi
						}
					}
				}
			}
		}
		if min == 0 {
			switch v := m.Period.(type) {
			case float64:
				pi := int64(v)
				if pi > 0 {
					min = pi
				}
			case int64:
				if v > 0 {
					min = v
				}
			case string:
				if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n > 0 {
					min = int64(n)
				}
			}
		}
		break
	}
	if min == 0 {
		if periodFallback > 0 {
			min = periodFallback
		} else {
			min = 60 // 默认值
		}
	}
	periodMu.Lock()
	periodCache[key] = min
	periodMu.Unlock()
	return min
}
