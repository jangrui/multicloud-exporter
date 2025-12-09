package discovery

import (
	"context"
	"strings"

	"multicloud-exporter/internal/config"

	"github.com/aliyun/alibaba-cloud-sdk-go/services/cms"
)

var newAliyunCMSClient = func(region, ak, sk string) (*cms.Client, error) { return cms.NewClientWithAccessKey(region, ak, sk) }

type AliyunDiscoverer struct{}

func (d *AliyunDiscoverer) Discover(ctx context.Context, cfg *config.Config) []config.Product {
	if cfg == nil {
		return nil
	}
	var accounts []config.CloudAccount
	accounts = append(accounts, cfg.AccountsList...)
	if cfg.AccountsByProvider != nil {
		if xs, ok := cfg.AccountsByProvider["aliyun"]; ok {
			accounts = append(accounts, xs...)
		}
	}
	if cfg.AccountsByProviderLegacy != nil {
		if xs, ok := cfg.AccountsByProviderLegacy["aliyun"]; ok {
			accounts = append(accounts, xs...)
		}
	}
	if len(accounts) == 0 {
		return nil
	}
	nsSet := make(map[string]struct{})
	for _, acc := range accounts {
		for _, r := range acc.Resources {
			switch r {
			case "ecs":
				nsSet["acs_ecs_dashboard"] = struct{}{}
			case "bwp":
				nsSet["acs_bandwidth_package"] = struct{}{}
			case "lb":
				nsSet["acs_slb_dashboard"] = struct{}{}
			case "*":
				nsSet["acs_ecs_dashboard"] = struct{}{}
				nsSet["acs_bandwidth_package"] = struct{}{}
				nsSet["acs_slb_dashboard"] = struct{}{}
			}
		}
	}
	ak := ""
	sk := ""
	if cfg.Credential != nil {
		ak = cfg.Credential.AccessKey
		sk = cfg.Credential.AccessSecret
	}
	prods := make([]config.Product, 0)
	for ns := range nsSet {
		region := "cn-hangzhou"
		if len(accounts) > 0 && len(accounts[0].Regions) > 0 && accounts[0].Regions[0] != "*" {
			region = accounts[0].Regions[0]
		}
		targetAK := ak
		targetSK := sk
		if targetAK == "" || targetSK == "" {
			targetAK = accounts[0].AccessKeyID
			targetSK = accounts[0].AccessKeySecret
		}
		client, err := newAliyunCMSClient(region, targetAK, targetSK)
		if err != nil {
			continue
		}
		req := cms.CreateDescribeMetricMetaListRequest()
		req.Namespace = ns
		resp, err := client.DescribeMetricMetaList(req)
		if err != nil || resp == nil || resp.Resources.Resource == nil {
			continue
		}
		var metrics []string
		for _, r := range resp.Resources.Resource {
			name := strings.TrimSpace(r.MetricName)
			if name == "" {
				continue
			}
			dims := strings.Split(strings.TrimSpace(r.Dimensions), ",")
			has := false
			for _, d := range dims {
				ld := strings.ToLower(strings.TrimSpace(d))
				if ld == "instanceid" || ld == "instance_id" {
					has = true
					break
				}
			}
			if !has {
				continue
			}
			metrics = append(metrics, name)
		}
		if ns == "acs_slb_dashboard" {
			fallback := []string{
				"TrafficRXNew", "TrafficTXNew",
				"DropTrafficRX", "DropTrafficTX",
				"PacketRX", "PacketTX",
				"DropPacketRX", "DropPacketTX",
				"StatusCode2xx", "StatusCode3xx", "StatusCode4xx", "StatusCode5xx", "StatusCodeOther",
				"Qps", "Rt",
				"ActiveConnection", "InactiveConnection", "NewConnection", "MaxConnection",
				// 实例级与 Upstream 指标
				"InstanceQps", "InstanceRt",
				"InstancePacketRX", "InstancePacketTX",
				"InstanceTrafficRXUtilization", "InstanceTrafficTXUtilization",
				"InstanceStatusCode2xx", "InstanceStatusCode3xx", "InstanceStatusCode4xx", "InstanceStatusCode5xx", "InstanceStatusCodeOther",
				"InstanceUpstreamCode4xx", "InstanceUpstreamCode5xx", "InstanceUpstreamRt",
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
		}
		if len(metrics) == 0 {
			continue
		}
		prods = append(prods, config.Product{Namespace: ns, AutoDiscover: true, MetricInfo: []config.MetricGroup{{MetricList: metrics}}})
	}
	return prods
}

func init() {
	Register("aliyun", &AliyunDiscoverer{})
}
