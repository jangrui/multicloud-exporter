package tencent

import (
	"time"

	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/logger"
	"multicloud-exporter/internal/metrics"

	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	monitor "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/monitor/v20180724"
	vpc "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/vpc/v20170312"
)

func (t *Collector) listBWPIDs(account config.CloudAccount, region string) []string {
	if ids, hit := t.getCachedIDs(account, region, "QCE/BWP", "bwp"); hit {
		logger.Log.Debugf("Tencent BWP IDs cache hit account_id=%s region=%s count=%d", account.AccountID, region, len(ids))
		return ids
	}

	credential := common.NewCredential(account.AccessKeyID, account.AccessKeySecret)
	client, err := vpc.NewClient(credential, region, profile.NewClientProfile())
	if err != nil {
		return []string{}
	}
	req := vpc.NewDescribeBandwidthPackagesRequest()
	start := time.Now()
	resp, err := client.DescribeBandwidthPackages(req)
	if err != nil {
		status := classifyTencentError(err)
		metrics.RequestTotal.WithLabelValues("tencent", "DescribeBandwidthPackages", status).Inc()
		return []string{}
	}
	metrics.RequestTotal.WithLabelValues("tencent", "DescribeBandwidthPackages", "success").Inc()
	metrics.RequestDuration.WithLabelValues("tencent", "DescribeBandwidthPackages").Observe(time.Since(start).Seconds())

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
	logger.Log.Debugf("Tencent BWP enumerated account_id=%s region=%s count=%d", account.AccountID, region, len(ids))
	return ids
}

func (t *Collector) fetchBWPMonitor(account config.CloudAccount, region string, prod config.Product, ids []string) {
	credential := common.NewCredential(account.AccessKeyID, account.AccessKeySecret)
	client, err := monitor.NewClient(credential, region, profile.NewClientProfile())
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
				if status == "limit_error" {
					metrics.RateLimitTotal.WithLabelValues("tencent", "GetMonitorData").Inc()
				}
				continue
			}
			metrics.RequestTotal.WithLabelValues("tencent", "GetMonitorData", "success").Inc()
			metrics.RequestDuration.WithLabelValues("tencent", "GetMonitorData").Observe(time.Since(reqStart).Seconds())

			if resp == nil || resp.Response == nil || resp.Response.DataPoints == nil {
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
				val := float64(0)
				if v := dp.Values[len(dp.Values)-1]; v != nil {
					val = *v
				}
				alias := metrics.NamespaceGauge("QCE/BWP", m)
				scaled := scaleBWPMetric(m, val)
				alias.WithLabelValues("tencent", account.AccountID, region, "bwp", rid, "QCE/BWP", m, "").Set(scaled)
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
