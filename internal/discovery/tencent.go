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
	accounts = append(accounts, cfg.AccountsList...)
	if cfg.AccountsByProvider != nil {
		if xs, ok := cfg.AccountsByProvider["tencent"]; ok {
			accounts = append(accounts, xs...)
		}
	}
	if cfg.AccountsByProviderLegacy != nil {
		if xs, ok := cfg.AccountsByProviderLegacy["tencent"]; ok {
			accounts = append(accounts, xs...)
		}
	}
	logger.Log.Debugf("Starting Tencent discovery with %d accounts", len(accounts))
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
		client, err := newTencentMonitorClient(region, ak, sk)
		if err != nil {
			return prods
		}
		req := monitor.NewDescribeBaseMetricsRequest()
		req.Namespace = common.StringPtr("QCE/BWP")
		resp, err := client.DescribeBaseMetrics(req)
		if err != nil {
			logger.Log.Warnf("Tencent DescribeBaseMetrics QCE/BWP error: %v", err)
		}
		var metrics []string
		if err == nil && resp != nil && resp.Response != nil && resp.Response.MetricSet != nil {
			for _, m := range resp.Response.MetricSet {
				if m == nil || m.MetricName == nil {
					continue
				}
				metrics = append(metrics, *m.MetricName)
			}
		}
		// 兜底补充常用 BWP 指标
		fallback := []string{
			"InTraffic", "OutTraffic",
			"InPkg", "OutPkg",
			"IntrafficBwpRatio", "OuttrafficBwpRatio",
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
		client, err := newTencentMonitorClient(region, ak, sk)
		if err == nil {
			ns := "qce/gwlb"
			req := monitor.NewDescribeBaseMetricsRequest()
			req.Namespace = common.StringPtr(ns)
			resp, err := client.DescribeBaseMetrics(req)
			if err != nil {
				logger.Log.Warnf("Tencent DescribeBaseMetrics %s error: %v", ns, err)
			}
			var metrics []string
			if resp != nil && resp.Response != nil && resp.Response.MetricSet != nil {
				for _, m := range resp.Response.MetricSet {
					if m == nil || m.MetricName == nil {
						continue
					}
					metrics = append(metrics, *m.MetricName)
				}
			}
			fallback := []string{
				"InTraffic", "OutTraffic",
				"NewConn", "ConcurConn",
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
				prods = append(prods, config.Product{Namespace: ns, AutoDiscover: true, MetricInfo: []config.MetricGroup{{MetricList: metrics}}})
			}
		}
	}
	if needCLB {
		region := "ap-guangzhou"
		if len(accounts) > 0 && len(accounts[0].Regions) > 0 && accounts[0].Regions[0] != "*" {
			region = accounts[0].Regions[0]
		}
		ak := accounts[0].AccessKeyID
		sk := accounts[0].AccessKeySecret
		client, err := newTencentMonitorClient(region, ak, sk)
		if err == nil {
			ns := "QCE/LB"
			req := monitor.NewDescribeBaseMetricsRequest()
			req.Namespace = common.StringPtr(ns)
			resp, err := client.DescribeBaseMetrics(req)
			if err != nil {
				logger.Log.Warnf("Tencent DescribeBaseMetrics %s error: %v", ns, err)
			}
			var metrics []string
			if resp != nil && resp.Response != nil && resp.Response.MetricSet != nil {
				for _, m := range resp.Response.MetricSet {
					if m == nil || m.MetricName == nil {
						continue
					}
					metrics = append(metrics, *m.MetricName)
				}
			}
			fallback := []string{
				"VipIntraffic", "VipOuttraffic",
				"VipInpkg", "VipOutpkg",
				"Vipindroppkts", "Vipoutdroppkts",
				"IntrafficVipRatio", "OuttrafficVipRatio",
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
				prods = append(prods, config.Product{Namespace: ns, AutoDiscover: true, MetricInfo: []config.MetricGroup{{MetricList: metrics}}})
			}
		}
	}
	if needCOS {
		region := "ap-guangzhou"
		if len(accounts) > 0 && len(accounts[0].Regions) > 0 && accounts[0].Regions[0] != "*" {
			region = accounts[0].Regions[0]
		}
		ak := accounts[0].AccessKeyID
		sk := accounts[0].AccessKeySecret
		client, err := newTencentMonitorClient(region, ak, sk)
		if err == nil {
			req := monitor.NewDescribeBaseMetricsRequest()
			req.Namespace = common.StringPtr("QCE/COS")
			resp, err := client.DescribeBaseMetrics(req)
			if err == nil && resp != nil && resp.Response != nil {
				var metrics []string
				if resp.Response.MetricSet != nil {
					for _, m := range resp.Response.MetricSet {
						if m == nil || m.MetricName == nil {
							continue
						}
						metrics = append(metrics, *m.MetricName)
					}
				}
				// 兜底补充常用 COS 指标
				fallback := []string{
					"StdStorage", "SiaStorage", "ArcStorage", "DeepArcStorage",
					"ItFreqStorage", "ItInfreqStorage",
					"InternetTraffic", "InternalTraffic", "CdnOriginTraffic",
					"TotalRequests", "GetRequests", "PutRequests",
					"4xxResponse", "5xxResponse",
					"2xxResponse", "3xxResponse",
					"CrossRegionReplicationTraffic",
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
