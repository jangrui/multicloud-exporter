// 阿里云采集器：按配置采集云监控指标
package aliyun

import (
	"encoding/json"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/discovery"
	"multicloud-exporter/internal/metrics"

	"sync"

	"github.com/aliyun/alibaba-cloud-sdk-go/services/cms"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/ecs"
)

// Collector 封装阿里云资源采集逻辑
type Collector struct {
	cfg       *config.Config
	disc      *discovery.Manager
	metaCache map[string]metricMeta
	metaMu    sync.RWMutex
	cacheMu   sync.RWMutex
	resCache  map[string]resCacheEntry
}

// NewCollector 创建阿里云采集器实例
func NewCollector(cfg *config.Config, mgr *discovery.Manager) *Collector {
	return &Collector{cfg: cfg, disc: mgr, metaCache: make(map[string]metricMeta), resCache: make(map[string]resCacheEntry)}
}

// Collect 根据账号配置遍历区域与资源类型并采集
func (a *Collector) Collect(account config.CloudAccount) {
	regions := account.Regions
	if len(regions) == 0 || (len(regions) == 1 && regions[0] == "*") {
		regions = a.getAllRegions(account)
	}

	log.Printf("Aliyun 开始账号采集 account_id=%s 区域数=%d", account.AccountID, len(regions))
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
		wg.Add(1)
		sem <- struct{}{}
		go func(r string) {
			defer wg.Done()
			defer func() { <-sem }()
			log.Printf("Aliyun 开始区域采集 account_id=%s region=%s", account.AccountID, r)
			a.collectCMSMetrics(account, r)
			log.Printf("Aliyun 完成区域采集 account_id=%s region=%s", account.AccountID, r)
		}(region)
	}
	wg.Wait()
}

