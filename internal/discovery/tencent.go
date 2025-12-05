package discovery

import (
	"context"

	"multicloud-exporter/internal/config"

	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	monitor "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/monitor/v20180724"
)

var newTencentMonitorClient = func(region, ak, sk string) (*monitor.Client, error) {
	cred := common.NewCredential(ak, sk)
	return monitor.NewClient(cred, region, profile.NewClientProfile())
}

func discoverTencent(ctx context.Context, cfg *config.Config) []config.Product {
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
	if len(accounts) == 0 {
		return nil
	}
	needBWP := false
	for _, acc := range accounts {
		for _, r := range acc.Resources {
			if r == "bwp" || r == "*" {
				needBWP = true
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
		if err != nil || resp == nil || resp.Response == nil {
			return prods
		}
		var metrics []string
		if resp.Response.MetricSet != nil {
			for _, m := range resp.Response.MetricSet {
				if m == nil || m.MetricName == nil {
					continue
				}
				metrics = append(metrics, *m.MetricName)
			}
		}
		if len(metrics) > 0 {
			prods = append(prods, config.Product{Namespace: "QCE/BWP", AutoDiscover: true, MetricInfo: []config.MetricGroup{{MetricList: metrics}}})
		}
	}
	return prods
}
