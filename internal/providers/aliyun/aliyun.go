// 阿里云采集器：按配置采集云监控指标
package aliyun

import (
	"encoding/json"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/metrics"

	"sync"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/requests"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/cms"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/ecs"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/vpc"
)

// Collector 封装阿里云资源采集逻辑
type Collector struct {
	cfg       *config.Config
	metaCache map[string]metricMeta
	metaMu    sync.RWMutex
	cacheMu   sync.RWMutex
	resCache  map[string]resCacheEntry
}

// NewCollector 创建阿里云采集器实例
func NewCollector(cfg *config.Config) *Collector {
	return &Collector{cfg: cfg, metaCache: make(map[string]metricMeta), resCache: make(map[string]resCacheEntry)}
}

// Collect 根据账号配置遍历区域与资源类型并采集
func (a *Collector) Collect(account config.CloudAccount) {
	regions := account.Regions
	if len(regions) == 0 || (len(regions) == 1 && regions[0] == "*") {
		regions = a.getAllRegions(account)
	}

	log.Printf("Aliyun 开始账号采集 account_id=%s 区域数=%d", account.AccountID, len(regions))
	for _, region := range regions {
		log.Printf("Aliyun 开始区域采集 account_id=%s region=%s", account.AccountID, region)
		a.collectCMSMetrics(account, region)
		log.Printf("Aliyun 完成区域采集 account_id=%s region=%s", account.AccountID, region)
	}
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
	if a.cfg.ProductsByProvider != nil {
		if ps, ok := a.cfg.ProductsByProvider["aliyun"]; ok && len(ps) > 0 {
			prods = ps
		}
	}
	if len(prods) == 0 && a.cfg.ProductsByProviderLegacy != nil {
		if ps, ok := a.cfg.ProductsByProviderLegacy["aliyun"]; ok && len(ps) > 0 {
			prods = ps
		}
	}
	if len(prods) == 0 {
		prods = a.cfg.ProductsList
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

	for _, prod := range prods {
		if prod.Namespace == "" {
			continue
		}
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
			for _, metricName := range group.MetricList {
				_ = metrics.NamespaceGauge(prod.Namespace, metricName)
				meta := a.getMetricMeta(client, prod.Namespace, metricName)
				if period == "" && meta.MinPeriod != "" {
					period = meta.MinPeriod
				}
				if len(meta.Dimensions) == 0 {
					continue
				}
				dimKey := meta.Dimensions[0]
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
				if rtype == "bwp" {
					tagLabels = a.fetchCBWPTags(account, region, resIDs)
				}

				for start := 0; start < len(resIDs); start += 50 {
					end := start + 50
					if end > len(resIDs) {
						end = len(resIDs)
					}
					dims := make([]map[string]string, 0, end-start)
					for _, id := range resIDs[start:end] {
						dims = append(dims, map[string]string{dimKey: id})
					}
					dimsJSON, _ := json.Marshal(dims)

					req := cms.CreateDescribeMetricLastRequest()
					req.Namespace = prod.Namespace
					req.MetricName = metricName
					if period != "" {
						req.Period = period
					}
					if a.cfg.Server != nil && a.cfg.Server.PageSize > 0 {
						req.Length = strconv.Itoa(a.cfg.Server.PageSize)
					} else if a.cfg.ServerConf != nil && a.cfg.ServerConf.PageSize > 0 {
						req.Length = strconv.Itoa(a.cfg.ServerConf.PageSize)
					}
					req.Dimensions = string(dimsJSON)
					nextToken := ""
					log.Printf("Aliyun 拉取指标开始 account_id=%s region=%s namespace=%s metric=%s period=%s 维度数=%d", account.AccountID, region, prod.Namespace, metricName, period, len(dims))
					for {
						if nextToken != "" {
							req.NextToken = nextToken
						}
						startReq := time.Now()
						resp, err := client.DescribeMetricLast(req)
						if err != nil {
							log.Printf("CMS DescribeMetricLast error: %v", err)
							metrics.RequestTotal.WithLabelValues("aliyun", "DescribeMetricLast", "error").Inc()
							break
						}
						metrics.RequestTotal.WithLabelValues("aliyun", "DescribeMetricLast", "success").Inc()
						metrics.RequestDuration.WithLabelValues("aliyun", "DescribeMetricLast").Observe(time.Since(startReq).Seconds())
						var points []map[string]interface{}
						if err := json.Unmarshal([]byte(resp.Datapoints), &points); err != nil {
							break
						}
						log.Printf("Aliyun 拉取指标成功 account_id=%s region=%s namespace=%s metric=%s 返回点数=%d", account.AccountID, region, prod.Namespace, metricName, len(points))
						for _, p := range points {
							idAny, ok := p[dimKey]
							if !ok {
								continue
							}
							id, _ := idAny.(string)
							val := pickStatisticValue(p, chooseStatistics(meta.Statistics, group.Statistics))
							var tagsVal string
							if tagLabels != nil {
								tagsVal = tagLabels[id]
							}
							metrics.NamespaceGauge(prod.Namespace, metricName).
								WithLabelValues(
									"aliyun",
									account.AccountID,
									region,
									rtype,
									id,
									prod.Namespace,
									metricName,
									tagsVal,
								).Set(val)
						}
						if resp.NextToken == "" {
							break
						}
						nextToken = resp.NextToken
						time.Sleep(25 * time.Millisecond)
					}
					log.Printf("Aliyun 拉取指标完成 account_id=%s region=%s namespace=%s metric=%s", account.AccountID, region, prod.Namespace, metricName)
				}
			}
		}
	}
}

