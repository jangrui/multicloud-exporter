package tencent

import (
	"context"
	"time"

	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/logger"
	"multicloud-exporter/internal/metrics"

	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	monitor "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/monitor/v20180724"
)

func (t *Collector) collectCOS(account config.CloudAccount, region string) {
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
		if p.Namespace != "QCE/COS" {
			continue
		}
		buckets := t.listCOSBuckets(account, region)
		if len(buckets) == 0 {
			return
		}
		t.fetchCOSMonitor(account, region, p, buckets)
	}
}

func (t *Collector) listCOSBuckets(account config.CloudAccount, region string) []string {
	if ids, hit := t.getCachedIDs(account, region, "QCE/COS", "cos"); hit {
		return ids
	}

	// COS Service Get usually works with any region endpoint, but best to use the current region or a default one.
	// Since we are iterating regions, we can just use the current region's endpoint.
	// Note: COS Go SDK requires a BaseURL. For Service operations, BucketURL is not strictly needed but the client requires it?
	// Actually for Service.Get, we don't need a bucket URL, but NewClient requires it.
	// We can pass nil or a dummy URL? Let's try passing a dummy URL with the correct region.

	// Construct a dummy bucket URL for the region.
	// We use the factory now.
	client, err := t.clientFactory.NewCOSClient(region, account.AccessKeyID, account.AccessKeySecret)
	if err != nil {
		logger.Log.Errorf("Tencent COS client error: %v", err)
		return []string{}
	}

	start := time.Now()
	// Get Service lists all buckets
	s, _, err := client.GetService(context.Background())
	if err != nil {
		metrics.RequestTotal.WithLabelValues("tencent", "ListBuckets", "error").Inc()
		logger.Log.Errorf("Tencent ListBuckets error: %v", err)
		return []string{}
	}
	metrics.RequestTotal.WithLabelValues("tencent", "ListBuckets", "success").Inc()
	metrics.RequestDuration.WithLabelValues("tencent", "ListBuckets").Observe(time.Since(start).Seconds())

	var buckets []string
	for _, b := range s.Buckets {
		// Filter by region
		// Bucket.Region is the region code, e.g., "ap-guangzhou"
		if b.Region == region {
			buckets = append(buckets, b.Name)
		}
	}

	t.setCachedIDs(account, region, "QCE/COS", "cos", buckets)
	if len(buckets) > 0 {
		max := 5
		if len(buckets) < max {
			max = len(buckets)
		}
		preview := buckets[:max]
        logger.Log.Debugf("Tencent COS buckets enumerated account_id=%s region=%s count=%d preview=%v", account.AccountID, region, len(buckets), preview)
    } else {
        logger.Log.Debugf("Tencent COS buckets enumerated account_id=%s region=%s count=%d", account.AccountID, region, len(buckets))
    }
	return buckets
}

func (t *Collector) fetchCOSMonitor(account config.CloudAccount, region string, prod config.Product, buckets []string) {
	client, err := t.clientFactory.NewMonitorClient(region, account.AccessKeyID, account.AccessKeySecret)
	if err != nil {
		logger.Log.Errorf("Tencent Monitor client error: %v", err)
		return
	}

	period := int64(300) // Default 5 minutes for COS usually
	if prod.Period != nil {
		period = int64(*prod.Period)
	}

	// Monitor API has limits on number of instances per request.
	// We need to batch the buckets.
	batchSize := 10 // Safe batch size

	for _, group := range prod.MetricInfo {
		if group.Period != nil {
			period = int64(*group.Period)
		}
		for _, m := range group.MetricList {
			for i := 0; i < len(buckets); i += batchSize {
				end := i + batchSize
				if end > len(buckets) {
					end = len(buckets)
				}
				batch := buckets[i:end]

				req := monitor.NewGetMonitorDataRequest()
				req.Namespace = common.StringPtr(prod.Namespace)
				req.MetricName = common.StringPtr(m)
				req.Period = common.Uint64Ptr(uint64(period))

				var inst []*monitor.Instance
				for _, bucket := range batch {
					inst = append(inst, &monitor.Instance{
						Dimensions: []*monitor.Dimension{
							{Name: common.StringPtr("bucket"), Value: common.StringPtr(bucket)},
						},
					})
				}
				req.Instances = inst

				now := time.Now()
				// Adjust time window to ensure data availability (e.g. 5 mins ago)
				// Cloud Monitor data might have delay.
				startT := now.Add(-time.Duration(period*2) * time.Second)
				endT := now.Add(-time.Duration(period) * time.Second)

				req.StartTime = common.StringPtr(startT.UTC().Format("2006-01-02T15:04:05Z"))
				req.EndTime = common.StringPtr(endT.UTC().Format("2006-01-02T15:04:05Z"))

				reqStart := time.Now()
				resp, err := client.GetMonitorData(req)
				if err != nil {
					status := classifyTencentError(err)
					metrics.RequestTotal.WithLabelValues("tencent", "GetMonitorData", status).Inc()
					metrics.RecordRequest("tencent", "GetMonitorData", status)
					logger.Log.Warnf("GetMonitorData error metric=%s: %v", m, err)
					continue
				}
				metrics.RequestTotal.WithLabelValues("tencent", "GetMonitorData", "success").Inc()
				metrics.RecordRequest("tencent", "GetMonitorData", "success")
				metrics.RequestDuration.WithLabelValues("tencent", "GetMonitorData").Observe(time.Since(reqStart).Seconds())

				if resp == nil || resp.Response == nil || len(resp.Response.DataPoints) == 0 {
					continue
				}

				for _, point := range resp.Response.DataPoints {
					if point == nil || len(point.Values) == 0 {
						continue
					}
					// Find bucket name from dimensions
					var bucketName string
					if point.Dimensions != nil {
						for _, d := range point.Dimensions {
							if *d.Name == "bucket" {
								bucketName = *d.Value
								break
							}
						}
					}
					if bucketName == "" {
						continue
					}

					// Use the latest value
					val := *point.Values[len(point.Values)-1]

					vec, count := metrics.NamespaceGauge("QCE/COS", m)
					labels := []string{"tencent", account.AccountID, region, "cos", bucketName, "QCE/COS", m, bucketName}
					for len(labels) < count {
						labels = append(labels, "")
					}
					vec.WithLabelValues(labels...).Set(val)
				}
				// Avoid rate limit
				time.Sleep(50 * time.Millisecond)
			}
		}
	}
}
