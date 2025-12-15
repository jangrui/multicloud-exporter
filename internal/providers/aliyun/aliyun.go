// 阿里云采集器：按配置采集云监控指标
package aliyun

import (
	"encoding/json"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/discovery"
	"multicloud-exporter/internal/logger"
	"multicloud-exporter/internal/metrics"
	"multicloud-exporter/internal/utils"

	"sync"

	alb20200616 "github.com/alibabacloud-go/alb-20200616/v2/client"
	nlb20220430 "github.com/alibabacloud-go/nlb-20220430/v4/client"
	"github.com/alibabacloud-go/tea/tea"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/cms"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/ecs"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/sts"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/tag"
	"golang.org/x/sync/singleflight"
)

// Collector 封装阿里云资源采集逻辑
type Collector struct {
	cfg           *config.Config
	disc          *discovery.Manager
	metaCache     map[string]metricMeta
	metaMu        sync.RWMutex
	cacheMu       sync.RWMutex
	resCache      map[string]resCacheEntry
	uidCache      map[string]string
	uidMu         sync.RWMutex
	ossCache      map[string]ossCacheEntry
	ossMu         sync.Mutex
	clientFactory ClientFactory
	sf            singleflight.Group
}

// resCacheEntry 缓存资源ID及元数据
type resCacheEntry struct {
	IDs       []string
	Meta      map[string]interface{}
	UpdatedAt time.Time
}

// ossCacheEntry 缓存账号级 OSS Bucket 列表
type ossCacheEntry struct {
	Buckets   []ossBucketInfo
	UpdatedAt time.Time
}

type ossBucketInfo struct {
	Name     string
	Location string
}

// NewCollector 创建阿里云采集器实例
func NewCollector(cfg *config.Config, mgr *discovery.Manager) *Collector {
	return &Collector{
		cfg:           cfg,
		disc:          mgr,
		metaCache:     make(map[string]metricMeta),
		resCache:      make(map[string]resCacheEntry),
		uidCache:      make(map[string]string),
		ossCache:      make(map[string]ossCacheEntry),
		clientFactory: &defaultClientFactory{},
	}
}

// getAccountUID 获取阿里云账号的数字 ID (UID)
func (a *Collector) getAccountUID(account config.CloudAccount, region string) string {
	// 1. 尝试从缓存获取
	a.uidMu.RLock()
	uid, ok := a.uidCache[account.AccessKeyID]
	a.uidMu.RUnlock()
	if ok {
		return uid
	}

	// 2. 调用 STS 获取 CallerIdentity
	// region 可以是任意有效区域，通常用 cn-hangzhou 或当前区域
	if region == "" {
		region = "cn-hangzhou"
	}
	client, err := a.clientFactory.NewSTSClient(region, account.AccessKeyID, account.AccessKeySecret)
	if err != nil {
		logger.Log.Errorf("Aliyun init STS client error: %v", err)
		return account.AccountID // 回退到配置ID
	}

	req := sts.CreateGetCallerIdentityRequest()
	resp, err := client.GetCallerIdentity(req)
	if err != nil {
		logger.Log.Errorf("Aliyun GetCallerIdentity error: %v", err)
		return account.AccountID // 回退到配置ID
	}

	uid = resp.AccountId
	if uid != "" {
		a.uidMu.Lock()
		a.uidCache[account.AccessKeyID] = uid
		a.uidMu.Unlock()
		return uid
	}

	return account.AccountID
}