// getAllRegions 通过 DescribeRegions 自动发现全部区域
func (a *Collector) getAllRegions(account config.CloudAccount) []string {
	client, err := ecs.NewClientWithAccessKey("cn-hangzhou", account.AccessKeyID, account.AccessKeySecret)
	if err != nil {
		log.Printf("Aliyun get regions error account_id=%s: %v", account.AccountID, err)
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
		log.Printf("Aliyun describe regions error account_id=%s status=%s: %v", account.AccountID, status, err)
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
	return regions
}

//

//

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
	log.Printf("Aliyun 加载产品配置 account_id=%s region=%s 数量=%d", account.AccountID, region, len(prods))
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

	client, err := cms.NewClientWithAccessKey(region, ak, sk)
	if err != nil {
		log.Printf("Aliyun CMS client error: %v", err)
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
		if rfilter == "" || !containsResource(account.Resources, rfilter) {
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
					_ = metrics.NamespaceGauge(prod.Namespace, metricName)
					meta := a.getMetricMeta(client, prod.Namespace, metricName)
					localPeriod := period
					if localPeriod == "" && meta.MinPeriod != "" {
						localPeriod = meta.MinPeriod
					}
					if len(meta.Dimensions) == 0 {
						continue
					}
					if prod.Namespace == "acs_slb_dashboard" {
						if !hasAnyDim(meta.Dimensions, []string{"InstanceId", "instanceId", "instance_id"}) {
							continue
						}
					}
					dimKey := chooseDimKeyForNamespace(prod.Namespace, meta.Dimensions)
					if dimKey == "" {
						continue
					}
					rtype := resourceTypeForNamespace(prod.Namespace)
					cachedIDs, hit := a.getCachedIDs(account, region, prod.Namespace, rtype)
					var resIDs []string
					if hit {
						resIDs = cachedIDs
						log.Printf("Aliyun 资源缓存命中 account_id=%s region=%s namespace=%s resource_type=%s 数量=%d", account.AccountID, region, prod.Namespace, rtype, len(resIDs))
					} else {
						resIDs, rtype = a.resourceIDsForNamespace(account, region, prod.Namespace)
						if len(resIDs) == 0 {
							continue
						}
						a.setCachedIDs(account, region, prod.Namespace, rtype, resIDs)
						log.Printf("Aliyun 资源枚举完成 account_id=%s region=%s namespace=%s resource_type=%s 数量=%d", account.AccountID, region, prod.Namespace, rtype, len(resIDs))
					}
					var tagLabels map[string]string
					// code_name 标签来源：
					// - ECS：使用 ListTagResources 过滤 TagKey=="CodeName"，得到实例的业务名称
					// - BWP：使用 ListTagResources 过滤 TagKey=="CodeName"，得到带宽包的业务名称
					// 其他命名空间当前不提供 code_name，保持为空字符串
					switch rtype {
					case "bwp":
						tagLabels = a.fetchCBWPTags(account, region, resIDs)
					case "ecs":
						tagLabels = a.fetchECSTags(account, region, resIDs)
					case "slb":
						tagLabels = a.fetchSLBTags(account, region, resIDs)
					}

					// 并发执行该指标的各批次查询：由 msem 控制并发槽，避免超过云监控 API 限流。
					mwg.Add(1)
					msem <- struct{}{}
					go func(ns, m string, dkey string, rtype string, ids []string, tags map[string]string, p string, stats []string) {
						defer mwg.Done()
						defer func() { <-msem }()
						for start := 0; start < len(ids); start += 50 {
							end := start + 50
							if end > len(ids) {
								end = len(ids)
							}
							dims := make([]map[string]string, 0, end-start)
							for _, id := range ids[start:end] {
								dims = append(dims, map[string]string{dkey: id})
							}
							dimsJSON, _ := json.Marshal(dims)
							req := cms.CreateDescribeMetricLastRequest()
							req.Namespace = ns
							req.MetricName = m
							if p != "" {
								req.Period = p
							}
							if a.cfg.Server != nil && a.cfg.Server.PageSize > 0 {
								req.Length = strconv.Itoa(a.cfg.Server.PageSize)
							} else if a.cfg.ServerConf != nil && a.cfg.ServerConf.PageSize > 0 {
								req.Length = strconv.Itoa(a.cfg.ServerConf.PageSize)
							}
							req.Dimensions = string(dimsJSON)
							nextToken := ""
							log.Printf("Aliyun 拉取指标开始 account_id=%s region=%s namespace=%s metric=%s period=%s 维度数=%d", account.AccountID, region, ns, m, p, len(dims))
							for {
								if nextToken != "" {
									req.NextToken = nextToken
								}
								var resp *cms.DescribeMetricLastResponse
								var callErr error
								// 带指数退避的重试以抵御暂时性错误与限流
								for attempt := 0; attempt < 3; attempt++ {
									startReq := time.Now()
									resp, callErr = client.DescribeMetricLast(req)
									if callErr == nil {
										metrics.RequestTotal.WithLabelValues("aliyun", "DescribeMetricLast", "success").Inc()
										metrics.RequestDuration.WithLabelValues("aliyun", "DescribeMetricLast").Observe(time.Since(startReq).Seconds())
										break
									}
									status := classifyAliyunError(callErr)
									metrics.RequestTotal.WithLabelValues("aliyun", "DescribeMetricLast", status).Inc()
									if status == "auth_error" || status == "region_skip" {
										log.Printf("CMS DescribeMetricLast error status=%s: %v", status, callErr)
										break
									}
									time.Sleep(time.Duration(200*(attempt+1)) * time.Millisecond)
								}
								if callErr != nil {
									break
								}
								var points []map[string]interface{}
								if dp := strings.TrimSpace(resp.Datapoints); dp == "" {
									points = []map[string]interface{}{}
								} else if err := json.Unmarshal([]byte(dp), &points); err != nil {
									// 当返回为空或非标准 JSON 时，回退为无数据，但仍暴露 0 值，便于在 /metrics 中检索指标是否存在
									points = []map[string]interface{}{}
								}
								log.Printf("Aliyun 拉取指标成功 account_id=%s region=%s namespace=%s metric=%s 返回点数=%d", account.AccountID, region, ns, m, len(points))
								if len(points) == 0 {
									// 无数据时仍输出 0 值样本，让指标可见
									for _, rid := range ids[start:end] {
										var codeNameVal string
										if tags != nil {
											codeNameVal = tags[rid]
										}
										metrics.NamespaceGauge(ns, m).WithLabelValues(
											"aliyun", account.AccountID, region, rtype, rid, ns, m, codeNameVal,
										).Set(0)
									}
								}
								for _, pnt := range points {
									idAny, ok := pnt[dkey]
									if !ok {
										continue
									}
									rid, _ := idAny.(string)
									val := pickStatisticValue(pnt, chooseStatistics(stats, group.Statistics))
									var codeNameVal string
									if tags != nil {
										codeNameVal = tags[rid]
									}
									// NamespaceGauge 的最后一个标签为 code_name，用于展示资源的业务标识
									metrics.NamespaceGauge(ns, m).WithLabelValues(
										"aliyun",
										account.AccountID,
										region,
										rtype,
										rid,
										ns,
										m,
										codeNameVal,
									).Set(val)
								}
								if resp.NextToken == "" {
									break
								}
								nextToken = resp.NextToken
								time.Sleep(25 * time.Millisecond)
							}
							log.Printf("Aliyun 拉取指标完成 account_id=%s region=%s namespace=%s metric=%s", account.AccountID, region, ns, m)
						}
					}(prod.Namespace, metricName, dimKey, rtype, resIDs, tagLabels, localPeriod, meta.Statistics)
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

func (a *Collector) getMetricMeta(client *cms.Client, namespace, metric string) metricMeta {
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
	resp, err := client.DescribeMetricMetaList(req)
	if err != nil {
		return metricMeta{}
	}
	metrics.RequestTotal.WithLabelValues("aliyun", "DescribeMetricMetaList", "success").Inc()
	metrics.RequestDuration.WithLabelValues("aliyun", "DescribeMetricMetaList").Observe(time.Since(start).Seconds())
	var out metricMeta
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
		if r.Statistics != "" {
			parts := strings.Split(r.Statistics, ",")
			for i := range parts {
				parts[i] = strings.TrimSpace(parts[i])
			}
			out.Statistics = parts
		}
		if r.Periods != "" {
			out.MinPeriod = strings.TrimSpace(r.Periods)
		}
	}
	a.metaMu.Lock()
	a.metaCache[key] = out
	a.metaMu.Unlock()
	return out
}

//

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
	pick := func(candidates ...string) string {
		for _, want := range candidates {
			lw := strings.ToLower(want)
			for _, d := range dims {
				if strings.ToLower(d) == lw {
					return d
				}
			}
		}
		return ""
	}
	switch namespace {
	case "acs_ecs_dashboard":
		if v := pick("instanceId", "InstanceId", "instance_id"); v != "" {
			return v
		}
	case "acs_bandwidth_package":
		// 阿里云 BWP 通常使用 InstanceId 作为维度键
		if v := pick("instanceId", "InstanceId", "instance_id"); v != "" {
			return v
		}
	case "acs_slb_dashboard":
		if v := pick("instanceId", "InstanceId", "instance_id"); v != "" {
			return v
		}
		return ""
	}
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
	for _, x := range list {
		if x == r || x == "*" {
			return true
		}
	}
	return false
}

//

func resourceTypeForNamespace(namespace string) string {
	switch namespace {
	case "acs_bandwidth_package":
		return "bwp"
	case "acs_ecs_dashboard":
		return "ecs"
	case "acs_slb_dashboard":
		return "lb"
	default:
		return ""
	}
}

func (a *Collector) resourceIDsForNamespace(account config.CloudAccount, region string, namespace string) ([]string, string) {
	switch namespace {
	case "acs_bandwidth_package":
		return a.listCBWPIDs(account, region), "bwp"
	case "acs_ecs_dashboard":
		return a.listECSInstanceIDs(account, region), "ecs"
	case "acs_slb_dashboard":
		return a.listSLBIDs(account, region), "lb"
	default:
		return []string{}, ""
	}
}

type resCacheEntry struct {
	IDs       []string
	UpdatedAt time.Time
}

func (a *Collector) cacheKey(account config.CloudAccount, region, namespace, rtype string) string {
	return account.AccountID + "|" + region + "|" + namespace + "|" + rtype
}

func (a *Collector) getCachedIDs(account config.CloudAccount, region, namespace, rtype string) ([]string, bool) {
	a.cacheMu.RLock()
	entry, ok := a.resCache[a.cacheKey(account, region, namespace, rtype)]
	a.cacheMu.RUnlock()
	if !ok || len(entry.IDs) == 0 {
		return nil, false
	}
	ttlDur := time.Hour
	if a.cfg != nil && a.cfg.ServerConf != nil {
		if a.cfg.ServerConf.DiscoveryTTL != "" {
			if d, err := time.ParseDuration(a.cfg.ServerConf.DiscoveryTTL); err == nil {
				ttlDur = d
			}
		}
	} else if a.cfg != nil && a.cfg.Server != nil {
		if a.cfg.Server.DiscoveryTTL != "" {
			if d, err := time.ParseDuration(a.cfg.Server.DiscoveryTTL); err == nil {
				ttlDur = d
			}
		}
	}
	if time.Since(entry.UpdatedAt) > ttlDur {
		return nil, false
	}
	return entry.IDs, true
}

func (a *Collector) setCachedIDs(account config.CloudAccount, region, namespace, rtype string, ids []string) {
	a.cacheMu.Lock()
	a.resCache[a.cacheKey(account, region, namespace, rtype)] = resCacheEntry{IDs: ids, UpdatedAt: time.Now()}
	a.cacheMu.Unlock()
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
