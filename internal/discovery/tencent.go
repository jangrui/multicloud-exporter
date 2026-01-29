package discovery

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/logger"

	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	monitor "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/monitor/v20180724"
)

type MonitorClient interface {
	DescribeBaseMetrics(request *monitor.DescribeBaseMetricsRequest) (response *monitor.DescribeBaseMetricsResponse, err error)
}

var newTencentMonitorClient = func(region, ak, sk string) (MonitorClient, error) {
	cred := common.NewCredential(ak, sk)
	return monitor.NewClient(cred, region, profile.NewClientProfile())
}

type TencentDiscoverer struct{}

func (d *TencentDiscoverer) Discover(ctx context.Context, cfg *config.Config) []config.Product {
	if cfg == nil {
		return nil
	}
	var accounts []config.CloudAccount
	if cfg.AccountsByProvider != nil {
		if xs, ok := cfg.AccountsByProvider["tencent"]; ok {
			accounts = append(accounts, xs...)
		}
	}
	ctxLog := logger.NewContextLogger("Tencent", "resource_type", "Discovery")
	ctxLog.Debugf("发现服务开始，账号数量=%d", len(accounts))
	if len(accounts) == 0 {
		return nil
	}
	cache := getDiscoveryMetaCache(cfg)
	needBWP := false
	needCLB := false
	needCOS := false
	needGWLB := false
	for _, acc := range accounts {
		for _, r := range acc.Resources {
			rr := r
			if rr != "" {
				rr = strings.ToLower(rr)
			}
			if rr == "bwp" || rr == "*" {
				needBWP = true
			}
			if rr == "clb" || rr == "*" {
				needCLB = true
			}
			if rr == "s3" || rr == "*" {
				needCOS = true
			}
			if rr == "gwlb" || rr == "*" {
				needGWLB = true
			}
		}
	}
	prods := make([]config.Product, 0)
	if needBWP {
		if customGroups, ok := collectTencentProductMetric(accounts, "bwp"); ok {
			bwpProd := buildTencentBWPProductWithCustomConfig(customGroups)
			prods = append(prods, bwpProd)
			ctxLog := logger.NewContextLogger("Tencent", "resource_type", "Discovery", "namespace", "QCE/BWP")
			ctxLog.Infof("发现服务完成，使用自定义配置，MetricGroup 数量=%d", len(bwpProd.MetricInfo))
		} else {
			cached := false
			if cache != nil {
				if cachedGroups, ok := cache.Get("tencent", "QCE/BWP", accounts[0]); ok {
					prods = append(prods, config.Product{Namespace: "QCE/BWP", AutoDiscover: true, MetricInfo: cachedGroups})
					ctxLog := logger.NewContextLogger("Tencent", "resource_type", "Discovery", "namespace", "QCE/BWP")
					ctxLog.Infof("发现服务命中缓存，MetricGroup 数量=%d", len(cachedGroups))
					cached = true
				}
			}
			if cached {
				// 已使用缓存
			} else {
				region := "ap-guangzhou"
				if len(accounts) > 0 && len(accounts[0].Regions) > 0 && accounts[0].Regions[0] != "*" {
					region = accounts[0].Regions[0]
				}
				ak := accounts[0].AccessKeyID
				sk := accounts[0].AccessKeySecret

				// 兜底补充常用 BWP 指标
				fallback := []string{
					"InTraffic", "OutTraffic",
					"InPkg", "OutPkg",
					"IntrafficBwpRatio", "OuttrafficBwpRatio",
				}

				var metrics []string
				client, err := newTencentMonitorClient(region, ak, sk)
				if err != nil {
					ctxLog := logger.NewContextLogger("Tencent", "resource_type", "Discovery", "namespace", "QCE/BWP")
					ctxLog.Warnf("客户端创建失败，错误=%v", err)
				} else {
					req := monitor.NewDescribeBaseMetricsRequest()
					req.Namespace = common.StringPtr("QCE/BWP")
					resp, err := client.DescribeBaseMetrics(req)
					if err != nil {
						ctxLog := logger.NewContextLogger("Tencent", "resource_type", "Discovery", "namespace", "QCE/BWP")
						ctxLog.Warnf("DescribeBaseMetrics API调用错误，错误=%v", err)
					}
					if err == nil && resp != nil && resp.Response != nil && resp.Response.MetricSet != nil {
						for _, m := range resp.Response.MetricSet {
							if m == nil || m.MetricName == nil {
								continue
							}
							metrics = append(metrics, *m.MetricName)
						}
					}
				}

				cur := make(map[string]struct{}, len(metrics))
				for _, m := range metrics {
					cur[m] = struct{}{}
				}
				for _, m := range fallback {
					if _, ok := cur[m]; !ok {
						metrics = append(metrics, m)
					}
				}
				if len(metrics) > 0 {
					prod := buildTencentBWPProductDefault(metrics)
					prods = append(prods, prod)
					if cache != nil {
						cache.Set("tencent", "QCE/BWP", accounts[0], prod.MetricInfo)
					}
					ctxLog := logger.NewContextLogger("Tencent", "resource_type", "Discovery", "namespace", "QCE/BWP")
					ctxLog.Infof("发现服务完成，指标数量=%d", len(metrics))
				} else {
					ctxLog := logger.NewContextLogger("Tencent", "resource_type", "Discovery", "namespace", "QCE/BWP")
					ctxLog.Warnf("发现服务未发现指标")
				}
			}
		}
	}
	if needGWLB {
		if customGroups, ok := collectTencentProductMetric(accounts, "gwlb"); ok {
			gwlbProd := buildTencentGWLBProductWithCustomConfig(customGroups)
			prods = append(prods, gwlbProd)
			ctxLog := logger.NewContextLogger("Tencent", "resource_type", "Discovery", "namespace", "qce/gwlb")
			ctxLog.Infof("发现服务完成，使用自定义配置，MetricGroup 数量=%d", len(gwlbProd.MetricInfo))
		} else {
			cached := false
			if cache != nil {
				if cachedGroups, ok := cache.Get("tencent", "qce/gwlb", accounts[0]); ok {
					prods = append(prods, config.Product{Namespace: "qce/gwlb", AutoDiscover: true, MetricInfo: cachedGroups})
					ctxLog := logger.NewContextLogger("Tencent", "resource_type", "Discovery", "namespace", "qce/gwlb")
					ctxLog.Infof("发现服务命中缓存，MetricGroup 数量=%d", len(cachedGroups))
					cached = true
				}
			}
			if cached {
				// 已使用缓存
			} else {
				region := "ap-guangzhou"
				if len(accounts) > 0 && len(accounts[0].Regions) > 0 && accounts[0].Regions[0] != "*" {
					region = accounts[0].Regions[0]
				}
				ak := accounts[0].AccessKeyID
				sk := accounts[0].AccessKeySecret

				fallback := []string{
					"InTraffic", "OutTraffic",
					"NewConn", "ConcurConn",
					"Unhealthyrscount",
				}

				var metrics []string
				client, err := newTencentMonitorClient(region, ak, sk)
				if err != nil {
					ctxLog := logger.NewContextLogger("Tencent", "resource_type", "Discovery", "namespace", "qce/gwlb")
					ctxLog.Warnf("客户端创建失败，错误=%v", err)
				} else {
					ns := "qce/gwlb"
					req := monitor.NewDescribeBaseMetricsRequest()
					req.Namespace = common.StringPtr(ns)
					resp, err := client.DescribeBaseMetrics(req)
					if err != nil {
						ctxLog := logger.NewContextLogger("Tencent", "resource_type", "Discovery", "namespace", ns)
						ctxLog.Warnf("DescribeBaseMetrics API调用错误，错误=%v", err)
					}
					if resp != nil && resp.Response != nil && resp.Response.MetricSet != nil {
						for _, m := range resp.Response.MetricSet {
							if m == nil || m.MetricName == nil {
								continue
							}
							metrics = append(metrics, *m.MetricName)
						}
					}
				}

				cur := make(map[string]struct{}, len(metrics))
				for _, m := range metrics {
					cur[m] = struct{}{}
				}
				for _, m := range fallback {
					if _, ok := cur[m]; !ok {
						metrics = append(metrics, m)
					}
				}
				if len(metrics) > 0 {
					prod := buildTencentGWLBProductDefault(metrics)
					prods = append(prods, prod)
					if cache != nil {
						cache.Set("tencent", "qce/gwlb", accounts[0], prod.MetricInfo)
					}
					ctxLog := logger.NewContextLogger("Tencent", "resource_type", "Discovery", "namespace", "qce/gwlb")
					ctxLog.Infof("发现服务完成，指标数量=%d", len(metrics))
				} else {
					ctxLog := logger.NewContextLogger("Tencent", "resource_type", "Discovery", "namespace", "qce/gwlb")
					ctxLog.Warnf("发现服务未发现指标")
				}
			}
		}
	}
	if needCLB {
		if customGroups, ok := collectTencentProductMetric(accounts, "clb"); ok {
			clbProd := buildTencentCLBProductWithCustomConfig(customGroups)
			prods = append(prods, clbProd)
			ctxLog := logger.NewContextLogger("Tencent", "resource_type", "Discovery", "namespace", "QCE/LB")
			ctxLog.Infof("发现服务完成，使用自定义配置，MetricGroup 数量=%d", len(clbProd.MetricInfo))
		} else {
			cached := false
			if cache != nil {
				if cachedGroups, ok := cache.Get("tencent", "QCE/LB", accounts[0]); ok {
					prods = append(prods, config.Product{Namespace: "QCE/LB", AutoDiscover: true, MetricInfo: cachedGroups})
					ctxLog := logger.NewContextLogger("Tencent", "resource_type", "Discovery", "namespace", "QCE/LB")
					ctxLog.Infof("发现服务命中缓存，MetricGroup 数量=%d", len(cachedGroups))
					cached = true
				}
			}
			if cached {
				// 已使用缓存
			} else {
				region := "ap-guangzhou"
				if len(accounts) > 0 && len(accounts[0].Regions) > 0 && accounts[0].Regions[0] != "*" {
					region = accounts[0].Regions[0]
				}
				ak := accounts[0].AccessKeyID
				sk := accounts[0].AccessKeySecret

				fallback := []string{
					"VipIntraffic", "VipOuttraffic",
					"VipInpkg", "VipOutpkg",
					"Vipindroppkts", "Vipoutdroppkts",
					"IntrafficVipRatio", "OuttrafficVipRatio",
					"ClientIntraffic", "ClientOuttraffic",
					"ClientInpkg", "ClientOutpkg",
					"InDropPkts", "OutDropPkts",
					"VNewConn", "VConnum",
					// 核心指标：配置文件中的主要映射指标
					"VIntraffic", "VOuttraffic", // 流量指标（实际采集到的指标名）
					"VInpkg", "VOutpkg", // 包速率指标（实际采集到的指标名）
					"NewConn", // 新建连接数
					// 腾讯云特殊实例类型指标
					"PvvNewConn", "PvvOutpkg", "PvvOuttraffic",
					"RrvConnum", "RrvInactiveConn", "RrvInpkg", "RrvIntraffic", "RrvNewConn", "RrvOutpkg", "RrvOuttraffic",
					"RvConnum", "RvInactiveConn", "RvInpkg", "RvIntraffic", "RvNewConn", "RvOutpkg", "RvOuttraffic",
				}

				var metrics []string
				client, err := newTencentMonitorClient(region, ak, sk)
				if err != nil {
					ctxLog := logger.NewContextLogger("Tencent", "resource_type", "Discovery", "namespace", "QCE/LB")
					ctxLog.Warnf("客户端创建失败，错误=%v", err)
				} else {
					ns := "QCE/LB"
					req := monitor.NewDescribeBaseMetricsRequest()
					req.Namespace = common.StringPtr(ns)
					resp, err := client.DescribeBaseMetrics(req)
					if err != nil {
						ctxLog := logger.NewContextLogger("Tencent", "resource_type", "Discovery", "namespace", ns)
						ctxLog.Warnf("DescribeBaseMetrics API调用错误，错误=%v", err)
					}
					if resp != nil && resp.Response != nil && resp.Response.MetricSet != nil {
						for _, m := range resp.Response.MetricSet {
							if m == nil || m.MetricName == nil {
								continue
							}
							metrics = append(metrics, *m.MetricName)
						}
					}
				}

				cur := make(map[string]struct{}, len(metrics))
				for _, m := range metrics {
					cur[m] = struct{}{}
				}
				for _, m := range fallback {
					if _, ok := cur[m]; !ok {
						metrics = append(metrics, m)
					}
				}
				if len(metrics) > 0 {
					prod := buildTencentCLBProductDefault(metrics)
					prods = append(prods, prod)
					if cache != nil {
						cache.Set("tencent", "QCE/LB", accounts[0], prod.MetricInfo)
					}
					ctxLog := logger.NewContextLogger("Tencent", "resource_type", "Discovery", "namespace", "QCE/LB")
					ctxLog.Infof("发现服务完成，指标数量=%d", len(metrics))
				} else {
					ctxLog := logger.NewContextLogger("Tencent", "resource_type", "Discovery", "namespace", "QCE/LB")
					ctxLog.Warnf("发现服务未发现指标")
				}
			}
		}
	}
	if needCOS {
		if customGroups, ok := collectTencentProductMetric(accounts, "cos"); ok {
			cosProd := buildCOSProductWithCustomConfig(customGroups)
			prods = append(prods, cosProd)
			ctxLog := logger.NewContextLogger("Tencent", "resource_type", "Discovery", "namespace", "QCE/COS")
			ctxLog.Infof("发现服务完成，使用自定义配置，MetricGroup 数量=%d", len(cosProd.MetricInfo))
			return prods
		}
		if cache != nil {
			if cachedGroups, ok := cache.Get("tencent", "QCE/COS", accounts[0]); ok {
				prods = append(prods, config.Product{Namespace: "QCE/COS", AutoDiscover: true, MetricInfo: cachedGroups})
				ctxLog := logger.NewContextLogger("Tencent", "resource_type", "Discovery", "namespace", "QCE/COS")
				ctxLog.Infof("发现服务命中缓存，MetricGroup 数量=%d", len(cachedGroups))
				return prods
			}
		}
		region := "ap-guangzhou"
		if len(accounts) > 0 && len(accounts[0].Regions) > 0 && accounts[0].Regions[0] != "*" {
			region = accounts[0].Regions[0]
		}
		ak := accounts[0].AccessKeyID
		sk := accounts[0].AccessKeySecret

		// COS 容量类指标（每日更新，需要 Period=86400）
		// 注意：StdMultipartStorage 和 SiaMultipartStorage 在腾讯云 COS 中不存在，已移除
		capacityFallback := []string{
			"StdStorage", "SiaStorage", "ArcStorage", "DeepArcStorage",
			"ItFreqStorage", "ItInfreqStorage",
			"DeepArcMultipartStorage", "DeepArcMultipartNumber", "DeepArcObjectNumber",
		}

		// COS 请求类指标（实时更新，使用 Period=300）
		requestFallback := []string{
			"InternetTraffic", "InternalTraffic", "CdnOriginTraffic",
			"InternetTrafficUp", "InternetTrafficDown",
			"InternalTrafficUp", "InternalTrafficDown",
			"TotalRequests", "GetRequests", "PutRequests", "HeadRequests",
			"DeleteObjectRequestsPs", "DeleteMultiObjRequestsPs", "PutObjectCopyRequestsPs",
			"4xxResponse", "5xxResponse",
			"2xxResponse", "3xxResponse",
			"2xxResponseRate", "3xxResponseRate", "4xxResponseRate", "5xxResponseRate",
			"RequestsSuccessRate",
			"CrossRegionReplicationTraffic",
			"FirstByteDelay",
			"FetchBandwidth",
			"DeepArcStandardRetrieval", "DeepArcBulkRetrieval", "DeepArcReadRequests", "DeepArcWriteRequests",
		}

		// 容量类指标集合（用于分类）
		capacitySet := map[string]bool{
			"StdStorage": true, "SiaStorage": true, "ArcStorage": true, "DeepArcStorage": true,
			"ItFreqStorage": true, "ItInfreqStorage": true,
			"DeepArcMultipartStorage": true, "DeepArcMultipartNumber": true, "DeepArcObjectNumber": true,
		}

		var capacityMetrics, requestMetrics []string
		client, err := newTencentMonitorClient(region, ak, sk)
		if err != nil {
			ctxLog := logger.NewContextLogger("Tencent", "resource_type", "Discovery", "namespace", "QCE/COS")
			ctxLog.Warnf("客户端创建失败，错误=%v", err)
			// 使用兜底指标
			capacityMetrics = capacityFallback
			requestMetrics = requestFallback
		} else {
			req := monitor.NewDescribeBaseMetricsRequest()
			req.Namespace = common.StringPtr("QCE/COS")
			resp, err := client.DescribeBaseMetrics(req)
			if err != nil {
				ctxLog := logger.NewContextLogger("Tencent", "resource_type", "Discovery", "namespace", "QCE/COS")
				ctxLog.Warnf("DescribeBaseMetrics API调用错误，错误=%v", err)
			}
			if err == nil && resp != nil && resp.Response != nil && resp.Response.MetricSet != nil {
				for _, m := range resp.Response.MetricSet {
					if m == nil || m.MetricName == nil {
						continue
					}
					name := *m.MetricName
					// 根据指标名称分类
					if capacitySet[name] {
						capacityMetrics = append(capacityMetrics, name)
					} else {
						requestMetrics = append(requestMetrics, name)
					}
				}
			}
		}

		// 合并兜底指标 - 容量类
		capCur := make(map[string]struct{}, len(capacityMetrics))
		for _, m := range capacityMetrics {
			capCur[m] = struct{}{}
		}
		for _, m := range capacityFallback {
			if _, ok := capCur[m]; !ok {
				capacityMetrics = append(capacityMetrics, m)
			}
		}

		// 合并兜底指标 - 请求类
		reqCur := make(map[string]struct{}, len(requestMetrics))
		for _, m := range requestMetrics {
			reqCur[m] = struct{}{}
		}
		for _, m := range requestFallback {
			if _, ok := reqCur[m]; !ok {
				requestMetrics = append(requestMetrics, m)
			}
		}

		if len(capacityMetrics) > 0 || len(requestMetrics) > 0 {
			// 创建包含两个 MetricGroup 的产品配置：
			// - 容量类指标：Period=86400（每日更新）
			// - 请求类指标：Period=300（实时更新）
			metricGroups := []config.MetricGroup{}
			if len(capacityMetrics) > 0 {
				capacityPeriod := 86400
				metricGroups = append(metricGroups, config.MetricGroup{
					Period:     &capacityPeriod,
					MetricList: capacityMetrics,
				})
			}
			if len(requestMetrics) > 0 {
				requestPeriod := 300
				metricGroups = append(metricGroups, config.MetricGroup{
					Period:     &requestPeriod,
					MetricList: requestMetrics,
				})
			}
			prods = append(prods, config.Product{Namespace: "QCE/COS", AutoDiscover: true, MetricInfo: metricGroups})
			if cache != nil {
				cache.Set("tencent", "QCE/COS", accounts[0], metricGroups)
			}
			ctxLog := logger.NewContextLogger("Tencent", "resource_type", "Discovery", "namespace", "QCE/COS")
			ctxLog.Infof("发现服务完成，容量类指标=%d 请求类指标=%d", len(capacityMetrics), len(requestMetrics))
		} else {
			ctxLog := logger.NewContextLogger("Tencent", "resource_type", "Discovery", "namespace", "QCE/COS")
			ctxLog.Warnf("发现服务未发现指标")
		}
	}
	return prods
}