func (a *Collector) listECSInstanceIDs(account config.CloudAccount, region string) []string {
	log.Printf("Aliyun 枚举ECS实例开始 account_id=%s region=%s", account.AccountID, region)
	client, err := ecs.NewClientWithAccessKey(region, account.AccessKeyID, account.AccessKeySecret)
	if err != nil {
		return []string{}
	}
	request := ecs.CreateDescribeInstancesRequest()
	pageSize := 100
	if a.cfg != nil {
		if a.cfg.Server != nil && a.cfg.Server.PageSize > 0 {
			if a.cfg.Server.PageSize < pageSize {
				pageSize = a.cfg.Server.PageSize
			}
		} else if a.cfg.ServerConf != nil && a.cfg.ServerConf.PageSize > 0 {
			if a.cfg.ServerConf.PageSize < pageSize {
				pageSize = a.cfg.ServerConf.PageSize
			}
		}
	}
	request.PageSize = requests.NewInteger(pageSize)
	response, err := client.DescribeInstances(request)
	if err != nil {
		return []string{}
	}
	var ids []string
	for _, instance := range response.Instances.Instance {
		ids = append(ids, instance.InstanceId)
	}
	log.Printf("Aliyun 枚举ECS实例完成 account_id=%s region=%s 数量=%d", account.AccountID, region, len(ids))
	return ids
}

func (a *Collector) listCBWPIDs(account config.CloudAccount, region string) []string {
	log.Printf("Aliyun 枚举共享带宽包开始 account_id=%s region=%s", account.AccountID, region)
	client, err := vpc.NewClientWithAccessKey(region, account.AccessKeyID, account.AccessKeySecret)
	if err != nil {
		return []string{}
	}
	var ids []string
	pageSize := 50
	if a.cfg != nil {
		if a.cfg.Server != nil && a.cfg.Server.PageSize > 0 {
			if a.cfg.Server.PageSize < pageSize {
				pageSize = a.cfg.Server.PageSize
			}
		} else if a.cfg.ServerConf != nil && a.cfg.ServerConf.PageSize > 0 {
			if a.cfg.ServerConf.PageSize < pageSize {
				pageSize = a.cfg.ServerConf.PageSize
			}
		}
	}
	page := 1
	for {
		req := vpc.CreateDescribeCommonBandwidthPackagesRequest()
		req.RegionId = region
		req.PageSize = requests.NewInteger(pageSize)
		req.PageNumber = requests.NewInteger(page)
		start := time.Now()
		resp, err := client.DescribeCommonBandwidthPackages(req)
		if err != nil {
			msg := err.Error()
			if strings.Contains(msg, "InvalidParameter") || strings.Contains(msg, "InvalidPageSize") {
				req.PageSize = requests.NewInteger(pageSize)
				resp, err = client.DescribeCommonBandwidthPackages(req)
				if err == nil {
					goto HANDLE_RESP
				}
			}
			if strings.Contains(msg, "InvalidRegionId") || strings.Contains(msg, "Unsupported") {
				metrics.RequestTotal.WithLabelValues("aliyun", "DescribeCommonBandwidthPackages", "skip").Inc()
				log.Printf("Aliyun CBWP skip region=%s: %v", region, err)
				break
			}
			metrics.RequestTotal.WithLabelValues("aliyun", "DescribeCommonBandwidthPackages", "error").Inc()
			log.Printf("Aliyun CBWP describe error region=%s page=%d: %v", region, page, err)
			break
		}
	HANDLE_RESP:
		metrics.RequestTotal.WithLabelValues("aliyun", "DescribeCommonBandwidthPackages", "success").Inc()
		metrics.RequestDuration.WithLabelValues("aliyun", "DescribeCommonBandwidthPackages").Observe(time.Since(start).Seconds())
		if resp.CommonBandwidthPackages.CommonBandwidthPackage == nil || len(resp.CommonBandwidthPackages.CommonBandwidthPackage) == 0 {
			break
		}
		for _, pkg := range resp.CommonBandwidthPackages.CommonBandwidthPackage {
			ids = append(ids, pkg.BandwidthPackageId)
		}
		if len(resp.CommonBandwidthPackages.CommonBandwidthPackage) < pageSize {
			break
		}
		page++
		time.Sleep(50 * time.Millisecond)
	}
	log.Printf("Aliyun 枚举共享带宽包完成 account_id=%s region=%s 数量=%d", account.AccountID, region, len(ids))
	return ids
}