// Collect 根据账号配置遍历区域与资源类型并采集
func (a *Collector) Collect(account config.CloudAccount) {
	regions := account.Regions
	if len(regions) == 0 || (len(regions) == 1 && regions[0] == "*") {
		regions = a.getAllRegions(account)
	}

	logger.Log.Debugf("Aliyun 开始账号采集 account_id=%s 区域数=%d", account.AccountID, len(regions))
	limit := 4
	if a.cfg != nil && a.cfg.Server != nil && a.cfg.Server.RegionConcurrency > 0 {
		limit = a.cfg.Server.RegionConcurrency
	} else if a.cfg != nil && a.cfg.ServerConf != nil && a.cfg.ServerConf.RegionConcurrency > 0 {
		limit = a.cfg.ServerConf.RegionConcurrency
	}
	if limit < 1 {
		limit = 1
	}
	sem := make(chan struct{}, limit)
	var wg sync.WaitGroup
	for _, region := range regions {
		wTotal, wIndex := utils.ClusterConfig()
		key := account.AccountID + "|" + region
		if !utils.ShouldProcess(key, wTotal, wIndex) {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(r string) {
			defer wg.Done()
			defer func() { <-sem }()
			logger.Log.Debugf("Aliyun 开始区域采集 account_id=%s region=%s", account.AccountID, r)
			a.collectCMSMetrics(account, r)
			logger.Log.Debugf("Aliyun 完成区域采集 account_id=%s region=%s", account.AccountID, r)
		}(region)
	}
	wg.Wait()
}

// getAllRegions 通过 DescribeRegions 自动发现全部区域
func (a *Collector) getAllRegions(account config.CloudAccount) []string {
	client, err := a.clientFactory.NewECSClient("cn-hangzhou", account.AccessKeyID, account.AccessKeySecret)
	if err != nil {
		logger.Log.Errorf("Aliyun get regions error account_id=%s: %v", account.AccountID, err)
		return []string{"cn-hangzhou"}
	}

	request := ecs.CreateDescribeRegionsRequest()
	start := time.Now()
	response, err := client.DescribeRegions(request)
	if err != nil {
		msg := err.Error()
		status := "error"
		if strings.Contains(msg, "InvalidAccessKeyId") || strings.Contains(msg, "Forbidden") || strings.Contains(msg, "SignatureDoesNotMatch") {
			status = "auth_error"
		} else if strings.Contains(msg, "timeout") || strings.Contains(msg, "unreachable") || strings.Contains(msg, "Temporary network") {
			status = "network_error"
		}
		logger.Log.Errorf("Aliyun describe regions error account_id=%s status=%s: %v", account.AccountID, status, err)
		metrics.RequestTotal.WithLabelValues("aliyun", "DescribeRegions", status).Inc()
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
		return []string{"cn-hangzhou"}
	}
	metrics.RequestTotal.WithLabelValues("aliyun", "DescribeRegions", "success").Inc()
	metrics.RequestDuration.WithLabelValues("aliyun", "DescribeRegions").Observe(time.Since(start).Seconds())

	var regions []string
	for _, region := range response.Regions.Region {
		regions = append(regions, region.RegionId)
	}
	logger.Log.Debugf("Aliyun DescribeRegions success count=%d account_id=%s", len(regions), account.AccountID)
	return regions
}

func (a *Collector) collectCMSMetrics(account config.CloudAccount, region string) {
	if a.cfg == nil {
		return
	}
	var prods []config.Product
	if a.disc != nil {
		if ps, ok := a.disc.Get()["aliyun"]; ok && len(ps) > 0 {
			prods = ps
		}
	}
	if len(prods) == 0 {
		return
	}
	baseLog := logger.NewContextLogger("Aliyun", "account_id", account.AccountID, "region", region)
	baseLog.Debugf("加载产品配置 数量=%d", len(prods))
	ak := account.AccessKeyID
	sk := account.AccessKeySecret
	if a.cfg.Credential != nil {
		if a.cfg.Credential.AccessKey != "" {
			ak = a.cfg.Credential.AccessKey
		}
		if a.cfg.Credential.AccessSecret != "" {
			sk = a.cfg.Credential.AccessSecret
		}
	}

	client, err := a.clientFactory.NewCMSClient(region, ak, sk)
	if err != nil {
		logger.Log.Errorf("Aliyun CMS client error: %v", err)
		return
	}
	// Endpoint 使用地域默认配置，如需自定义可在此处扩展

	// 不预先限定 ECS 维度，按具体指标的维度键枚举对应资源
	// 并发层级说明：
	// 1) 区域级并发：在 Collect 中控制（同账号多 region 并行）
	// 2) 产品级并发：在本函数内控制（同 region 下多个命名空间并行）
	// 3) 指标级并发：在每个产品 goroutine 内控制（同命名空间下多个指标批次并行）
	// 其中 mlimit 控制第 3 层并发，plimit 控制第 2 层并发。

	// 指标并发控制（命名空间/指标级）
	mlimit := 5
	if a.cfg != nil && a.cfg.Server != nil && a.cfg.Server.MetricConcurrency > 0 {
		mlimit = a.cfg.Server.MetricConcurrency
	} else if a.cfg != nil && a.cfg.ServerConf != nil && a.cfg.ServerConf.MetricConcurrency > 0 {
		mlimit = a.cfg.ServerConf.MetricConcurrency
	}
	if mlimit < 1 {
		mlimit = 1
	}
	msem := make(chan struct{}, mlimit)
	var mwg sync.WaitGroup

	// 产品并发控制（命名空间级）：控制同一地域内不同命名空间（如 ECS/BWP）并行度，避免串行导致总时长过长。
	plimit := 2
	if a.cfg != nil && a.cfg.Server != nil && a.cfg.Server.ProductConcurrency > 0 {
		plimit = a.cfg.Server.ProductConcurrency
	} else if a.cfg != nil && a.cfg.ServerConf != nil && a.cfg.ServerConf.ProductConcurrency > 0 {
		plimit = a.cfg.ServerConf.ProductConcurrency
	}
	if plimit < 1 {
		plimit = 1
	}
	psem := make(chan struct{}, plimit)
	var pwg sync.WaitGroup

	for _, prod := range prods {
		if prod.Namespace == "" {
			continue
		}
		rfilter := resourceTypeForNamespace(prod.Namespace)
		// Check for resource permission, including aliases (s3->oss, bwp->cbwp)
		allowed := false
		if rfilter != "" {
			if containsResource(account.Resources, rfilter) {
				allowed = true
			} else if rfilter == "oss" && containsResource(account.Resources, "s3") {
				allowed = true
			} else if rfilter == "cbwp" && containsResource(account.Resources, "bwp") {
				allowed = true
			}
		}
		if !allowed {
			continue
		}
		pwg.Add(1)
		psem <- struct{}{}
		go func(prod config.Product) {
			defer pwg.Done()
			defer func() { <-psem }()
			for _, group := range prod.MetricInfo {
				var period string
				switch {
				case group.Period != nil:
					period = strconv.Itoa(*group.Period)
				case prod.Period != nil:
					period = strconv.Itoa(*prod.Period)
				default:
					period = ""
				}
				// 对每个指标使用指标级并发进行批次拉取。每批最多 50 个维度（实例）。
				for _, metricName := range group.MetricList {
					meta := a.getMetricMeta(client, prod.Namespace, metricName)
					localPeriod := period
					if localPeriod == "" && meta.MinPeriod != "" {
						localPeriod = meta.MinPeriod
					}
					if localPeriod == "" && prod.Namespace == "acs_bandwidth_package" {
						// Fallback: 当元数据不可用时，为共享带宽设置保守的默认周期
						localPeriod = "60"
					}
					if prod.Namespace == "acs_slb_dashboard" && (metricName == "InstanceTrafficRXUtilization" || metricName == "InstanceTrafficTXUtilization") {
						need := []string{"InstanceId", "port", "protocol"}
						if len(meta.Dimensions) == 0 {
							meta.Dimensions = need
						} else if !hasAnyDim(meta.Dimensions, []string{"instanceId", "InstanceId", "instance_id"}) {
							meta.Dimensions = append(meta.Dimensions, need...)
						}
					}
					if len(meta.Dimensions) == 0 {
						baseLog.With("namespace", prod.Namespace, "metric", metricName).Warn("metric skipped (no dimensions)")
						continue
					}
					// 针对不同产品，检查是否包含必要的维度
					// 使用统一的维度检查函数，不再硬编码产品名称
					if !a.checkRequiredDimensions(prod.Namespace, meta.Dimensions) {
						baseLog.With("namespace", prod.Namespace, "metric", metricName).Warnf("metric skipped (dimension mismatch): dims=%v", meta.Dimensions)
						continue
					}
					dimKey := chooseDimKeyForNamespace(prod.Namespace, meta.Dimensions)
					if dimKey == "" {
						continue
					}
					rtype := resourceTypeForNamespace(prod.Namespace)
					cachedIDs, metaInfo, hit := a.getCachedIDs(account, region, prod.Namespace, rtype)
					var resIDs []string
					if hit {
						resIDs = cachedIDs
						baseLog.With("namespace", prod.Namespace, "resource_type", rtype).Debugf("资源缓存命中 数量=%d", len(resIDs))
					} else {
						resIDs, rtype, metaInfo = a.resourceIDsForNamespace(account, region, prod.Namespace)
						a.setCachedIDs(account, region, prod.Namespace, rtype, resIDs, metaInfo)
						baseLog.With("namespace", prod.Namespace, "resource_type", rtype).Debugf("资源枚举完成 数量=%d", len(resIDs))
						if len(resIDs) == 0 {
							continue
						}
					}
					var tagLabels map[string]string
					// code_name 标签来源：
					// - ECS：使用 ListTagResources 过滤 TagKey=="CodeName"，得到实例的业务名称
					// - CBWP：使用 ListTagResources 过滤 TagKey=="CodeName"，得到带宽包的业务名称
					// 其他命名空间当前不提供 code_name，保持为空字符串
					switch rtype {
					case "cbwp", "bwp":
						tagLabels = a.fetchCBWPTags(account, region, resIDs)
					case "lb", "slb", "clb":
						tagLabels = a.fetchSLBTags(account, region, prod.Namespace, metricName, resIDs)
					case "alb":
						tagLabels = a.fetchALBTags(account, region, resIDs)
					case "nlb":
						tagLabels = a.fetchNLBTags(account, region, resIDs)
					case "oss":
						tagLabels = a.fetchOSSBucketTags(account, region, resIDs)
					}

					if len(tagLabels) > 0 {
						logger.NewContextLogger("Aliyun", "account_id", account.AccountID, "region", region, "rtype", rtype).Debugf("FetchTags success count=%d", len(tagLabels))
					} else {
						logger.NewContextLogger("Aliyun", "account_id", account.AccountID, "region", region, "rtype", rtype).Debugf("FetchTags empty or failed")
					}

					// 并发执行该指标的各批次查询：由 msem 控制并发槽，避免超过云监控 API 限流。
					mwg.Add(1)
					msem <- struct{}{}
					go func(ns, m string, dkey string, rtype string, ids []string, tags map[string]string, p string, stats []string, meta map[string]interface{}, metricDims []string) {
						defer mwg.Done()
						defer func() { <-msem }()

						ctxLog := logger.NewContextLogger("Aliyun", "account_id", account.AccountID, "region", region, "namespace", ns, "metric", m)

						allDims, dynamicDims := a.buildMetricDimensions(ids, dkey, metricDims, meta)
						a.fetchAndRecordMetrics(client, account, region, ns, m, dkey, rtype, p, allDims, dynamicDims, tags, stats, ctxLog)
					}(prod.Namespace, metricName, dimKey, rtype, resIDs, tagLabels, localPeriod, meta.Statistics, metaInfo, meta.Dimensions)
				}
			}
		}(prod)
	}
	pwg.Wait()
	mwg.Wait()
}

func pickStatisticValue(p map[string]interface{}, stats []string) float64 {
	order := stats
	if len(order) == 0 {
		order = []string{"Average", "Maximum", "Minimum"}
	}
	for _, k := range order {
		if v, ok := p[k]; ok {
			switch num := v.(type) {
			case float64:
				return num
			case int:
				return float64(num)
			case json.Number:
				f, _ := num.Float64()
				return f
			}
		}
	}
	return 0
}

type metricMeta struct {
	Dimensions []string
	Statistics []string
	MinPeriod  string
}

func (a *Collector) getMetricMeta(client CMSClient, namespace, metric string) metricMeta {
	key := namespace + "|" + metric
	a.metaMu.RLock()
	m, ok := a.metaCache[key]
	a.metaMu.RUnlock()
	if ok {
		return m
	}
	req := cms.CreateDescribeMetricMetaListRequest()
	req.Namespace = namespace
	req.MetricName = metric
	start := time.Now()
	var resp *cms.DescribeMetricMetaListResponse
	var err error
	for attempt := 0; attempt < 5; attempt++ {
		resp, err = client.DescribeMetricMetaList(req)
		if err == nil {
			break
		}
		if classifyAliyunError(err) == "limit_error" {
			sleep := time.Duration(200*(1<<attempt)) * time.Millisecond
			if sleep > 5*time.Second {
				sleep = 5 * time.Second
			}
			time.Sleep(sleep)
			continue
		}
		break
	}
	var out metricMeta
	if err != nil {
		logger.Log.Warnf("Aliyun getMetricMeta error: namespace=%s metric=%s error=%v", namespace, metric, err)
		st := classifyAliyunError(err)
		metrics.RequestTotal.WithLabelValues("aliyun", "DescribeMetricMetaList", st).Inc()
		metrics.RecordRequest("aliyun", "DescribeMetricMetaList", st)
		// Don't return empty meta on error, fall through to use default dimensions
	} else {
		metrics.RequestTotal.WithLabelValues("aliyun", "DescribeMetricMetaList", "success").Inc()
		metrics.RequestDuration.WithLabelValues("aliyun", "DescribeMetricMetaList").Observe(time.Since(start).Seconds())
		metrics.RecordRequest("aliyun", "DescribeMetricMetaList", "success")
		if len(resp.Resources.Resource) > 0 {
			r := resp.Resources.Resource[0]
			dims := strings.Split(r.Dimensions, ",")
			var ds []string
			for _, d := range dims {
				d = strings.TrimSpace(d)
				if d != "" && d != "userId" {
					ds = append(ds, d)
				}
			}
			out.Dimensions = ds
			if len(out.Dimensions) == 0 {
				key := "aliyun." + namespace
				if a.cfg != nil && a.cfg.ServerConf != nil {
					if req, ok := a.cfg.ServerConf.ResourceDimMapping[key]; ok && len(req) > 0 {
						out.Dimensions = append(out.Dimensions, req...)
					}
				}
				if len(out.Dimensions) == 0 {
					defaults := config.DefaultResourceDimMapping()
					if req, ok := defaults[key]; ok && len(req) > 0 {
						out.Dimensions = append(out.Dimensions, req...)
					}
				}
			}
			if r.Statistics != "" {
				parts := strings.Split(r.Statistics, ",")
				for i := range parts {
					parts[i] = strings.TrimSpace(parts[i])
				}
				out.Statistics = parts
			}
			if r.Periods != "" {
				// 解析 Periods (如 "60,300") 取最小值，避免直接透传导致 API 报错
				parts := strings.Split(r.Periods, ",")
				minP := 0
				for _, p := range parts {
					p = strings.TrimSpace(p)
					if val, err := strconv.Atoi(p); err == nil && val > 0 {
						if minP == 0 || val < minP {
							minP = val
						}
					}
				}
				if minP > 0 {
					out.MinPeriod = strconv.Itoa(minP)
				} else {
					// fallback
					out.MinPeriod = strings.TrimSpace(r.Periods)
				}
			}
		}
	}
	if len(out.Dimensions) == 0 {
		key := "aliyun." + namespace
		if a.cfg != nil && a.cfg.ServerConf != nil {
			if req, ok := a.cfg.ServerConf.ResourceDimMapping[key]; ok && len(req) > 0 {
				out.Dimensions = append(out.Dimensions, req...)
			}
		}
		if len(out.Dimensions) == 0 {
			defaults := config.DefaultResourceDimMapping()
			if req, ok := defaults[key]; ok && len(req) > 0 {
				out.Dimensions = append(out.Dimensions, req...)
			}
		}
	}
	a.metaMu.Lock()
	a.metaCache[key] = out
	a.metaMu.Unlock()
	return out
}

//

func (a *Collector) checkRequiredDimensions(namespace string, dims []string) bool {
	// 优先使用配置驱动的映射；若不匹配，继续尝试默认与兜底逻辑
	if a.cfg != nil && a.cfg.ServerConf != nil && len(a.cfg.ServerConf.ResourceDimMapping) > 0 {
		key := "aliyun." + namespace
		if req, ok := a.cfg.ServerConf.ResourceDimMapping[key]; ok && len(req) > 0 {
			if hasAnyDim(dims, req) {
				return true
			}
		}
	}

	// 内置默认映射
	defaults := config.DefaultResourceDimMapping()
	key := "aliyun." + namespace
	if req, ok := defaults[key]; ok {
		if hasAnyDim(dims, req) {
			return true
		}
	}

	// 兜底：存在通用主键 instanceId 时允许尝试采集（SLB/部分 CBWP 场景）
	if hasAnyDim(dims, []string{"instanceId", "InstanceId", "instance_id"}) {
		return true
	}

	return false
}

// chooseStatistics 对所需的统计数据与可用的统计数据进行筛选。
// 如果没有找到所需的统计数据，则返回可用的统计数据。
// 注意：此函数目前未使用，但保留以供将来使用。
//
//nolint:unused
func chooseStatistics(available []string, desired []string) []string {
	if len(desired) == 0 {
		return available
	}
	var res []string
	set := make(map[string]struct{})
	for _, a := range available {
		set[a] = struct{}{}
	}
	for _, d := range desired {
		if _, ok := set[d]; ok {
			res = append(res, d)
		}
	}
	if len(res) == 0 {
		return available
	}
	return res
}

func chooseDimKeyForNamespace(namespace string, dims []string) string {
	if len(dims) == 0 {
		return ""
	}
	pick := func(candidates []string) string {
		for _, want := range candidates {
			lw := strings.ToLower(strings.TrimSpace(want))
			for _, d := range dims {
				if strings.ToLower(strings.TrimSpace(d)) == lw {
					return d
				}
			}
		}
		return ""
	}
	// 使用默认映射表中的候选维度键
	var candidates []string
	defaults := config.DefaultResourceDimMapping()
	if req, ok := defaults["aliyun."+namespace]; ok {
		candidates = append(candidates, req...)
	}
	if v := pick(candidates); v != "" {
		return v
	}
	// 兼容旧逻辑的兜底（防止云侧返回大小写/别名差异）
	switch namespace {
	case "acs_bandwidth_package":
		if v := pick([]string{"BandwidthPackageId", "bandwidthPackageId", "sharebandwidthpackages"}); v != "" {
			return v
		}
	case "acs_slb_dashboard":
		if v := pick([]string{"instanceId", "InstanceId", "instance_id"}); v != "" {
			return v
		}
	case "acs_oss_dashboard":
		if v := pick([]string{"BucketName", "bucketName", "bucket_name"}); v != "" {
			return v
		}
	}
	// 最后回退到第一个维度（保持原始行为）
	return dims[0]
}

func hasAnyDim(dims []string, keys []string) bool {
	if len(dims) == 0 || len(keys) == 0 {
		return false
	}
	lower := make(map[string]struct{}, len(dims))
	for _, d := range dims {
		lower[strings.ToLower(strings.TrimSpace(d))] = struct{}{}
	}
	for _, k := range keys {
		if _, ok := lower[strings.ToLower(k)]; ok {
			return true
		}
	}
	return false
}

func containsResource(list []string, r string) bool {
	want := strings.ToLower(r)
	for _, x := range list {
		xx := strings.ToLower(x)
		if xx == "*" || xx == want {
			return true
		}
	}
	return false
}

//

func resourceTypeForNamespace(namespace string) string {
	switch namespace {
	case "acs_bandwidth_package":
		return "cbwp"
	case "acs_slb_dashboard":
		return "clb"
	case "acs_oss_dashboard":
		return "oss"
	case "acs_alb":
		return "alb"
	case "acs_nlb":
		return "nlb"
	case "acs_gwlb":
		return "gwlb"
	default:
		return ""
	}
}

func (a *Collector) resourceIDsForNamespace(account config.CloudAccount, region string, namespace string) ([]string, string, map[string]interface{}) {
	switch namespace {
	case "acs_bandwidth_package":
		return a.listCBWPIDs(account, region), "cbwp", nil
	case "acs_slb_dashboard":
		ids, meta := a.listSLBIDs(account, region)
		return ids, "clb", meta
	case "acs_oss_dashboard":
		return a.listOSSIDs(account, region), "oss", nil
	case "acs_alb":
		return a.listALBIDs(account, region), "alb", nil
	case "acs_nlb":
		return a.listNLBIDs(account, region), "nlb", nil
	case "acs_gwlb":
		return a.listAliGWLBIDs(account, region), "gwlb", nil
	default:
		return []string{}, "", nil
	}
}

func (a *Collector) cacheKey(account config.CloudAccount, region, namespace, rtype string) string {
	return account.AccountID + "|" + region + "|" + namespace + "|" + rtype
}

func (a *Collector) listALBIDs(account config.CloudAccount, region string) []string {
	if ids, _, hit := a.getCachedIDs(account, region, "acs_alb", "alb"); hit {
		return ids
	}
	var out []string
	var meta map[string]interface{}
	albClient, err := a.clientFactory.NewALBClient(region, account.AccessKeyID, account.AccessKeySecret)
	if err == nil && albClient != nil {
		pageSize := 100
		if a.cfg != nil {
			if a.cfg.Server != nil && a.cfg.Server.PageSize > 0 && a.cfg.Server.PageSize < pageSize {
				pageSize = a.cfg.Server.PageSize
			} else if a.cfg.ServerConf != nil && a.cfg.ServerConf.PageSize > 0 && a.cfg.ServerConf.PageSize < pageSize {
				pageSize = a.cfg.ServerConf.PageSize
			}
		}
		nextToken := ""
		for {
			req := &alb20200616.ListLoadBalancersRequest{
				MaxResults: tea.Int32(int32(pageSize)),
			}
			if nextToken != "" {
				req.NextToken = tea.String(nextToken)
			}
			resp, callErr := albClient.ListLoadBalancers(req)
			if callErr != nil || resp == nil || resp.Body == nil {
				out = []string{}
				break
			}
			if resp.Body.LoadBalancers != nil {
				for _, lb := range resp.Body.LoadBalancers {
					if lb != nil && lb.LoadBalancerId != nil {
						id := tea.StringValue(lb.LoadBalancerId)
						if id != "" {
							out = append(out, id)
						}
					}
				}
			}
			if resp.Body.NextToken == nil || tea.StringValue(resp.Body.NextToken) == "" {
				break
			}
			nextToken = tea.StringValue(resp.Body.NextToken)
			time.Sleep(25 * time.Millisecond)
		}
	}
	if len(out) == 0 {
		cmsClient, cmsErr := a.clientFactory.NewCMSClient(region, account.AccessKeyID, account.AccessKeySecret)
		if cmsErr != nil {
			return []string{}
		}
		out = a.listIDsByCMS(cmsClient, region, "acs_alb", "LoadBalancerActiveConnection", "loadBalancerId")
		if len(out) > 0 {
			meta = a.buildALBMetaByCMS(cmsClient, region, out)
		}
	} else {
		cmsClient, cmsErr := a.clientFactory.NewCMSClient(region, account.AccessKeyID, account.AccessKeySecret)
		if cmsErr == nil {
			meta = a.buildALBMetaByCMS(cmsClient, region, out)
		}
	}
	a.setCachedIDs(account, region, "acs_alb", "alb", out, meta)
	return out
}

func (a *Collector) listNLBIDs(account config.CloudAccount, region string) []string {
	if ids, _, hit := a.getCachedIDs(account, region, "acs_nlb", "nlb"); hit {
		return ids
	}
	var out []string
	var meta map[string]interface{}
	nlbClient, err := a.clientFactory.NewNLBClient(region, account.AccessKeyID, account.AccessKeySecret)
	if err == nil && nlbClient != nil {
		pageSize := 100
		if a.cfg != nil {
			if a.cfg.Server != nil && a.cfg.Server.PageSize > 0 && a.cfg.Server.PageSize < pageSize {
				pageSize = a.cfg.Server.PageSize
			} else if a.cfg.ServerConf != nil && a.cfg.ServerConf.PageSize > 0 && a.cfg.ServerConf.PageSize < pageSize {
				pageSize = a.cfg.ServerConf.PageSize
			}
		}
		nextToken := ""
		for {
			req := &nlb20220430.ListLoadBalancersRequest{
				MaxResults: tea.Int32(int32(pageSize)),
			}
			if nextToken != "" {
				req.NextToken = tea.String(nextToken)
			}
			resp, callErr := nlbClient.ListLoadBalancers(req)
			if callErr != nil || resp == nil || resp.Body == nil {
				out = []string{}
				break
			}
			if resp.Body.LoadBalancers != nil {
				for _, lb := range resp.Body.LoadBalancers {
					if lb != nil && lb.LoadBalancerId != nil {
						id := tea.StringValue(lb.LoadBalancerId)
						if id != "" {
							out = append(out, id)
						}
					}
				}
			}
			if resp.Body.NextToken == nil || tea.StringValue(resp.Body.NextToken) == "" {
				break
			}
			nextToken = tea.StringValue(resp.Body.NextToken)
			time.Sleep(25 * time.Millisecond)
		}
	}
	if len(out) == 0 {
		cmsClient, cmsErr := a.clientFactory.NewCMSClient(region, account.AccessKeyID, account.AccessKeySecret)
		if cmsErr != nil {
			return []string{}
		}
		out = a.listIDsByCMS(cmsClient, region, "acs_nlb", "InstanceActiveConnection", "instanceId")
		if len(out) > 0 {
			meta = a.buildNLBMetaByCMS(cmsClient, region, out)
		}
	} else {
		cmsClient, cmsErr := a.clientFactory.NewCMSClient(region, account.AccessKeyID, account.AccessKeySecret)
		if cmsErr == nil {
			meta = a.buildNLBMetaByCMS(cmsClient, region, out)
		}
	}
	a.setCachedIDs(account, region, "acs_nlb", "nlb", out, meta)
	return out
}

// listAliGWLBIDs 通过 CMS 指标数据枚举 GWLB 资源 ID
func (a *Collector) listAliGWLBIDs(account config.CloudAccount, region string) []string {
	if ids, _, hit := a.getCachedIDs(account, region, "acs_gwlb", "gwlb"); hit {
		return ids
	}
	client, err := a.clientFactory.NewCMSClient(region, account.AccessKeyID, account.AccessKeySecret)
	if err != nil {
		return []string{}
	}
	metric := "ActiveConnection"
	out := a.listIDsByCMS(client, region, "acs_gwlb", metric, "instanceId")
	a.setCachedIDs(account, region, "acs_gwlb", "gwlb", out, nil)
	return out
}

// listIDsByCMS 使用 DescribeMetricList 拉取短时间窗口的数据，解析维度提取资源ID
func (a *Collector) listIDsByCMS(client CMSClient, region, namespace, metric, idKey string) []string {
	req := cms.CreateDescribeMetricListRequest()
	req.Namespace = namespace
	req.MetricName = metric
	// 使用最近 5 分钟窗口以减少数据量
	end := time.Now().UTC()
	start := end.Add(-5 * time.Minute)
	req.StartTime = start.Format("2006-01-02 15:04:05")
	req.EndTime = end.Format("2006-01-02 15:04:05")
	req.Period = "60"
	var resp *cms.DescribeMetricListResponse
	var callErr error
	for attempt := 0; attempt < 3; attempt++ {
		st := time.Now()
		resp, callErr = client.DescribeMetricList(req)
		if callErr == nil {
			metrics.RequestTotal.WithLabelValues("aliyun", "DescribeMetricList", "success").Inc()
			metrics.RequestDuration.WithLabelValues("aliyun", "DescribeMetricList").Observe(time.Since(st).Seconds())
			metrics.RecordRequest("aliyun", "DescribeMetricList", "success")
			break
		}
		status := classifyAliyunError(callErr)
		metrics.RequestTotal.WithLabelValues("aliyun", "DescribeMetricList", status).Inc()
		metrics.RecordRequest("aliyun", "DescribeMetricList", status)
		if status == "auth_error" || status == "region_skip" {
			break
		}
		// 简单退避
		time.Sleep(time.Duration(200*(1<<attempt)) * time.Millisecond)
	}
	if callErr != nil || resp == nil {
		return []string{}
	}
	var out []string
	seen := make(map[string]struct{})
	points := strings.TrimSpace(resp.Datapoints)
	if points != "" {
		var arr []map[string]interface{}
		if err := json.Unmarshal([]byte(points), &arr); err == nil {
			for _, p := range arr {
				if v, ok := p[idKey]; ok {
					if id, ok2 := v.(string); ok2 && id != "" {
						if _, exists := seen[id]; !exists {
							seen[id] = struct{}{}
							out = append(out, id)
						}
					}
				}
			}
		}
	}
	return out
}
func (a *Collector) getCachedIDs(account config.CloudAccount, region, namespace, rtype string) ([]string, map[string]interface{}, bool) {
	a.cacheMu.RLock()
	entry, ok := a.resCache[a.cacheKey(account, region, namespace, rtype)]
	a.cacheMu.RUnlock()
	if !ok {
		return nil, nil, false
	}
	ttlDur := time.Hour
	if a.cfg != nil && a.cfg.ServerConf != nil {
		if a.cfg.ServerConf.DiscoveryTTL != "" {
			if d, err := utils.ParseDuration(a.cfg.ServerConf.DiscoveryTTL); err == nil {
				ttlDur = d
			}
		}
	} else if a.cfg != nil && a.cfg.Server != nil {
		if a.cfg.Server.DiscoveryTTL != "" {
			if d, err := utils.ParseDuration(a.cfg.Server.DiscoveryTTL); err == nil {
				ttlDur = d
			}
		}
	}
	if time.Since(entry.UpdatedAt) > ttlDur {
		return nil, nil, false
	}
	return entry.IDs, entry.Meta, true
}

func (a *Collector) setCachedIDs(account config.CloudAccount, region, namespace, rtype string, ids []string, meta map[string]interface{}) {
	a.cacheMu.Lock()
	a.resCache[a.cacheKey(account, region, namespace, rtype)] = resCacheEntry{IDs: ids, Meta: meta, UpdatedAt: time.Now()}
	a.cacheMu.Unlock()
}
func (a *Collector) buildALBMetaByCMS(client CMSClient, region string, ids []string) map[string]interface{} {
	idSet := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		idSet[id] = struct{}{}
	}
	out := make(map[string]interface{})
	type q struct {
		ns   string
		name string
		id   string
		dims []string
	}
	qs := []q{
		{ns: "acs_alb", name: "ListenerActiveConnection", id: "loadBalancerId", dims: []string{"listenerProtocol", "listenerPort"}},
		{ns: "acs_alb", name: "ServerGroupHTTPCodeUpstream2XX", id: "loadBalancerId", dims: []string{"listenerProtocol", "listenerPort", "serverGroupId"}},
		{ns: "acs_alb", name: "VipActiveConnection", id: "loadBalancerId", dims: []string{"vip"}},
	}
	for _, qq := range qs {
		req := cms.CreateDescribeMetricListRequest()
		req.Namespace = qq.ns
		req.MetricName = qq.name
		end := time.Now().UTC()
		start := end.Add(-5 * time.Minute)
		req.StartTime = start.Format("2006-01-02 15:04:05")
		req.EndTime = end.Format("2006-01-02 15:04:05")
		req.Period = "60"
		resp, err := client.DescribeMetricList(req)
		if err != nil || resp == nil {
			continue
		}
		points := strings.TrimSpace(resp.Datapoints)
		if points == "" {
			continue
		}
		var arr []map[string]interface{}
		if json.Unmarshal([]byte(points), &arr) != nil {
			continue
		}
		type seenKey struct {
			id  string
			key string
		}
		seen := make(map[seenKey]struct{})
		for _, p := range arr {
			v, ok := p[qq.id]
			if !ok {
				continue
			}
			lbID, ok2 := v.(string)
			if !ok2 || lbID == "" {
				continue
			}
			if _, ok3 := idSet[lbID]; !ok3 {
				continue
			}
			item := make(map[string]string)
			for _, dk := range qq.dims {
				if vv, ok := p[dk]; ok {
					if s, ok := vv.(string); ok && s != "" {
						item[dk] = s
					} else if n, ok := vv.(float64); ok {
						item[dk] = strconv.Itoa(int(n))
					}
				}
			}
			if len(item) == 0 {
				continue
			}
			var keyParts []string
			for _, dk := range qq.dims {
				if val, ok := item[dk]; ok {
					keyParts = append(keyParts, dk+"="+val)
				}
			}
			if len(keyParts) == 0 {
				continue
			}
			sk := seenKey{id: lbID, key: strings.Join(keyParts, "|")}
			if _, ok := seen[sk]; ok {
				continue
			}
			seen[sk] = struct{}{}
			if _, ok := out[lbID]; !ok {
				out[lbID] = []map[string]string{}
			}
			list := out[lbID].([]map[string]string)
			list = append(list, item)
			out[lbID] = list
		}
	}
	return out
}
func (a *Collector) buildNLBMetaByCMS(client CMSClient, region string, ids []string) map[string]interface{} {
	idSet := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		idSet[id] = struct{}{}
	}
	out := make(map[string]interface{})
	type q struct {
		ns   string
		name string
		id   string
		dims []string
	}
	qs := []q{
		{ns: "acs_nlb", name: "ListenerPacketRX", id: "instanceId", dims: []string{"listenerProtocol", "listenerPort"}},
		{ns: "acs_nlb", name: "ListenerHeathyServerCount", id: "instanceId", dims: []string{"listenerProtocol", "listenerPort"}},
		{ns: "acs_nlb", name: "VipActiveConnection", id: "instanceId", dims: []string{"vip"}},
	}
	for _, qq := range qs {
		req := cms.CreateDescribeMetricListRequest()
		req.Namespace = qq.ns
		req.MetricName = qq.name
		end := time.Now().UTC()
		start := end.Add(-5 * time.Minute)
		req.StartTime = start.Format("2006-01-02 15:04:05")
		req.EndTime = end.Format("2006-01-02 15:04:05")
		req.Period = "60"
		resp, err := client.DescribeMetricList(req)
		if err != nil || resp == nil {
			continue
		}
		points := strings.TrimSpace(resp.Datapoints)
		if points == "" {
			continue
		}
		var arr []map[string]interface{}
		if json.Unmarshal([]byte(points), &arr) != nil {
			continue
		}
		type seenKey struct {
			id  string
			key string
		}
		seen := make(map[seenKey]struct{})
		for _, p := range arr {
			v, ok := p[qq.id]
			if !ok {
				continue
			}
			instID, ok2 := v.(string)
			if !ok2 || instID == "" {
				continue
			}
			if _, ok3 := idSet[instID]; !ok3 {
				continue
			}
			item := make(map[string]string)
			for _, dk := range qq.dims {
				if vv, ok := p[dk]; ok {
					if s, ok := vv.(string); ok && s != "" {
						item[dk] = s
					} else if n, ok := vv.(float64); ok {
						item[dk] = strconv.Itoa(int(n))
					}
				}
			}
			if len(item) == 0 {
				continue
			}
			var keyParts []string
			for _, dk := range qq.dims {
				if val, ok := item[dk]; ok {
					keyParts = append(keyParts, dk+"="+val)
				}
			}
			if len(keyParts) == 0 {
				continue
			}
			sk := seenKey{id: instID, key: strings.Join(keyParts, "|")}
			if _, ok := seen[sk]; ok {
				continue
			}
			seen[sk] = struct{}{}
			if _, ok := out[instID]; !ok {
				out[instID] = []map[string]string{}
			}
			list := out[instID].([]map[string]string)
			list = append(list, item)
			out[instID] = list
		}
	}
	return out
}