func buildTencentCLBProductDefault(fallbackList []string) config.Product {
	metricGroups := []config.MetricGroup{}
	if len(fallbackList) > 0 {
		metricGroups = append(metricGroups, config.MetricGroup{
			MetricList: fallbackList,
		})
	}
	return config.Product{
		Namespace:    "QCE/LB",
		AutoDiscover: true,
		MetricInfo:   metricGroups,
	}
}

func buildTencentCLBProductWithCustomConfig(metricGroups []config.MetricGroupConfig) config.Product {
	var metricInfo []config.MetricGroup
	for _, mg := range metricGroups {
		metricInfo = append(metricInfo, config.MetricGroup{
			Period:     mg.Period,
			MetricList: mg.MetricList,
		})
	}
	return config.Product{
		Namespace:    "QCE/LB",
		AutoDiscover: false,
		MetricInfo:   metricInfo,
	}
}

func buildTencentGWLBProductDefault(fallbackList []string) config.Product {
	metricGroups := []config.MetricGroup{}
	if len(fallbackList) > 0 {
		metricGroups = append(metricGroups, config.MetricGroup{
			MetricList: fallbackList,
		})
	}
	return config.Product{
		Namespace:    "qce/gwlb",
		AutoDiscover: true,
		MetricInfo:   metricGroups,
	}
}

