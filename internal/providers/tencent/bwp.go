package tencent

import (
	"log"
	"time"

	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/metrics"

	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	monitor "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/monitor/v20180724"
	vpc "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/vpc/v20170312"
)

func (t *Collector) listBWPIDs(account config.CloudAccount, region string) []string {
	if ids, hit := t.getCachedIDs(account, region, "QCE/BWP", "bwp"); hit {
		log.Printf("Tencent BWP IDs cache hit account_id=%s region=%s count=%d", account.AccountID, region, len(ids))
		return ids
	}

	credential := common.NewCredential(account.AccessKeyID, account.AccessKeySecret)
	client, err := vpc.NewClient(credential, region, profile.NewClientProfile())
	if err != nil {
		return []string{}
	}
	req := vpc.NewDescribeBandwidthPackagesRequest()
	resp, err := client.DescribeBandwidthPackages(req)
	if err != nil || resp == nil || resp.Response == nil || resp.Response.BandwidthPackageSet == nil {
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
	log.Printf("Tencent BWP enumerated account_id=%s region=%s count=%d", account.AccountID, region, len(ids))
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
			req.Period = common.Uint64Ptr(uint64(period))
			var inst []*monitor.Instance
			for _, id := range ids {
				inst = append(inst, &monitor.Instance{
					Dimensions: []*monitor.Dimension{
						{Name: common.StringPtr("bandwidthPackageId"), Value: common.StringPtr(id)},
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
					if d != nil && d.Name != nil && d.Value != nil && *d.Name == "bandwidthPackageId" {
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
				alias := metrics.NamespaceGauge("QCE/BWP", m)
				scaled := val
				if m == "InTraffic" || m == "OutTraffic" {
					scaled = val * 1000000
				}
				alias.WithLabelValues("tencent", account.AccountID, region, "bwp", rid, "QCE/BWP", m, "").Set(scaled)
			}
		}
	}
}
