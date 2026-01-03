package tencent

import (
	"time"

	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/logger"
	"multicloud-exporter/internal/metrics"
	providerscommon "multicloud-exporter/internal/providers/common"

	clb "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/clb/v20180317"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	monitor "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/monitor/v20180724"
)

func (t *Collector) listCLBVips(account config.CloudAccount, region string) []string {
	ctxLog := logger.NewContextLogger("Tencent", "account_id", account.AccountID, "region", region, "rtype", "clb")

	if ids, hit := t.getCachedIDs(account, region, "QCE/LB", "clb"); hit {
		ctxLog.Debugf("CLB VIPs 缓存命中，数量=%d", len(ids))
		return ids
	}

	client, err := t.clientFactory.NewCLBClient(region, account.AccessKeyID, account.AccessKeySecret)
	if err != nil {
		return []string{}
	}

	ctxLog.Debugf("开始枚举 CLB VIPs")

	var vips []string
	limit := int64(100) // 腾讯云 CLB API 默认单次最多返回 100 条
	offset := int64(0)

	for {
		req := clb.NewDescribeLoadBalancersRequest()
		req.Limit = common.Int64Ptr(limit)
		req.Offset = common.Int64Ptr(offset)

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
			status := providerscommon.ClassifyTencentError(callErr)
			metrics.RequestTotal.WithLabelValues("tencent", "DescribeLoadBalancers", status).Inc()
			metrics.RecordRequest("tencent", "DescribeLoadBalancers", status)
			if status == "limit_error" {
				// 记录限流指标
				metrics.RateLimitTotal.WithLabelValues("tencent", "DescribeLoadBalancers").Inc()
			}
			if status == "auth_error" {
				return []string{}
			}
			// 指数退避重试
			sleep := time.Duration(200*(1<<attempt)) * time.Millisecond
			if sleep > 5*time.Second {
				sleep = 5 * time.Second
			}
			time.Sleep(sleep)
		}
		if callErr != nil {
			ctxLog.Warnf("CLB DescribeLoadBalancers 失败 offset=%d: %v", offset, callErr)
			break
		}

		if resp == nil || resp.Response == nil || resp.Response.LoadBalancerSet == nil {
			break
		}

		currentCount := uint64(len(resp.Response.LoadBalancerSet))
		if currentCount == 0 {
			break
		}

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

		// 使用 TotalCount 和当前已获取的数量来判断是否还有更多数据
		// 如果返回的数据量小于 limit，说明已经是最后一页
		// 如果返回的数据量等于 limit，需要检查是否还有更多页
		if resp.Response.TotalCount != nil && *resp.Response.TotalCount > 0 {
			totalCollected := uint64(len(vips))
			if totalCollected >= *resp.Response.TotalCount {
				// 已收集的数量达到总数，停止分页
				ctxLog.Debugf("CLB 分页采集完成 offset=%d current_count=%d total_collected=%d total_count=%d",
					offset, currentCount, totalCollected, *resp.Response.TotalCount)
				break
			}
		}

		if int64(currentCount) < limit {
			// 当前页数据量小于 limit，说明已经是最后一页
			break
		}

		// 继续下一页
		offset += limit
		ctxLog.Debugf("CLB 分页采集 offset=%d current_count=%d total_collected=%d", offset, currentCount, len(vips))
		time.Sleep(50 * time.Millisecond)
	}

	t.setCachedIDs(account, region, "QCE/LB", "clb", vips)

	// 更新区域状态
	if t.regionManager != nil {
		status := providerscommon.RegionStatusEmpty
		if len(vips) > 0 {
			status = providerscommon.RegionStatusActive
		}
		t.regionManager.UpdateRegionStatus(account.AccountID, region, len(vips), status)
		ctxLog.Debugf("更新区域状态 account=%s region=%s status=%s count=%d",
			account.AccountID, region, status, len(vips))
	}

	if len(vips) > 0 {
		max := 5
		if len(vips) < max {
			max = len(vips)
		}
		preview := vips[:max]
		ctxLog := logger.NewContextLogger("Tencent", "account_id", account.AccountID, "region", region, "resource_type", "CLB")
		ctxLog.Debugf("CLB VIPs 已枚举，数量=%d 预览=%v", len(vips), preview)
	} else {
		ctxLog := logger.NewContextLogger("Tencent", "account_id", account.AccountID, "region", region, "resource_type", "CLB")
		ctxLog.Debugf("CLB VIPs 已枚举，数量=%d", len(vips))
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
				fallback := int64(60)
				if server := t.cfg.GetServer(); server != nil && server.PeriodFallback > 0 {
					fallback = int64(server.PeriodFallback)
				}
				per = minPeriodForMetric(region, account, prod.Namespace, m, fallback)
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
				status := providerscommon.ClassifyTencentError(err)
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
					ctxLog := logger.NewContextLogger("Tencent", "account_id", account.AccountID, "region", region, "resource_type", "CLB")
					ctxLog.Debugf("CLB指标映射: 命名空间=%s 原始=%s 别名=%s 最终名称=%s_%s", prod.Namespace, m, metricAlias, rtype, metricAlias)
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