func buildTencentGWLBProductWithCustomConfig(metricGroups []config.MetricGroupConfig) config.Product {
	var metricInfo []config.MetricGroup
	for _, mg := range metricGroups {
		metricInfo = append(metricInfo, config.MetricGroup{
			Period:     mg.Period,
			MetricList: mg.MetricList,
		})
	}
	return config.Product{
		Namespace:    "qce/gwlb",
		AutoDiscover: false,
		MetricInfo:   metricInfo,
	}
}

func buildTencentBWPProductDefault(fallbackList []string) config.Product {
	metricGroups := []config.MetricGroup{}
	if len(fallbackList) > 0 {
		metricGroups = append(metricGroups, config.MetricGroup{
			MetricList: fallbackList,
		})
	}
	return config.Product{
		Namespace:    "QCE/BWP",
		AutoDiscover: true,
		MetricInfo:   metricGroups,
	}
}

func buildTencentBWPProductWithCustomConfig(metricGroups []config.MetricGroupConfig) config.Product {
	var metricInfo []config.MetricGroup
	for _, mg := range metricGroups {
		metricInfo = append(metricInfo, config.MetricGroup{
			Period:     mg.Period,
			MetricList: mg.MetricList,
		})
	}
	return config.Product{
		Namespace:    "QCE/BWP",
		AutoDiscover: false,
		MetricInfo:   metricInfo,
	}
}

