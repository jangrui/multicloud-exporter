package tencent

import (
	"log"
	"time"

	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/metrics"

	clb "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/clb/v20180317"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	monitor "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/monitor/v20180724"
)

func (t *Collector) listCLBVips(account config.CloudAccount, region string) []string {
	if ids, hit := t.getCachedIDs(account, region, "QCE/CLB", "lb"); hit {
		log.Printf("Tencent CLB VIPs cache hit account_id=%s region=%s count=%d", account.AccountID, region, len(ids))
		return ids
	}

	credential := common.NewCredential(account.AccessKeyID, account.AccessKeySecret)
	client, err := clb.NewClient(credential, region, profile.NewClientProfile())
	if err != nil {
		return []string{}
	}
	req := clb.NewDescribeLoadBalancersRequest()
	resp, err := client.DescribeLoadBalancers(req)
	if err != nil || resp == nil || resp.Response == nil || resp.Response.LoadBalancerSet == nil {
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
	t.setCachedIDs(account, region, "QCE/CLB", "lb", vips)
	log.Printf("Tencent CLB VIPs enumerated account_id=%s region=%s count=%d", account.AccountID, region, len(vips))
	return vips
}

func (t *Collector) fetchCLBMonitor(account config.CloudAccount, region string, prod config.Product, vips []string) {
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
			req.Namespace = common.StringPtr("QCE/CLB")
			req.MetricName = common.StringPtr(m)
			req.Period = common.Uint64Ptr(uint64(period))
			var inst []*monitor.Instance
			for _, vip := range vips {
				inst = append(inst, &monitor.Instance{
					Dimensions: []*monitor.Dimension{
						{Name: common.StringPtr("vip"), Value: common.StringPtr(vip)},
					},
				})
			}
			req.Instances = inst
			start := time.Now().Add(-time.Duration(period) * time.Second)
			end := time.Now()
			req.StartTime = common.StringPtr(start.UTC().Format("2006-01-02T15:04:05Z"))
			req.EndTime = common.StringPtr(end.UTC().Format("2006-01-02T15:04:05Z"))
			resp, err := client.GetMonitorData(req)
			if err != nil || resp == nil || resp.Response == nil || resp.Response.DataPoints == nil {
				continue
			}
			for _, dp := range resp.Response.DataPoints {
				if dp == nil || len(dp.Dimensions) == 0 || len(dp.Values) == 0 {
					continue
				}
				var rid string
				for _, d := range dp.Dimensions {
					if d != nil && d.Name != nil && d.Value != nil && *d.Name == "vip" {
						rid = *d.Value
						break
					}
				}
				if rid == "" {
					continue
				}
				val := float64(0)
				if v := dp.Values[len(dp.Values)-1]; v != nil {
					val = *v
				}
				alias := metrics.NamespaceGauge("QCE/CLB", m)
				scaled := val
				if m == "VipIntraffic" || m == "VipOuttraffic" {
					scaled = val * 1000000
				}
				alias.WithLabelValues("tencent", account.AccountID, region, "lb", rid, "QCE/CLB", m, "").Set(scaled)
			}
		}
	}
}
