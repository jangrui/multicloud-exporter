package discovery

import (
	"context"
	"strings"

	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/logger"

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
	if cfg.AccountsByProvider != nil {
		if xs, ok := cfg.AccountsByProvider["aliyun"]; ok {
			accounts = append(accounts, xs...)
		}
	}
	if len(accounts) == 0 {
		return nil
	}
	nsSet := make(map[string]struct{})
	for _, acc := range accounts {
		for _, r := range acc.Resources {
			rr := strings.ToLower(r)
			switch rr {
			case "bwp":
				nsSet["acs_bandwidth_package"] = struct{}{}
			case "clb":
				nsSet["acs_slb_dashboard"] = struct{}{}
			case "alb":
				nsSet["acs_alb"] = struct{}{}
			case "nlb":
				nsSet["acs_nlb"] = struct{}{}
			case "gwlb":
				nsSet["acs_gwlb"] = struct{}{}
			case "s3":
				nsSet["acs_oss_dashboard"] = struct{}{}
			case "*":
				nsSet["acs_bandwidth_package"] = struct{}{}
				nsSet["acs_slb_dashboard"] = struct{}{}
				nsSet["acs_oss_dashboard"] = struct{}{}
				nsSet["acs_alb"] = struct{}{}
				nsSet["acs_nlb"] = struct{}{}
				nsSet["acs_gwlb"] = struct{}{}
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

	// Fetch meta for each namespace
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

		// Pre-defined fallback metrics for each namespace
		// These are used when:
		// 1. Meta API fails (network error, permission denied, etc.)
		// 2. Meta API returns empty list
		// 3. To ensure core metrics are always collected even if API doesn't return them
		fallbackMap := map[string][]string{
			"acs_bandwidth_package": {
				"net_rx.rate", "net_tx.rate",
				"net_rx.Pkgs", "net_tx.Pkgs",
				"in_bandwidth_utilization", "out_bandwidth_utilization",
				"in_ratelimit_drop_pps", "out_ratelimit_drop_pps",
				"net_rx.ratePercent", "net_tx.ratePercent",
			},
			"acs_slb_dashboard": {
				"InstanceTrafficRX", "InstanceTrafficTX",
				"InstancePacketRX", "InstancePacketTX",
				"InstanceDropPacketRX", "InstanceDropPacketTX",
				"InstanceDropTrafficRX", "InstanceDropTrafficTX",
				"NewConnection", "ActiveConnection", "DropConnection",
				"Qps", "Rt",
				"StatusCode2xx", "StatusCode3xx", "StatusCode4xx", "StatusCode5xx", "StatusCodeOther",
				"UnhealthyServerCount", "HealthyServerCountWithRule",
				"InstanceQps", "InstanceRt",
				"InstanceTrafficRXUtilization", "InstanceTrafficTXUtilization",
				"InstanceStatusCode2xx", "InstanceStatusCode3xx",
				"InstanceStatusCode4xx", "InstanceStatusCode5xx", "InstanceStatusCodeOther",
				"InstanceUpstreamCode4xx", "InstanceUpstreamCode5xx", "InstanceUpstreamRt",
				"InactiveConnection", "MaxConnection",
				"GroupActiveConnection", "GroupNewConnection",
				"GroupTotalTrafficRX", "GroupTotalTrafficTX",
				"GroupTrafficRX", "GroupTrafficTX",
				"GroupUnhealthyServerCount", "HeathyServerCount",
				"InstanceActiveConnection", "InstanceDropConnection",
				"InstanceInactiveConnection", "InstanceMaxConnection",
				"InstanceMaxConnectionUtilization", "InstanceNewConnection",
				"InstanceNewConnectionUtilization", "InstanceQpsUtilization",
				"UnhealthyServerCountWithRule",
				"UpstreamCode4xx", "UpstreamCode5xx", "UpstreamRt",
			},
			"acs_oss_dashboard": {
				"UserStorage",
				"InternetRecv", "InternetSend",
				"IntranetRecv", "IntranetSend",
				"UserCdnRecv", "UserCdnSend",
				"TotalRequestCount",
				"GetObjectCount", "PutObjectCount", "HeadObjectCount",
				"AppendObjectCount", "DeleteObjectCount", "CopyObjectCount",
				"UserAvailability",
				"ServerErrorCount",
				"AuthorizationErrorCount", "AuthorizationErrorRate",
				"ClientTimeoutErrorCount", "ClientTimeoutErrorRate",
				"GetObjectE2eLatency", "GetObjectServerLatency",
				"PutObjectE2eLatency", "PutObjectServerLatency",
				"AppendObjectE2eLatency", "AppendObjectServerLatency",
				"CopyObjectE2eLatency", "CopyObjectServerLatency",
				"InternetRecvBandwidth", "InternetSendBandwidth",
				"IntranetRecvBandwidth", "IntranetSendBandwidth",
				"CacheRecv", "CacheSend",
				"CacheOriginRecv", "CacheOriginSend",
				"ObjectCount",
				"MeteringSyncRX", "MeteringSyncTX",
			},
			"acs_alb": {
				"LoadBalancerActiveConnection", "LoadBalancerNewConnection", "LoadBalancerRejectedConnection",
				"LoadBalancerInBits", "LoadBalancerOutBits",
				"LoadBalancerQPS", "LoadBalancerRequestTime",
				"LoadBalancerHTTPCode2XX", "LoadBalancerHTTPCode3XX",
				"LoadBalancerHTTPCode4XX", "LoadBalancerHTTPCode5XX",
				"ListenerActiveConnection", "ListenerClientTLSNegotiationError",
				"ListenerHTTPCode2XX", "ListenerHTTPCode3XX",
				"ListenerHTTPCode4XX", "ListenerHTTPCode500",
				"ListenerHTTPCode502", "ListenerHTTPCode503",
				"ListenerHTTPCode504", "ListenerHTTPCode5XX",
				"ListenerHTTPCodeUpstream2XX", "ListenerHTTPCodeUpstream3XX",
				"ListenerHTTPCodeUpstream4XX", "ListenerHTTPCodeUpstream5XX",
				"ListenerHTTPFixedResponse", "ListenerHTTPRedirect",
				"ListenerHealthyHostCount", "ListenerInBits",
				"ListenerInactiveConnection", "ListenerMaxConnection",
				"ListenerNewConnection", "ListenerNonStickyRequest",
				"ListenerOutBits", "ListenerQPS", "ListenerRejectedConnection",
				"ListenerRequestTime", "ListenerUnHealthyHostCount",
				"ListenerUpstreamConnectionError", "ListenerUpstreamResponseTime",
				"ListenerUpstreamTLSNegotiationError",
				"VipActiveConnection", "VipClientTLSNegotiationError",
				"VipHTTPCode2XX", "VipHTTPCode3XX",
				"VipHTTPCode4XX", "VipHTTPCode500",
				"VipHTTPCode502", "VipHTTPCode503",
				"VipHTTPCode504", "VipHTTPCode5XX",
				"VipHTTPFixedResponse", "VipHTTPRedirect",
				"VipInBits", "VipInactiveConnection",
				"VipMaxConnection", "VipNewConnection",
				"VipNonStickyRequest", "VipOutBits",
				"VipQPS", "VipRejectedConnection",
				"VipRequestTime", "VipUpstreamConnectionError",
				"VipUpstreamResponseTime", "VipUpstreamTLSNegotiationError",
				"ServerGroupHTTPCodeUpstream2XX", "ServerGroupHTTPCodeUpstream3XX",
				"ServerGroupHTTPCodeUpstream4XX", "ServerGroupHTTPCodeUpstream5XX",
				"ServerGroupHealthyHostCount", "ServerGroupNonStickyRequest",
				"ServerGroupQPS", "ServerGroupRequestTime",
				"ServerGroupUnHealthyHostCount", "ServerGroupUpstreamConnectionError",
				"ServerGroupUpstreamResponseTime", "ServerGroupUpstreamTLSNegotiationError",
				"RuleHTTPCodeUpstream2XX",
			},
			"acs_nlb": {
				"InstanceActiveConnection", "InstanceNewConnection", "DropConnection",
				"InstanceTrafficRX", "InstanceTrafficTX",
				"InstanceDropPacketRX", "InstanceDropPacketTX",
				"InstancePacketRX", "InstancePacketTX",
				"ListenerHeathyServerCount", "ListenerUnhealthyServerCount",
				"ListenerPacketRX", "ListenerPacketTX",
				"DropPacketRX", "DropPacketTX",
				"DropTrafficRX", "DropTrafficTX",
				"InstanceInactiveConnection", "InstanceMaxConnection",
				"InstanceDropConnection",
				"InstanceDropTrafficRX", "InstanceDropTrafficTX",
				"VipActiveConnection", "VipClientResetPacket",
				"VipDropConnection",
				"VipDropPacketRX", "VipDropPacketTX",
				"VipDropTrafficRX", "VipDropTrafficTX",
				"VipPacketRX", "VipPacketTX",
				"VipTrafficRX", "VipTrafficTX",
			},
			"acs_gwlb": {
				"ActiveConnection", "NewConnection",
				"TrafficRX", "TrafficTX",
				"PacketRX", "PacketTX",
				"ServerGroupUnhealthyHostCount", "ServerGroupHealthyHostCount",
			},
		}

		client, err := newAliyunCMSClient(region, targetAK, targetSK)
		if err != nil {
			if list, ok := fallbackMap[ns]; ok {
				logger.Log.Warnf("Aliyun CMS 客户端创建失败，命名空间=%s，使用备用指标，错误=%v", ns, err)
				prods = append(prods, config.Product{Namespace: ns, AutoDiscover: true, MetricInfo: []config.MetricGroup{{MetricList: list}}})
			}
			continue
		}
		req := cms.CreateDescribeMetricMetaListRequest()
		req.Namespace = ns
		resp, err := client.DescribeMetricMetaList(req)
		var metrics []string

		// 1. Handle API Failure: Use fallback directly
		if err != nil || resp == nil || resp.Resources.Resource == nil {
			if list, ok := fallbackMap[ns]; ok {
				logger.Log.Infof("Aliyun 发现服务启用备用指标，命名空间=%s 原因=元数据不可用", ns)
				metrics = list
			} else {
				continue
			}
		} else {
			// 2. Handle API Success: Parse metrics
			mapping := config.DefaultResourceDimMapping()
			key := "aliyun." + ns
			required := mapping[key]

			// 特别记录 BWP 命名空间的调试信息
			if ns == "acs_bandwidth_package" {
				logger.Log.Infof("Aliyun BWP 发现服务，必需维度=%v", required)
			}

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

				// 对于 BWP 命名空间，记录每个指标的维度匹配情况
				if ns == "acs_bandwidth_package" {
					if has {
						logger.Log.Debugf("Aliyun BWP 保留指标=%s 维度=%v", name, dims)
					} else {
						logger.Log.Warnf("Aliyun BWP 过滤指标=%s 维度=%v 必需维度=%v", name, dims, required)
					}
				}

				if !has {
					// 改进：对于 BWP 命名空间，即使维度不完全匹配，也保留指标
					// 原因：某些指标（如 net_tx.rate）可能在某些账号/区域返回的维度与配置不完全一致
					// 但这些指标在实际采集中仍然有效，不应被过滤
					if ns == "acs_bandwidth_package" && len(dims) > 0 {
						logger.Log.Infof("Aliyun BWP 强制保留指标=%s（维度不完全匹配但存在维度），维度=%v", name, dims)
						has = true
					} else {
						logger.Log.Debugf("Aliyun 发现服务过滤指标，命名空间=%s 指标=%s 原因=维度不匹配 必需维度=%v 指标维度=%v", ns, name, required, dims)
						continue
					}
				}
				metrics = append(metrics, name)
			}
			if ns == "acs_bandwidth_package" {
				logger.Log.Infof("Aliyun BWP 发现服务解析完成，原始指标=%d 过滤后指标=%d", len(resp.Resources.Resource), len(metrics))
				// 【诊断日志】列出所有保留的指标
				if len(metrics) > 0 {
					logger.Log.Debugf("Aliyun BWP 保留的指标列表=%v", metrics)
				}
			}
		}

		// 3. Ensure Core Metrics: Append fallback metrics if missing
		if list, ok := fallbackMap[ns]; ok {
			cur := make(map[string]struct{}, len(metrics))
			for _, m := range metrics {
				cur[m] = struct{}{}
			}
			added := 0
			for _, m := range list {
				if _, ok := cur[m]; !ok {
					metrics = append(metrics, m)
					added++
				}
			}
			if added > 0 {
				logger.Log.Infof("Aliyun 发现服务已追加备用指标，命名空间=%s 新增数量=%d 追加指标=%v", ns, added, func() []string {
					missing := []string{}
					for _, m := range list {
						if _, ok := cur[m]; !ok {
							missing = append(missing, m)
						}
					}
					return missing
				}())
			}
		}

		if len(metrics) == 0 {
			logger.Log.Warnf("Aliyun 发现服务未发现指标，命名空间=%s", ns)
			continue
		}
		prods = append(prods, config.Product{Namespace: ns, AutoDiscover: true, MetricInfo: []config.MetricGroup{{MetricList: metrics}}})
		logger.Log.Infof("Aliyun 发现服务完成，命名空间=%s，指标数量=%d", ns, len(metrics))
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
			Provider:    "aliyun",
			Namespace:   namespace,
			Name:        name,
			Unit:        strings.TrimSpace(r.Unit),
			Dimensions:  ndims,
			Description: strings.TrimSpace(r.Description),
		}
		out = append(out, mm)
	}
	return out, nil
}
