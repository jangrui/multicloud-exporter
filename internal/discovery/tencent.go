package discovery

import (
	"context"

	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/logger"

	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	monitor "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/monitor/v20180724"
)

var newTencentMonitorClient = func(region, ak, sk string) (*monitor.Client, error) {
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
	for _, acc := range accounts {
		for _, r := range acc.Resources {
			if r == "bwp" || r == "cbwp" || r == "*" {
				needBWP = true
			}
			if r == "lb" || r == "clb" || r == "*" {
				needCLB = true
			}
			if r == "cos" || r == "*" {
				needCOS = true
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
	if needCLB {
		region := "ap-guangzhou"
		if len(accounts) > 0 && len(accounts[0].Regions) > 0 && accounts[0].Regions[0] != "*" {
			region = accounts[0].Regions[0]
		}
		ak := accounts[0].AccessKeyID
		sk := accounts[0].AccessKeySecret
		client, err := newTencentMonitorClient(region, ak, sk)
		if err == nil {
			req := monitor.NewDescribeBaseMetricsRequest()
			req.Namespace = common.StringPtr("QCE/LB")
			resp, err := client.DescribeBaseMetrics(req)
			if err != nil {
				logger.Log.Warnf("Tencent DescribeBaseMetrics QCE/LB error: %v", err)
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
			// 兜底补充常用 CLB 指标，避免云侧元数据缺失导致无法采集
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
				prods = append(prods, config.Product{Namespace: "QCE/LB", AutoDiscover: true, MetricInfo: []config.MetricGroup{{MetricList: metrics}}})
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
					"StdStorage", "InfrequentStorage", "ArchiveStorage", "StandardIAStorage",
					"DeepArchiveStorage", "IntelligentStorage",
					"InternetTraffic", "InternalTraffic", "CdnOriginTraffic",
					"Requests", "GetRequests", "PutRequests",
					"4xxErrors", "5xxErrors",
					"2xxResponse", "3xxResponse", "4xxResponse", "5xxResponse",
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
