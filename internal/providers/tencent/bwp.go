package tencent

import (
	"time"

	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/logger"
	"multicloud-exporter/internal/metrics"

	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	monitor "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/monitor/v20180724"
	vpc "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/vpc/v20170312"
)

func (t *Collector) listBWPIDs(account config.CloudAccount, region string) []string {
	if ids, hit := t.getCachedIDs(account, region, "QCE/BWP", "bwp"); hit {
		logger.Log.Debugf("Tencent BWP IDs 缓存命中，账号ID=%s 区域=%s 数量=%d", account.AccountID, region, len(ids))
		return ids
	}

	client, err := t.clientFactory.NewVPCClient(region, account.AccessKeyID, account.AccessKeySecret)
	if err != nil {
		return []string{}
	}
	req := vpc.NewDescribeBandwidthPackagesRequest()
	start := time.Now()
	var resp *vpc.DescribeBandwidthPackagesResponse
	var callErr error
	for attempt := 0; attempt < 3; attempt++ {
		resp, callErr = client.DescribeBandwidthPackages(req)
		if callErr == nil {
			metrics.RequestTotal.WithLabelValues("tencent", "DescribeBandwidthPackages", "success").Inc()
			metrics.RecordRequest("tencent", "DescribeBandwidthPackages", "success")
			metrics.RequestDuration.WithLabelValues("tencent", "DescribeBandwidthPackages").Observe(time.Since(start).Seconds())
			break
		}
		status := classifyTencentError(callErr)
		metrics.RequestTotal.WithLabelValues("tencent", "DescribeBandwidthPackages", status).Inc()
		metrics.RecordRequest("tencent", "DescribeBandwidthPackages", status)
		if status == "limit_error" {
			// 记录限流指标
			metrics.RateLimitTotal.WithLabelValues("tencent", "DescribeBandwidthPackages").Inc()
		}
		if status == "auth_error" {
			return []string{}
		}
		time.Sleep(time.Duration(200*(attempt+1)) * time.Millisecond)
	}
	if callErr != nil {
		return []string{}
	}

	if resp == nil || resp.Response == nil || resp.Response.BandwidthPackageSet == nil {
		return []string{}
	}
	var ids []string
	for _, bp := range resp.Response.BandwidthPackageSet {
		if bp == nil || bp.BandwidthPackageId == nil {
			continue
		}
		ids = append(ids, *bp.BandwidthPackageId)
	}
	t.setCachedIDs(account, region, "QCE/BWP", "bwp", ids)
	if len(ids) > 0 {
		max := 5
		if len(ids) < max {
			max = len(ids)
		}
		preview := ids[:max]
		logger.Log.Debugf("Tencent BWP 已枚举，账号ID=%s 区域=%s 数量=%d 预览=%v", account.AccountID, region, len(ids), preview)
	} else {
		logger.Log.Debugf("Tencent BWP 已枚举，账号ID=%s 区域=%s 数量=%d", account.AccountID, region, len(ids))
	}
	return ids
}

func (t *Collector) fetchBWPMonitor(account config.CloudAccount, region string, prod config.Product, ids []string) {
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
			req.Namespace = common.StringPtr("QCE/BWP")
			req.MetricName = common.StringPtr(m)
			per := period
			if prod.Period == nil && group.Period == nil {
				per = minPeriodForMetric(region, account, "QCE/BWP", m)
			}
			req.Period = common.Uint64Ptr(uint64(per))
			var inst []*monitor.Instance
			for _, id := range ids {
				inst = append(inst, &monitor.Instance{
					Dimensions: []*monitor.Dimension{
						{Name: common.StringPtr("bandwidthPackageId"), Value: common.StringPtr(id)},
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
				rid := extractDimension(dp.Dimensions, "bandwidthPackageId")
				if rid == "" {
					continue
				}
				// 如果最后一个值为 nil，表示没有数据，跳过指标（而不是设置为 0）
				v := dp.Values[len(dp.Values)-1]
				if v == nil {
					continue
				}
				val := *v
				alias, count := metrics.NamespaceGauge("QCE/BWP", m)
				scaled := scaleBWPMetric(m, val)
				labels := []string{"tencent", account.AccountID, region, "bwp", rid, "QCE/BWP", m, ""}
				for len(labels) < count {
					labels = append(labels, "")
				}
				alias.WithLabelValues(labels...).Set(scaled)
				metrics.IncSampleCount("QCE/BWP", 1)
			}
		}
	}
}

func scaleBWPMetric(metric string, val float64) float64 {
	if s := metrics.GetMetricScale("QCE/BWP", metric); s != 0 && s != 1 {
		return val * s
	}
	if metric == "InTraffic" || metric == "OutTraffic" {
		return val * 1000000
	}
	return val

}