func buildCOSProductWithCustomConfig(metricGroups []config.MetricGroupConfig) config.Product {
	var metricInfo []config.MetricGroup
	for _, mg := range metricGroups {
		metricInfo = append(metricInfo, config.MetricGroup{
			Period:     mg.Period,
			MetricList: mg.MetricList,
		})
	}
	return config.Product{
		Namespace:    "QCE/COS",
		AutoDiscover: false,
		MetricInfo:   metricInfo,
	}
}

func collectTencentProductMetric(accounts []config.CloudAccount, productName string) ([]config.MetricGroupConfig, bool) {
	keys := []string{productName}
	if productName == "cos" {
		keys = append(keys, "s3")
	}

	var allGroups []config.MetricGroupConfig
	for _, acc := range accounts {
		if acc.ProductMetric == nil {
			continue
		}
		for _, key := range keys {
			if groups, ok := acc.ProductMetric[key]; ok && len(groups) > 0 {
				allGroups = append(allGroups, groups...)
			}
		}
	}
	if len(allGroups) == 0 {
		return nil, false
	}
	return unionTencentMetricGroups(allGroups), true
}

func unionTencentMetricGroups(groups []config.MetricGroupConfig) []config.MetricGroupConfig {
	merged := make(map[int]map[string]struct{})
	for _, group := range groups {
		periodKey := 0
		if group.Period != nil {
			periodKey = *group.Period
		}
		if merged[periodKey] == nil {
			merged[periodKey] = make(map[string]struct{})
		}
		for _, metric := range group.MetricList {
			merged[periodKey][metric] = struct{}{}
		}
	}

	var periods []int
	for period := range merged {
		periods = append(periods, period)
	}
	sort.Ints(periods)

	var result []config.MetricGroupConfig
	for _, period := range periods {
		metricSet := merged[period]
		metrics := make([]string, 0, len(metricSet))
		for metric := range metricSet {
			metrics = append(metrics, metric)
		}
		sort.Strings(metrics)

		var p *int
		if period > 0 {
			p = &period
		}
		result = append(result, config.MetricGroupConfig{
			Period:     p,
			MetricList: metrics,
		})
	}
	return result
}

