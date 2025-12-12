package tencent

import (
	"time"

	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/logger"
	"multicloud-exporter/internal/metrics"

	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	monitor "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/monitor/v20180724"
)

func (t *Collector) listGWLBIDs(account config.CloudAccount, region string) []string {
	if ids, hit := t.getCachedIDs(account, region, "qce/gwlb", "gwlb"); hit {
		logger.Log.Infof("Tencent GWLB IDs cache hit account_id=%s region=%s count=%d", account.AccountID, region, len(ids))
		return ids
	}
	client, err := t.clientFactory.NewMonitorClient(region, account.AccessKeyID, account.AccessKeySecret)
	if err != nil {
		return []string{}
	}
	req := monitor.NewGetMonitorDataRequest()
	req.Namespace = common.StringPtr("qce/gwlb")
	req.MetricName = common.StringPtr("ConcurConn")
	period := int64(60)
	req.Period = common.Uint64Ptr(uint64(period))
	start := time.Now().Add(-time.Duration(period) * time.Second)
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
		return []string{}
	}
	metrics.RequestTotal.WithLabelValues("tencent", "GetMonitorData", "success").Inc()
	metrics.RequestDuration.WithLabelValues("tencent", "GetMonitorData").Observe(time.Since(reqStart).Seconds())

	var ids []string
	seen := make(map[string]struct{})
	if resp != nil && resp.Response != nil && resp.Response.DataPoints != nil {
		for _, dp := range resp.Response.DataPoints {
			if dp == nil || len(dp.Dimensions) == 0 {
				continue
			}
			rid := extractDimension(dp.Dimensions, "gwLoadBalancerId")
			if rid == "" {
				continue
			}
			if _, ok := seen[rid]; !ok {
				seen[rid] = struct{}{}
				ids = append(ids, rid)
			}
		}
	}
	t.setCachedIDs(account, region, "qce/gwlb", "gwlb", ids)
	if len(ids) > 0 {
		max := 5
		if len(ids) < max {
			max = len(ids)
		}
		preview := ids[:max]
		logger.Log.Infof("Tencent GWLB enumerated account_id=%s region=%s count=%d preview=%v", account.AccountID, region, len(ids), preview)
	} else {
		logger.Log.Infof("Tencent GWLB enumerated account_id=%s region=%s count=%d", account.AccountID, region, len(ids))
	}
	return ids
}

func (t *Collector) fetchGWLBMonitor(account config.CloudAccount, region string, prod config.Product, ids []string) {
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
			req.Namespace = common.StringPtr("qce/gwlb")
			req.MetricName = common.StringPtr(m)
			per := period
			if prod.Period == nil && group.Period == nil {
				per = minPeriodForMetric(region, account, "qce/gwlb", m)
			}
			req.Period = common.Uint64Ptr(uint64(per))
			var inst []*monitor.Instance
			for _, id := range ids {
				inst = append(inst, &monitor.Instance{
					Dimensions: []*monitor.Dimension{
						{Name: common.StringPtr("gwLoadBalancerId"), Value: common.StringPtr(id)},
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
				alias, count := metrics.NamespaceGauge("qce/gwlb", m)
				for _, id := range ids {
					labels := []string{"tencent", account.AccountID, region, "gwlb", id, "qce/gwlb", m, ""}
					for len(labels) < count {
						labels = append(labels, "")
					}
					alias.WithLabelValues(labels...).Set(0)
					metrics.IncSampleCount("qce/gwlb", 1)
				}
				continue
			}
			for _, dp := range resp.Response.DataPoints {
				if dp == nil || len(dp.Dimensions) == 0 || len(dp.Values) == 0 {
					continue
				}
				rid := extractDimension(dp.Dimensions, "gwLoadBalancerId")
				if rid == "" {
					continue
				}
				val := float64(0)
				if v := dp.Values[len(dp.Values)-1]; v != nil {
					val = *v
				}
				alias, count := metrics.NamespaceGauge("qce/gwlb", m)
				scaled := metrics.GetMetricScale("qce/gwlb", m)
				if scaled != 0 && scaled != 1 {
					val = val * scaled
				}
				labels := []string{"tencent", account.AccountID, region, "gwlb", rid, "qce/gwlb", m, ""}
				for len(labels) < count {
					labels = append(labels, "")
				}
				alias.WithLabelValues(labels...).Set(val)
				metrics.IncSampleCount("qce/gwlb", 1)
			}
		}
	}
}

func (t *Collector) collectGWLB(account config.CloudAccount, region string) {
	if t.cfg == nil {
		return
	}
	var prods []config.Product
	if t.disc != nil {
		if ps, ok := t.disc.Get()["tencent"]; ok && len(ps) > 0 {
			prods = ps
		}
	}
	if len(prods) == 0 {
		return
	}
	for _, p := range prods {
		if p.Namespace != "qce/gwlb" {
			continue
		}
		ids := t.listGWLBIDs(account, region)
		if len(ids) == 0 {
			return
		}
		t.fetchGWLBMonitor(account, region, p, ids)
	}
}
