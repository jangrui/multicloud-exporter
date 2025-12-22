package discovery

import (
	"context"
	"encoding/json"
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
	logger.Log.Debugf("Tencent 发现服务开始，账号数量=%d", len(accounts))
	if len(accounts) == 0 {
		return nil
	}
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
			logger.Log.Warnf("Tencent 客户端创建失败，命名空间=QCE/BWP 错误=%v", err)
		} else {
			req := monitor.NewDescribeBaseMetricsRequest()
			req.Namespace = common.StringPtr("QCE/BWP")
			resp, err := client.DescribeBaseMetrics(req)
			if err != nil {
				logger.Log.Warnf("Tencent DescribeBaseMetrics 错误，命名空间=QCE/BWP 错误=%v", err)
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
			prods = append(prods, config.Product{Namespace: "QCE/BWP", AutoDiscover: true, MetricInfo: []config.MetricGroup{{MetricList: metrics}}})
		}
	}
	if needGWLB {
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
			logger.Log.Warnf("Tencent 客户端创建失败，命名空间=qce/gwlb 错误=%v", err)
		} else {
			ns := "qce/gwlb"
			req := monitor.NewDescribeBaseMetricsRequest()
			req.Namespace = common.StringPtr(ns)
			resp, err := client.DescribeBaseMetrics(req)
			if err != nil {
				logger.Log.Warnf("Tencent DescribeBaseMetrics 错误，命名空间=%s 错误=%v", ns, err)
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
			prods = append(prods, config.Product{Namespace: "qce/gwlb", AutoDiscover: true, MetricInfo: []config.MetricGroup{{MetricList: metrics}}})
		}
	}
	if needCLB {
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
			logger.Log.Warnf("Tencent 客户端创建失败，命名空间=QCE/LB 错误=%v", err)
		} else {
			ns := "QCE/LB"
			req := monitor.NewDescribeBaseMetricsRequest()
			req.Namespace = common.StringPtr(ns)
			resp, err := client.DescribeBaseMetrics(req)
			if err != nil {
				logger.Log.Warnf("Tencent DescribeBaseMetrics 错误，命名空间=%s 错误=%v", ns, err)
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
			prods = append(prods, config.Product{Namespace: "QCE/LB", AutoDiscover: true, MetricInfo: []config.MetricGroup{{MetricList: metrics}}})
		}
	}
	if needCOS {
		region := "ap-guangzhou"
		if len(accounts) > 0 && len(accounts[0].Regions) > 0 && accounts[0].Regions[0] != "*" {
			region = accounts[0].Regions[0]
		}
		ak := accounts[0].AccessKeyID
		sk := accounts[0].AccessKeySecret

		// 兜底补充常用 COS 指标
		fallback := []string{
			"StdStorage", "SiaStorage", "ArcStorage", "DeepArcStorage",
			"ItFreqStorage", "ItInfreqStorage",
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
			"DeepArcMultipartStorage", "DeepArcMultipartNumber", "DeepArcObjectNumber",
			"DeepArcStandardRetrieval", "DeepArcBulkRetrieval", "DeepArcReadRequests", "DeepArcWriteRequests",
		}

		var metrics []string
		client, err := newTencentMonitorClient(region, ak, sk)
		if err != nil {
			logger.Log.Warnf("Tencent 客户端创建失败，命名空间=QCE/COS 错误=%v", err)
		} else {
			req := monitor.NewDescribeBaseMetricsRequest()
			req.Namespace = common.StringPtr("QCE/COS")
			resp, err := client.DescribeBaseMetrics(req)
			if err != nil {
				logger.Log.Warnf("Tencent DescribeBaseMetrics 错误，命名空间=QCE/COS 错误=%v", err)
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
			prods = append(prods, config.Product{Namespace: "QCE/COS", AutoDiscover: true, MetricInfo: []config.MetricGroup{{MetricList: metrics}}})
		}
	}
	return prods
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