func (a *Collector) fetchCBWPTags(account config.CloudAccount, region string, ids []string) map[string]string {
	if len(ids) == 0 {
		return map[string]string{}
	}
	client, err := vpc.NewClientWithAccessKey(region, account.AccessKeyID, account.AccessKeySecret)
	if err != nil {
		return map[string]string{}
	}
	out := make(map[string]string, len(ids))
	const batchSize = 20
	for start := 0; start < len(ids); start += batchSize {
		end := start + batchSize
		if end > len(ids) {
			end = len(ids)
		}
		batch := ids[start:end]
		req := vpc.CreateListTagResourcesRequest()
		req.RegionId = region
		req.ResourceType = "COMMONBANDWIDTHPACKAGE"
		req.MaxResults = requests.NewInteger(50)
		req.ResourceId = &batch
		nextToken := ""
		for {
			if nextToken != "" {
				req.NextToken = nextToken
			}
			startReq := time.Now()
			resp, err := client.ListTagResources(req)
			if err != nil {
				metrics.RequestTotal.WithLabelValues("aliyun", "ListTagResources", "error").Inc()
				break
			}
			metrics.RequestTotal.WithLabelValues("aliyun", "ListTagResources", "success").Inc()
			metrics.RequestDuration.WithLabelValues("aliyun", "ListTagResources").Observe(time.Since(startReq).Seconds())
			if resp.TagResources.TagResource == nil {
				break
			}
			tmp := make(map[string]map[string]string)
			for _, tr := range resp.TagResources.TagResource {
				rid := tr.ResourceId
				if rid == "" {
					continue
				}
				if _, ok := tmp[rid]; !ok {
					tmp[rid] = make(map[string]string)
				}
				if tr.TagKey != "" {
					tmp[rid][tr.TagKey] = tr.TagValue
				}
			}
			for rid, kv := range tmp {
				var keys []string
				for k := range kv {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				var parts []string
				for _, k := range keys {
					parts = append(parts, k+"="+kv[k])
				}
				out[rid] = strings.Join(parts, ",")
			}
			if resp.NextToken == "" {
				break
			}
			nextToken = resp.NextToken
			time.Sleep(25 * time.Millisecond)
		}
		time.Sleep(25 * time.Millisecond)
	}
	return out
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

func resourceTypeForDim(dimKey string) string {
	switch dimKey {
	case "instanceId":
		return "ecs"
	case "sharebandwidthpackages", "bandwidthPackageId":
		return "bwp"
	default:
		return dimKey
	}
}

func resourceTypeForNamespace(namespace string) string {
	switch namespace {
	case "acs_ecs_dashboard":
		return "ecs"
	case "acs_bandwidth_package":
		return "bwp"
	default:
		return ""
	}
}

func (a *Collector) resourceIDsForNamespace(account config.CloudAccount, region string, namespace string) ([]string, string) {
	switch namespace {
	case "acs_ecs_dashboard":
		return a.listECSInstanceIDs(account, region), "ecs"
	case "acs_bandwidth_package":
		return a.listCBWPIDs(account, region), "bwp"
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
