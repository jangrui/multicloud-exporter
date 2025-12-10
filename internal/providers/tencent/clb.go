package tencent

import (
	"time"

	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/logger"
	"multicloud-exporter/internal/metrics"

	clb "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/clb/v20180317"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	monitor "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/monitor/v20180724"
)

func (t *Collector) listCLBVips(account config.CloudAccount, region string) []string {
	if ids, hit := t.getCachedIDs(account, region, "QCE/LB", "lb"); hit {
		logger.Log.Debugf("Tencent CLB VIPs cache hit account_id=%s region=%s count=%d", account.AccountID, region, len(ids))
		return ids
	}

	client, err := t.clientFactory.NewCLBClient(region, account.AccessKeyID, account.AccessKeySecret)
	if err != nil {
		return []string{}
	}
	req := clb.NewDescribeLoadBalancersRequest()
	start := time.Now()
	resp, err := client.DescribeLoadBalancers(req)
	if err != nil {
		status := classifyTencentError(err)
		metrics.RequestTotal.WithLabelValues("tencent", "DescribeLoadBalancers", status).Inc()
		return []string{}
	}
	metrics.RequestTotal.WithLabelValues("tencent", "DescribeLoadBalancers", "success").Inc()
	metrics.RequestDuration.WithLabelValues("tencent", "DescribeLoadBalancers").Observe(time.Since(start).Seconds())

	if resp == nil || resp.Response == nil || resp.Response.LoadBalancerSet == nil {
		return []string{}
	}
	var vips []string
	for _, lb := range resp.Response.LoadBalancerSet {
		if lb == nil || lb.LoadBalancerVips == nil {
			continue
		}
		for _, vip := range lb.LoadBalancerVips {
			if vip != nil {
				vips = append(vips, *vip)
			}
		}
	}
	t.setCachedIDs(account, region, "QCE/LB", "lb", vips)
	logger.Log.Debugf("Tencent CLB VIPs enumerated account_id=%s region=%s count=%d", account.AccountID, region, len(vips))
	return vips
}

func (t *Collector) fetchCLBMonitor(account config.CloudAccount, region string, prod config.Product, vips []string) {
	client, err := t.clientFactory.NewMonitorClient(region, account.AccessKeyID, account.AccessKeySecret)
	if err != nil {
		return
	}
	period := int64(60)
	if prod.Period != nil {
		period = int64(*prod.Period)
	}
	for _, group := range prod.MetricInfo {
		if group.Period != nil {
			period = int64(*group.Period)
		}
		for _, m := range group.MetricList {
			req := monitor.NewGetMonitorDataRequest()
			req.Namespace = common.StringPtr("QCE/LB")
			req.MetricName = common.StringPtr(m)
			per := period
			if prod.Period == nil && group.Period == nil {
				per = minPeriodForMetric(region, account, "QCE/LB", m)
			}
			req.Period = common.Uint64Ptr(uint64(per))
			var inst []*monitor.Instance
			for _, vip := range vips {
				inst = append(inst, &monitor.Instance{
					Dimensions: []*monitor.Dimension{
						{Name: common.StringPtr("vip"), Value: common.StringPtr(vip)},
					},
				})
			}
			req.Instances = inst
			start := time.Now().Add(-time.Duration(per) * time.Second)
			end := time.Now()
			req.StartTime = common.StringPtr(start.UTC().Format("2006-01-02T15:04:05Z"))
			req.EndTime = common.StringPtr(end.UTC().Format("2006-01-02T15:04:05Z"))

			reqStart := time.Now()
			resp, err := client.GetMonitorData(req)
			if err != nil {
				status := classifyTencentError(err)
				metrics.RequestTotal.WithLabelValues("tencent", "GetMonitorData", status).Inc()
				if status == "limit_error" {
					metrics.RateLimitTotal.WithLabelValues("tencent", "GetMonitorData").Inc()
				}
				continue
			}
			metrics.RequestTotal.WithLabelValues("tencent", "GetMonitorData", "success").Inc()
			metrics.RequestDuration.WithLabelValues("tencent", "GetMonitorData").Observe(time.Since(reqStart).Seconds())

			if resp == nil || resp.Response == nil || resp.Response.DataPoints == nil || len(resp.Response.DataPoints) == 0 {
				// 输出 0 值样本以保证指标可见性
				alias, count := metrics.NamespaceGauge("QCE/CLB", m)
				for _, vip := range vips {
					labels := []string{"tencent", account.AccountID, region, "lb", vip, "QCE/CLB", m, ""}
					for len(labels) < count {
						labels = append(labels, "")
					}
					alias.WithLabelValues(labels...).Set(0)
				}
				continue
			}
			for _, dp := range resp.Response.DataPoints {
				if dp == nil || len(dp.Dimensions) == 0 || len(dp.Values) == 0 {
					continue
				}
				rid := extractDimension(dp.Dimensions, "vip")
				if rid == "" {
					continue
				}
				val := float64(0)
				if v := dp.Values[len(dp.Values)-1]; v != nil {
					val = *v
				}
				alias, count := metrics.NamespaceGauge("QCE/CLB", m)
				scaled := scaleCLBMetric(m, val)
				labels := []string{"tencent", account.AccountID, region, "lb", rid, "QCE/CLB", m, ""}
				for len(labels) < count {
					labels = append(labels, "")
				}
				alias.WithLabelValues(labels...).Set(scaled)
			}
		}
	}
}

func extractDimension(dims []*monitor.Dimension, target string) string {
	for _, d := range dims {
		if d != nil && d.Name != nil && d.Value != nil && *d.Name == target {
			return *d.Value
		}
	}
	return ""
}

func scaleCLBMetric(metric string, val float64) float64 {
	if s := metrics.GetMetricScale("QCE/CLB", metric); s != 0 && s != 1 {
		return val * s
	}
	if metric == "VipIntraffic" || metric == "VipOuttraffic" {
		return val * 1000000
	}
	return val
}