func init() {
	Register("tencent", &TencentDiscoverer{})
}

func FetchTencentMetricMeta(region, ak, sk, namespace string) ([]MetricMeta, error) {
	client, err := newTencentMonitorClient(region, ak, sk)
	if err != nil {
		return nil, err
	}
	req := monitor.NewDescribeBaseMetricsRequest()
	req.Namespace = common.StringPtr(namespace)
	resp, err := client.DescribeBaseMetrics(req)
	if err != nil || resp == nil || resp.Response == nil || resp.Response.MetricSet == nil {
		return nil, err
	}
	var out []MetricMeta
	for _, m := range resp.Response.MetricSet {
		if m == nil || m.MetricName == nil {
			continue
		}
		var dims []string
		if m.Dimensions != nil {
			// 尝试通过 JSON 泛化解析可能的字段形态
			if b, e := json.Marshal(m.Dimensions); e == nil {
				var xs []map[string]interface{}
				if je := json.Unmarshal(b, &xs); je == nil {
					for _, item := range xs {
						// 常见字段：Name / Key
						if v, ok := item["Name"].(string); ok && v != "" {
							dims = append(dims, v)
							continue
						}
						if v, ok := item["Key"].(string); ok && v != "" {
							dims = append(dims, v)
							continue
						}
						// 有的结构会提供 Dimensions: ["vip", "loadBalancerId"]
						if arr, ok := item["Dimensions"].([]interface{}); ok {
							for _, d := range arr {
								if sv, ok := d.(string); ok && sv != "" {
									dims = append(dims, sv)
								}
							}
						}
					}
				}
			}
		}
		// 兜底：根据默认资源维度映射补充可能的主键
		if len(dims) == 0 {
			defaults := config.DefaultResourceDimMapping()
			key := "tencent." + namespace
			if req, ok := defaults[key]; ok {
				dims = append(dims, req...)
			}
		}
		desc := ""
		if m.Meaning != nil && m.Meaning.Zh != nil {
			desc = *m.Meaning.Zh
		}
		unit := ""
		if m.Unit != nil {
			unit = *m.Unit
		}
		mm := MetricMeta{
			Provider:    "tencent",
			Namespace:   namespace,
			Name:        *m.MetricName,
			Unit:        unit,
			Dimensions:  dims,
			Description: desc,
		}
		out = append(out, mm)
	}
	return out, nil
}
