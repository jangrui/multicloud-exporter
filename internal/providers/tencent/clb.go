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
	if ids, hit := t.getCachedIDs(account, region, "QCE/LB", "clb"); hit {
		logger.Log.Debugf("Tencent CLB VIPs 缓存命中，账号ID=%s 区域=%s 数量=%d", account.AccountID, region, len(ids))
		return ids
	}

	client, err := t.clientFactory.NewCLBClient(region, account.AccessKeyID, account.AccessKeySecret)
	if err != nil {
		return []string{}
	}
	req := clb.NewDescribeLoadBalancersRequest()
	start := time.Now()
	var resp *clb.DescribeLoadBalancersResponse
	var callErr error
	for attempt := 0; attempt < 3; attempt++ {
		resp, callErr = client.DescribeLoadBalancers(req)
		if callErr == nil {
			metrics.RequestTotal.WithLabelValues("tencent", "DescribeLoadBalancers", "success").Inc()
			metrics.RecordRequest("tencent", "DescribeLoadBalancers", "success")
			metrics.RequestDuration.WithLabelValues("tencent", "DescribeLoadBalancers").Observe(time.Since(start).Seconds())
			break
		}
		status := classifyTencentError(callErr)
		metrics.RequestTotal.WithLabelValues("tencent", "DescribeLoadBalancers", status).Inc()
		metrics.RecordRequest("tencent", "DescribeLoadBalancers", status)
		if status == "limit_error" {
			// 记录限流指标
			metrics.RateLimitTotal.WithLabelValues("tencent", "DescribeLoadBalancers").Inc()
		}
		if status == "auth_error" {
			return []string{}
		}
		time.Sleep(time.Duration(200*(attempt+1)) * time.Millisecond)
	}
	if callErr != nil {
		return []string{}
	}

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
	t.setCachedIDs(account, region, "QCE/LB", "clb", vips)
	if len(vips) > 0 {
		max := 5
		if len(vips) < max {
			max = len(vips)
		}
		preview := vips[:max]
		logger.Log.Debugf("Tencent CLB VIPs 已枚举，账号ID=%s 区域=%s 数量=%d 预览=%v", account.AccountID, region, len(vips), preview)
	} else {
		logger.Log.Debugf("Tencent CLB VIPs 已枚举，账号ID=%s 区域=%s 数量=%d", account.AccountID, region, len(vips))
	}
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
			req.Namespace = common.StringPtr(prod.Namespace)
			req.MetricName = common.StringPtr(m)
			per := period
			if prod.Period == nil && group.Period == nil {
				per = minPeriodForMetric(region, account, prod.Namespace, m)
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
				metrics.RecordRequest("tencent", "GetMonitorData", status)
				if status == "limit_error" {
					metrics.RateLimitTotal.WithLabelValues("tencent", "GetMonitorData").Inc()
				}
				continue
			}
			metrics.RequestTotal.WithLabelValues("tencent", "GetMonitorData", "success").Inc()
			metrics.RecordRequest("tencent", "GetMonitorData", "success")
			metrics.RequestDuration.WithLabelValues("tencent", "GetMonitorData").Observe(time.Since(reqStart).Seconds())

			if resp == nil || resp.Response == nil || resp.Response.DataPoints == nil || len(resp.Response.DataPoints) == 0 {
				// 如果没有数据点，不暴露指标（而不是设置 0 值）
				// 根据 Prometheus 最佳实践：不存在资源或无数据时，不应该暴露指标
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
				// 如果最后一个值为 nil，表示没有数据，跳过指标（而不是设置为 0）
				v := dp.Values[len(dp.Values)-1]
				if v == nil {
					continue
				}
				val := *v
				alias, count := metrics.NamespaceGauge(prod.Namespace, m)
				rtype := metrics.GetNamespacePrefix(prod.Namespace)
				if rtype == "" {
					rtype = "clb"
				}
				scaled := scaleCLBMetric(prod.Namespace, m, val)
				// 调试日志：记录指标映射信息
				metricAlias := metrics.GetMetricAlias(prod.Namespace, m)
				if metricAlias != "" {
					logger.Log.Debugf("Tencent CLB 指标映射: 命名空间=%s 原始=%s 别名=%s 最终名称=%s_%s", prod.Namespace, m, metricAlias, rtype, metricAlias)
				}
				labels := []string{"tencent", account.AccountID, region, rtype, rid, prod.Namespace, m, ""}
				for len(labels) < count {
					labels = append(labels, "")
				}
				alias.WithLabelValues(labels...).Set(scaled)
				metrics.IncSampleCount(prod.Namespace, 1)
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

func scaleCLBMetric(namespace, metric string, val float64) float64 {
	if s := metrics.GetMetricScale(namespace, metric); s != 0 && s != 1 {
		return val * s
	}
	// 兼容多种指标名称的流量指标（单位从 Mbps 转换为 bit/s）
	if metric == "VipIntraffic" || metric == "VipOuttraffic" ||
		metric == "ClientIntraffic" || metric == "ClientOuttraffic" ||
		metric == "VIntraffic" || metric == "VOuttraffic" {
		return val * 1000000
	}
	return val
}
