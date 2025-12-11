package discovery

import (
	"context"
	"strings"

	"multicloud-exporter/internal/config"

	"github.com/aliyun/alibaba-cloud-sdk-go/services/cms"
)

type CMSClient interface {
	DescribeMetricMetaList(request *cms.DescribeMetricMetaListRequest) (response *cms.DescribeMetricMetaListResponse, err error)
}

var newAliyunCMSClient = func(region, ak, sk string) (CMSClient, error) {
	return cms.NewClientWithAccessKey(region, ak, sk)
}

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
			case "bwp", "cbwp":
				nsSet["acs_bandwidth_package"] = struct{}{}
			case "lb", "slb":
				nsSet["acs_slb_dashboard"] = struct{}{}
			case "oss":
				nsSet["acs_oss_dashboard"] = struct{}{}
			case "*":
				nsSet["acs_bandwidth_package"] = struct{}{}
				nsSet["acs_slb_dashboard"] = struct{}{}
				nsSet["acs_oss_dashboard"] = struct{}{}
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
        mapping := config.DefaultResourceDimMapping()
        key := "aliyun." + ns
        required := mapping[key]
        for _, r := range resp.Resources.Resource {
            name := strings.TrimSpace(r.MetricName)
            if name == "" {
                continue
            }
            dims := strings.Split(strings.TrimSpace(r.Dimensions), ",")
            has := false
            if len(required) > 0 {
                lower := make(map[string]struct{}, len(dims))
                for _, d := range dims {
                    ld := strings.ToLower(strings.TrimSpace(d))
                    if ld != "" {
                        lower[ld] = struct{}{}
                    }
                }
                for _, k := range required {
                    if _, ok := lower[strings.ToLower(k)]; ok {
                        has = true
                        break
                    }
                }
            } else {
                for _, d := range dims {
                    ld := strings.ToLower(strings.TrimSpace(d))
                    if ld == "instanceid" || ld == "instance_id" {
                        has = true
                        break
                    }
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
				"UnhealthyServerCount", "HealthyServerCountWithRule",
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

func FetchAliyunMetricMeta(region, ak, sk, namespace string) ([]MetricMeta, error) {
    client, err := newAliyunCMSClient(region, ak, sk)
    if err != nil {
        return nil, err
    }
    req := cms.CreateDescribeMetricMetaListRequest()
    req.Namespace = namespace
    resp, err := client.DescribeMetricMetaList(req)
    if err != nil || resp == nil || resp.Resources.Resource == nil {
        return nil, err
    }
    var out []MetricMeta
    for _, r := range resp.Resources.Resource {
        name := strings.TrimSpace(r.MetricName)
        if name == "" {
            continue
        }
        dims := strings.Split(strings.TrimSpace(r.Dimensions), ",")
        var ndims []string
        for _, d := range dims {
            ld := strings.TrimSpace(d)
            if ld != "" {
                ndims = append(ndims, ld)
            }
        }
        mm := MetricMeta{
            Provider:   "aliyun",
            Namespace:  namespace,
            Name:       name,
            Unit:       strings.TrimSpace(r.Unit),
            Dimensions: ndims,
            Description: strings.TrimSpace(r.Description),
        }
        out = append(out, mm)
    }
    return out, nil
}
