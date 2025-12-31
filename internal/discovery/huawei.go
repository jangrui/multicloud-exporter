// 华为云产品发现：自动发现 ELB 和 OBS 的可用指标
package discovery

import (
	"context"
	"strings"

	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/logger"

	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/auth/basic"
	ces "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ces/v1"
	cesmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ces/v1/model"
	cesregion "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ces/v1/region"
)

// CESDiscoveryClient 定义 CES 发现客户端接口
type CESDiscoveryClient interface {
	ListMetrics(request *cesmodel.ListMetricsRequest) (*cesmodel.ListMetricsResponse, error)
}

var newHuaweiCESClient = func(region, ak, sk string) (CESDiscoveryClient, error) {
	auth, err := basic.NewCredentialsBuilder().
		WithAk(ak).
		WithSk(sk).
		SafeBuild()
	if err != nil {
		return nil, err
	}

	reg, err := cesregion.SafeValueOf(region)
	if err != nil {
		return nil, err
	}

	hcClient, err := ces.CesClientBuilder().
		WithRegion(reg).
		WithCredential(auth).
		SafeBuild()
	if err != nil {
		return nil, err
	}

	return ces.NewCesClient(hcClient), nil
}

// HuaweiDiscoverer 华为云产品发现器
type HuaweiDiscoverer struct{}