func (a *Collector) fetchALBTags(account config.CloudAccount, region string, ids []string) map[string]string {
	if len(ids) == 0 {
		return map[string]string{}
	}
	ctxLog := logger.NewContextLogger("Aliyun", "account_id", account.AccountID, "region", region, "rtype", "alb")
	tagClient, tagErr := a.clientFactory.NewTagClient(region, account.AccessKeyID, account.AccessKeySecret)
	if tagErr != nil {
		return map[string]string{}
	}
	out := make(map[string]string, len(ids))
	batchSize := 50
	total := len(ids)
	uid := a.getAccountUID(account, region)
	for start := 0; start < total; start += batchSize {
		end := start + batchSize
		if end > total {
			end = total
		}
		var arns []string
		for _, id := range ids[start:end] {
			arns = append(arns, "arn:acs:alb:"+region+":"+uid+":loadbalancer/"+id)
		}
		req := tag.CreateListTagResourcesRequest()
		req.RegionId = region
		req.ResourceARN = &arns
		resp, callErr := tagClient.ListTagResources(req)
		if callErr != nil || resp == nil {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		if len(resp.TagResources) > 0 {
			for _, tr := range resp.TagResources {
				id := tr.ResourceId
				if id == "" && tr.ResourceARN != "" {
					parts := strings.Split(tr.ResourceARN, "/")
					if len(parts) > 0 {
						id = parts[len(parts)-1]
					}
				}
				if id == "" {
					continue
				}
				for _, t := range tr.Tags {
					k := t.Key
					v := t.Value
					if strings.EqualFold(k, "CodeName") || strings.EqualFold(k, "code_name") {
						out[id] = v
					}
				}
			}
		} else {
			content := resp.GetHttpContentBytes()
			parsed := parseContentCodeName(content)
			for k, v := range parsed {
				out[k] = v
			}
		}
	}
	assigned := 0
	for _, v := range out {
		if v != "" {
			assigned++
		}
	}
	ctxLog.Debugf("ALB 标签采集完成 资源数=%d 有code_name=%d", len(ids), assigned)
	return out
}

func (a *Collector) fetchNLBTags(account config.CloudAccount, region string, ids []string) map[string]string {
	if len(ids) == 0 {
		return map[string]string{}
	}
	ctxLog := logger.NewContextLogger("Aliyun", "account_id", account.AccountID, "region", region, "rtype", "nlb")
	tagClient, tagErr := a.clientFactory.NewTagClient(region, account.AccessKeyID, account.AccessKeySecret)
	if tagErr != nil {
		return map[string]string{}
	}
	out := make(map[string]string, len(ids))
	batchSize := 50
	total := len(ids)
	uid := a.getAccountUID(account, region)
	for start := 0; start < total; start += batchSize {
		end := start + batchSize
		if end > total {
			end = total
		}
		var arns []string
		for _, id := range ids[start:end] {
			arns = append(arns, "arn:acs:nlb:"+region+":"+uid+":loadbalancer/"+id)
		}
		req := tag.CreateListTagResourcesRequest()
		req.RegionId = region
		req.ResourceARN = &arns
		resp, callErr := tagClient.ListTagResources(req)
		if callErr != nil || resp == nil {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		if len(resp.TagResources) > 0 {
			for _, tr := range resp.TagResources {
				id := tr.ResourceId
				if id == "" && tr.ResourceARN != "" {
					parts := strings.Split(tr.ResourceARN, "/")
					if len(parts) > 0 {
						id = parts[len(parts)-1]
					}
				}
				if id == "" {
					continue
				}
				for _, t := range tr.Tags {
					k := t.Key
					v := t.Value
					if strings.EqualFold(k, "CodeName") || strings.EqualFold(k, "code_name") {
						out[id] = v
					}
				}
			}
		} else {
			content := resp.GetHttpContentBytes()
			parsed := parseContentCodeName(content)
			for k, v := range parsed {
				out[k] = v
			}
		}
	}
	assigned := 0
	for _, v := range out {
		if v != "" {
			assigned++
		}
	}
	ctxLog.Debugf("NLB 标签采集完成 资源数=%d 有code_name=%d", len(ids), assigned)
	return out
}

func parseContentCodeName(content []byte) map[string]string {
	out := make(map[string]string)
	var jrA struct {
		TagResources []struct {
			ResourceId  string `json:"ResourceId"`
			ResourceARN string `json:"ResourceARN"`
			Tags        []struct {
				Key      string `json:"Key"`
				Value    string `json:"Value"`
				TagKey   string `json:"TagKey"`
				TagValue string `json:"TagValue"`
			} `json:"Tags"`
		} `json:"TagResources"`
	}
	if err := json.Unmarshal(content, &jrA); err == nil && len(jrA.TagResources) > 0 {
		for _, tr := range jrA.TagResources {
			id := tr.ResourceId
			if id == "" && tr.ResourceARN != "" {
				parts := strings.Split(tr.ResourceARN, "/")
				if len(parts) > 0 {
					id = parts[len(parts)-1]
				}
			}
			if id == "" {
				continue
			}
			for _, t := range tr.Tags {
				k := t.Key
				if k == "" {
					k = t.TagKey
				}
				v := t.Value
				if v == "" {
					v = t.TagValue
				}
				if strings.EqualFold(k, "CodeName") || strings.EqualFold(k, "code_name") {
					out[id] = v
				}
			}
		}
	}
	var jrB struct {
		TagResources struct {
			TagResource []struct {
				ResourceId string `json:"ResourceId"`
				TagKey     string `json:"TagKey"`
				TagValue   string `json:"TagValue"`
				Key        string `json:"Key"`
				Value      string `json:"Value"`
			} `json:"TagResource"`
		} `json:"TagResources"`
	}
	if err := json.Unmarshal(content, &jrB); err == nil && len(jrB.TagResources.TagResource) > 0 {
		for _, tr := range jrB.TagResources.TagResource {
			if tr.ResourceId == "" {
				continue
			}
			k := tr.TagKey
			if k == "" {
				k = tr.Key
			}
			v := tr.TagValue
			if v == "" {
				v = tr.Value
			}
			if strings.EqualFold(k, "CodeName") || strings.EqualFold(k, "code_name") {
				out[tr.ResourceId] = v
			}
		}
	}
	return out
}
func (a *Collector) buildMetricDimensions(ids []string, dkey string, metricDims []string, meta map[string]interface{}) ([]map[string]string, []string) {
	var dynamicDims []string
	reserved := map[string]struct{}{
		"region":         {},
		"userid":         {},
		"cloud_provider": {},
		"account_id":     {},
		"resource_type":  {},
		"resource_id":    {},
		"namespace":      {},
		"metric_name":    {},
		"code_name":      {},
	}
	for _, d := range metricDims {
		lower := strings.ToLower(d)
		if strings.EqualFold(d, dkey) {
			continue
		}
		if _, ok := reserved[lower]; ok {
			continue
		}
		dynamicDims = append(dynamicDims, d)
	}
	sort.Strings(dynamicDims)
	hasDynamicDims := len(dynamicDims) > 0

	var allDims []map[string]string

	for _, id := range ids {
		added := false
		if hasDynamicDims && meta != nil {
			if subResources, ok := meta[id]; ok {
				if list, ok := subResources.([]map[string]string); ok && len(list) > 0 {
					for _, item := range list {
						d := make(map[string]string)
						d[dkey] = id
						matchCount := 0
						for _, dimKey := range dynamicDims {
							for k, v := range item {
								if strings.EqualFold(k, dimKey) {
									d[dimKey] = v
									matchCount++
									break
								}
							}
						}
						if matchCount > 0 {
							allDims = append(allDims, d)
							added = true
						}
					}
				}
			}
		}
		if !added {
			allDims = append(allDims, map[string]string{dkey: id})
		}
	}
	return allDims, dynamicDims
}

func (a *Collector) fetchAndRecordMetrics(
	client CMSClient,
	account config.CloudAccount,
	region, ns, m, dkey, rtype, period string,
	allDims []map[string]string,
	dynamicDims []string,
	tags map[string]string,
	stats []string,
	ctxLog *logger.ContextLogger,
) {
	for start := 0; start < len(allDims); start += 50 {
		end := start + 50
		if end > len(allDims) {
			end = len(allDims)
		}

		if (end)%100 == 0 || end == len(allDims) {
			ctxLog.Debugf("指标采集进度 progress=%d/%d (%.1f%%) tag_count=%d", end, len(allDims), float64(end)/float64(len(allDims))*100, len(tags))
		}

		dims := allDims[start:end]
		dimsJSON, _ := json.Marshal(dims)
		req := cms.CreateDescribeMetricLastRequest()
		req.Namespace = ns
		req.MetricName = m
		if period != "" {
			req.Period = period
		}
		if a.cfg.Server != nil && a.cfg.Server.PageSize > 0 {
			req.Length = strconv.Itoa(a.cfg.Server.PageSize)
		} else if a.cfg.ServerConf != nil && a.cfg.ServerConf.PageSize > 0 {
			req.Length = strconv.Itoa(a.cfg.ServerConf.PageSize)
		}
		req.Dimensions = string(dimsJSON)

		a.processMetricBatch(client, req, dims, account, region, ns, m, dkey, rtype, dynamicDims, tags, stats, ctxLog)
	}
	ctxLog.Debugf("拉取指标完成")
}

func (a *Collector) processMetricBatch(client CMSClient, req *cms.DescribeMetricLastRequest, dims []map[string]string, account config.CloudAccount, region, ns, m, dkey, rtype string, dynamicDims []string, tags map[string]string, stats []string, ctxLog *logger.ContextLogger) {
	nextToken := ""
	for {
		if nextToken != "" {
			req.NextToken = nextToken
		}
		var resp *cms.DescribeMetricLastResponse
		var callErr error
		for attempt := 0; attempt < 5; attempt++ {
			startReq := time.Now()
			resp, callErr = client.DescribeMetricLast(req)
			if callErr == nil {
				metrics.RequestTotal.WithLabelValues("aliyun", "DescribeMetricLast", "success").Inc()
				metrics.RequestDuration.WithLabelValues("aliyun", "DescribeMetricLast").Observe(time.Since(startReq).Seconds())
				metrics.RecordRequest("aliyun", "DescribeMetricLast", "success")
				break
			}
			status := classifyAliyunError(callErr)
			metrics.RequestTotal.WithLabelValues("aliyun", "DescribeMetricLast", status).Inc()
			metrics.RecordRequest("aliyun", "DescribeMetricLast", status)
			if status == "auth_error" || status == "region_skip" {
				ctxLog.Warnf("CMS DescribeMetricLast error status=%s: %v", status, callErr)
				break
			}

			// 指数退避重试
			sleep := time.Duration(200*(1<<attempt)) * time.Millisecond
			if sleep > 5*time.Second {
				sleep = 5 * time.Second
			}
			time.Sleep(sleep)
		}
		if callErr != nil {
			ctxLog.Errorf("拉取指标失败 error=%v", callErr)
			if len(dims) > 0 {
				for _, dim := range dims {
					rid := dim[dkey]
					var codeNameVal string
					if tags != nil {
						codeNameVal = tags[rid]
					}
					var dynamicLabelValues []string
					for _, dimKey := range dynamicDims {
						valStr := ""
						if v, ok := dim[dimKey]; ok {
							valStr = v
						}
						dynamicLabelValues = append(dynamicLabelValues, valStr)
					}
					labels := []string{"aliyun", account.AccountID, region, rtype, rid, ns, m, codeNameVal}
					labels = append(labels, dynamicLabelValues...)
					vec, count := metrics.NamespaceGauge(ns, m, dynamicDims...)
					if len(labels) > count {
						labels = labels[:count]
					} else {
						for len(labels) < count {
							labels = append(labels, "")
						}
					}
					vec.WithLabelValues(labels...).Set(0)
					metrics.IncSampleCount(ns, 1)
				}
			}
			break
		}

		var points []map[string]interface{}
		if dp := strings.TrimSpace(resp.Datapoints); dp == "" {
			points = []map[string]interface{}{}
		} else if err := json.Unmarshal([]byte(dp), &points); err != nil {
			points = []map[string]interface{}{}
			ctxLog.Errorf("指标数据解析失败 content=%s error=%v", dp, err)
		}

		if len(points) == 0 {
			ctxLog.Debugf("Metric %s has 0 points, filling 0 for %d dims", m, len(dims))
			for _, dim := range dims {
				rid := dim[dkey]
				var codeNameVal string
				if tags != nil {
					codeNameVal = tags[rid]
				}
				var dynamicLabelValues []string
				for _, dimKey := range dynamicDims {
					valStr := ""
					if v, ok := dim[dimKey]; ok {
						valStr = v
					}
					dynamicLabelValues = append(dynamicLabelValues, valStr)
				}
				labels := []string{"aliyun", account.AccountID, region, rtype, rid, ns, m, codeNameVal}
				labels = append(labels, dynamicLabelValues...)
				vec, count := metrics.NamespaceGauge(ns, m, dynamicDims...)
				if len(labels) > count {
					labels = labels[:count]
				} else {
					for len(labels) < count {
						labels = append(labels, "")
					}
				}
				vec.WithLabelValues(labels...).Set(0)
			}
		}

		for _, pnt := range points {
			idAny, ok := pnt[dkey]
			if !ok {
				continue
			}
			rid, _ := idAny.(string)
			val := pickStatisticValue(pnt, stats)
			var codeNameVal string
			if tags != nil {
				codeNameVal = tags[rid]
			}

			var dynamicLabelValues []string
			for _, dimKey := range dynamicDims {
				valStr := ""
				if v, ok := pnt[dimKey]; ok {
					switch s := v.(type) {
					case string:
						valStr = s
					case float64:
						valStr = strconv.FormatFloat(s, 'f', -1, 64)
					case int:
						valStr = strconv.Itoa(s)
					}
				}
				dynamicLabelValues = append(dynamicLabelValues, valStr)
			}

			if scale := metrics.GetMetricScale(ns, m); scale != 0 && scale != 1 {
				val *= scale
			}

			labels := []string{"aliyun", account.AccountID, region, rtype, rid, ns, m, codeNameVal}
			labels = append(labels, dynamicLabelValues...)
			vec, count := metrics.NamespaceGauge(ns, m, dynamicDims...)
			if len(labels) > count {
				labels = labels[:count]
			} else {
				for len(labels) < count {
					labels = append(labels, "")
				}
			}
			vec.WithLabelValues(labels...).Set(val)
			metrics.IncSampleCount(ns, 1)
		}
		if resp.NextToken == "" {
			break
		}
		nextToken = resp.NextToken
		time.Sleep(25 * time.Millisecond)
	}
}

func classifyAliyunError(err error) string {
	msg := err.Error()
	if strings.Contains(msg, "InvalidAccessKeyId") || strings.Contains(msg, "Forbidden") || strings.Contains(msg, "SignatureDoesNotMatch") {
		return "auth_error"
	}
	if strings.Contains(msg, "Throttling") || strings.Contains(msg, "flow control") {
		return "limit_error"
	}
	if strings.Contains(msg, "InvalidRegionId") || strings.Contains(msg, "Unsupported") {
		return "region_skip"
	}
	if strings.Contains(msg, "timeout") || strings.Contains(msg, "unreachable") || strings.Contains(msg, "Temporary network") {
		return "network_error"
	}
	return "error"
}
