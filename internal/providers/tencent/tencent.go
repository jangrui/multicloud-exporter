package tencent

import (
	"bufio"
	"encoding/json"
	"hash/fnv"
	"net"
	"os"
	"sort"
	"strconv"
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
	monitor "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/monitor/v20180724"
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
	if len(regions) == 0 || (len(regions) == 1 && regions[0] == "*") {
		regions = t.getAllRegions(account)
		if len(regions) == 0 {
			regions = []string{"ap-guangzhou"}
		}
	}

	var wg sync.WaitGroup
	wTotal, wIndex := clusterConf()
	for _, region := range regions {
		if !assignRegion(account.AccountID, region, wTotal, wIndex) {
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
	credential := common.NewCredential(account.AccessKeyID, account.AccessKeySecret)
	client, err := cvm.NewClient(credential, "ap-guangzhou", profile.NewClientProfile())
	if err != nil {
		return []string{"ap-guangzhou"}
	}
	req := cvm.NewDescribeRegionsRequest()
	start := time.Now()
	resp, err := client.DescribeRegions(req)
	if err != nil || resp == nil || resp.Response == nil || resp.Response.RegionSet == nil {
		status := classifyTencentError(err)
		metrics.RequestTotal.WithLabelValues("tencent", "DescribeRegions", status).Inc()
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
	metrics.RequestTotal.WithLabelValues("tencent", "DescribeRegions", "success").Inc()
	metrics.RequestDuration.WithLabelValues("tencent", "DescribeRegions").Observe(time.Since(start).Seconds())
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
		resp, err := client.DescribeBaseMetrics(req)
		if err != nil || resp == nil || resp.Response == nil {
			return nil, err
		}
		metrics.RequestTotal.WithLabelValues("tencent", "DescribeBaseMetrics", "success").Inc()
		metrics.RequestDuration.WithLabelValues("tencent", "DescribeBaseMetrics").Observe(time.Since(start).Seconds())
		return json.Marshal(resp.Response)
	}
)

func minPeriodForMetric(region string, account config.CloudAccount, namespace, metric string) int64 {
	key := namespace + "|" + metric
	periodMu.RLock()
	if v, ok := periodCache[key]; ok && v > 0 {
		periodMu.RUnlock()
		return v
	}
	periodMu.RUnlock()
	bs, err := describeBaseMetricsJSON(region, account.AccessKeyID, account.AccessKeySecret, namespace)
	if err != nil {
		status := classifyTencentError(err)
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
		min = 60
	}
	periodMu.Lock()
	periodCache[key] = min
	periodMu.Unlock()
	return min
}

func clusterConf() (int, int) {
	if os.Getenv("CLUSTER_DISCOVERY") == "headless" {
		svc := os.Getenv("CLUSTER_SVC")
		selfIP := os.Getenv("POD_IP")
		if svc != "" && selfIP != "" {
			if ips, err := net.LookupIP(svc); err == nil && len(ips) > 0 {
				var list []string
				for _, ip := range ips {
					list = append(list, ip.String())
				}
				sort.Strings(list)
				for i, ip := range list {
					if ip == selfIP {
						return len(list), i
					}
				}
			}
		}
	}
	if os.Getenv("CLUSTER_DISCOVERY") == "file" {
		path := os.Getenv("CLUSTER_FILE")
		self := os.Getenv("POD_NAME")
		if self == "" {
			self = os.Getenv("HOSTNAME")
		}
		if path != "" && self != "" {
			if f, err := os.Open(path); err == nil {
				defer func() { _ = f.Close() }()
				var members []string
				sc := bufio.NewScanner(f)
				for sc.Scan() {
					line := strings.TrimSpace(sc.Text())
					if line != "" {
						members = append(members, line)
					}
				}
				if len(members) > 0 {
					sort.Strings(members)
					for i, m := range members {
						if m == self {
							return len(members), i
						}
					}
				}
			}
		}
	}
	total := 1
	index := 0
	if v := os.Getenv("CLUSTER_WORKERS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			total = n
		}
	}
	if v := os.Getenv("CLUSTER_INDEX"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			index = n
		}
	}
	if index >= total {
		index = index % total
	}
	return total, index
}

func shardOf(s string, n int) int {
	if n <= 1 {
		return 0
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	return int(h.Sum32() % uint32(n))
}

func assignRegion(accountID, region string, total, index int) bool {
	if total <= 1 {
		return true
	}
	key := accountID + "|" + region
	return shardOf(key, total) == index
}