// Discover 发现华为云产品和指标
func (d *HuaweiDiscoverer) Discover(ctx context.Context, cfg *config.Config) []config.Product {
	if cfg == nil {
		return nil
	}
	var accounts []config.CloudAccount
	if cfg.AccountsByProvider != nil {
		if xs, ok := cfg.AccountsByProvider["huawei"]; ok {
			accounts = append(accounts, xs...)
		}
	}
	ctxLog := logger.NewContextLogger("Huawei", "resource_type", "Discovery")
	ctxLog.Debugf("发现服务开始，账号数量=%d", len(accounts))
	if len(accounts) == 0 {
		return nil
	}

	needELB := false
	needOBS := false
	for _, acc := range accounts {
		for _, r := range acc.Resources {
			rr := strings.ToLower(r)
			if rr == "clb" || rr == "elb" || rr == "*" {
				needELB = true
			}
			if rr == "s3" || rr == "obs" || rr == "*" {
				needOBS = true
			}
		}
	}

	prods := make([]config.Product, 0)

	if needELB {
		region := "cn-north-4"
		if len(accounts) > 0 && len(accounts[0].Regions) > 0 && accounts[0].Regions[0] != "*" {
			region = accounts[0].Regions[0]
		}
		ak := accounts[0].AccessKeyID
		sk := accounts[0].AccessKeySecret

		// ELB 兜底指标
		fallback := []string{
			"m1_cps", "m2_act_conn", "m3_inact_conn", "m4_ncps",
			"m5_in_pps", "m6_out_pps", "m7_in_Bps", "m8_out_Bps",
			"m9_abnormal_servers", "ma_normal_servers",
			"mb_l7_qps", "mc_l7_http_2xx", "md_l7_http_3xx",
			"me_l7_http_4xx", "mf_l7_http_5xx",
			"m14_l7_rt", "m15_l7_upstream_4xx", "m16_l7_upstream_5xx",
			"m17_l7_upstream_rt", "m21_client_rps",
			"m22_in_bandwidth", "m23_out_bandwidth",
		}

		var metrics []string
		client, err := newHuaweiCESClient(region, ak, sk)
		if err != nil {
			ctxLog := logger.NewContextLogger("Huawei", "resource_type", "Discovery", "namespace", "SYS.ELB")
			ctxLog.Warnf("CES 客户端创建失败，错误=%v", err)
		} else {
			ns := "SYS.ELB"
			req := &cesmodel.ListMetricsRequest{
				Namespace: &ns,
			}
			resp, err := client.ListMetrics(req)
			if err != nil {
				ctxLog := logger.NewContextLogger("Huawei", "resource_type", "Discovery", "namespace", "SYS.ELB")
				ctxLog.Warnf("ListMetrics API调用错误，错误=%v", err)
			}
			if resp != nil && resp.Metrics != nil {
				for _, m := range *resp.Metrics {
					if m.MetricName != "" {
						metrics = append(metrics, m.MetricName)
					}
				}
			}
		}

		// 合并兜底指标
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
			prods = append(prods, config.Product{Namespace: "SYS.ELB", AutoDiscover: true, MetricInfo: []config.MetricGroup{{MetricList: metrics}}})
			ctxLog := logger.NewContextLogger("Huawei", "resource_type", "Discovery", "namespace", "SYS.ELB")
			ctxLog.Infof("发现服务完成，指标数量=%d", len(metrics))
		} else {
			ctxLog := logger.NewContextLogger("Huawei", "resource_type", "Discovery", "namespace", "SYS.ELB")
			ctxLog.Warnf("发现服务未发现指标")
		}
	}

	if needOBS {
		region := "cn-north-4"
		if len(accounts) > 0 && len(accounts[0].Regions) > 0 && accounts[0].Regions[0] != "*" {
			region = accounts[0].Regions[0]
		}
		ak := accounts[0].AccessKeyID
		sk := accounts[0].AccessKeySecret

		// OBS 容量类指标（每日更新，需要 Period=86400）
		capacityFallback := []string{
			"capacity_total", "capacity_standard", "capacity_infrequent_access",
			"capacity_archive", "capacity_deep_archive",
			"object_num_all", "object_num_standard_total",
		}

		// OBS 请求类指标（实时更新，使用 Period=300）
		requestFallback := []string{
			"get_request_count", "put_request_count", "head_request_count",
			"download_bytes", "upload_bytes",
			"download_bytes_extranet", "upload_bytes_extranet",
			"download_bytes_intranet", "upload_bytes_intranet",
			"first_byte_latency", "total_request_latency",
			"request_success_rate", "request_count_4xx",
			"request_count_monitor_2XX", "request_count_monitor_3XX",
			"request_count_monitor_4XX",
		}

		var capacityMetrics, requestMetrics []string
		client, err := newHuaweiCESClient(region, ak, sk)
		if err != nil {
			ctxLog := logger.NewContextLogger("Huawei", "resource_type", "Discovery", "namespace", "SYS.OBS")
			ctxLog.Warnf("CES 客户端创建失败，错误=%v", err)
			// 使用兜底指标
			capacityMetrics = capacityFallback
			requestMetrics = requestFallback
		} else {
			ns := "SYS.OBS"
			req := &cesmodel.ListMetricsRequest{
				Namespace: &ns,
			}
			resp, err := client.ListMetrics(req)
			if err != nil {
				ctxLog := logger.NewContextLogger("Huawei", "resource_type", "Discovery", "namespace", "SYS.OBS")
				ctxLog.Warnf("ListMetrics API调用错误，错误=%v", err)
			}
			if resp != nil && resp.Metrics != nil {
				for _, m := range *resp.Metrics {
					if m.MetricName == "" {
						continue
					}
					// 根据指标名称分类
					if strings.HasPrefix(m.MetricName, "capacity_") || strings.HasPrefix(m.MetricName, "object_num_") {
						capacityMetrics = append(capacityMetrics, m.MetricName)
					} else {
						requestMetrics = append(requestMetrics, m.MetricName)
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
			prods = append(prods, config.Product{Namespace: "SYS.OBS", AutoDiscover: true, MetricInfo: metricGroups})
			ctxLog := logger.NewContextLogger("Huawei", "resource_type", "Discovery", "namespace", "SYS.OBS")
			ctxLog.Infof("发现服务完成，容量类指标=%d 请求类指标=%d", len(capacityMetrics), len(requestMetrics))
		} else {
			ctxLog := logger.NewContextLogger("Huawei", "resource_type", "Discovery", "namespace", "SYS.OBS")
			ctxLog.Warnf("发现服务未发现指标")
		}
	}

	return prods
}

func init() {
	Register("huawei", &HuaweiDiscoverer{})
}
