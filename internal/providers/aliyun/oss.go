package aliyun

import (
	"strings"
	"time"

	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/logger"
	"multicloud-exporter/internal/metrics"
	"multicloud-exporter/internal/utils"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
)

func (a *Collector) listOSSIDs(account config.CloudAccount, region string) []string {
	ctxLog := logger.NewContextLogger("Aliyun", "account_id", account.AccountID, "region", region)

	// Check Account-level Cache
	a.ossMu.Lock()
	entry, ok := a.ossCache[account.AccountID]
	a.ossMu.Unlock()

	var buckets []ossBucketInfo

	// TTL Logic
	ttlDur := time.Hour
	if a.cfg != nil {
		if a.cfg.Server != nil && a.cfg.Server.DiscoveryTTL != "" {
			if d, err := utils.ParseDuration(a.cfg.Server.DiscoveryTTL); err == nil {
				ttlDur = d
			}
		} else if a.cfg.ServerConf != nil && a.cfg.ServerConf.DiscoveryTTL != "" {
			if d, err := utils.ParseDuration(a.cfg.ServerConf.DiscoveryTTL); err == nil {
				ttlDur = d
			}
		}
	}

	valid := ok && time.Since(entry.UpdatedAt) < ttlDur

	if valid {
		buckets = entry.Buckets
	} else {
		// Fetch from API
		// OSS ListBuckets is a global operation, but we need an endpoint.
		// Using the current region's endpoint is fine.
		client, err := a.clientFactory.NewOSSClient(region, account.AccessKeyID, account.AccessKeySecret)
		if err != nil {
			ctxLog.Errorf("Init OSS client error: %v", err)
			return []string{}
		}

		var allBuckets []ossBucketInfo
		marker := ""
		for {
			start := time.Now()
			lsRes, err := client.ListBuckets(oss.Marker(marker), oss.MaxKeys(100))
			if err != nil {
				metrics.RequestTotal.WithLabelValues("aliyun", "ListBuckets", "error").Inc()
				ctxLog.Errorf("ListBuckets error: %v", err)
				break
			}
			metrics.RequestTotal.WithLabelValues("aliyun", "ListBuckets", "success").Inc()
			metrics.RequestDuration.WithLabelValues("aliyun", "ListBuckets").Observe(time.Since(start).Seconds())

			for _, bucket := range lsRes.Buckets {
				allBuckets = append(allBuckets, ossBucketInfo{
					Name:     bucket.Name,
					Location: strings.TrimPrefix(bucket.Location, "oss-"),
				})
			}

			if !lsRes.IsTruncated {
				break
			}
			marker = lsRes.NextMarker
		}

		if len(allBuckets) > 0 {
			a.ossMu.Lock()
			a.ossCache[account.AccountID] = ossCacheEntry{
				Buckets:   allBuckets,
				UpdatedAt: time.Now(),
			}
			a.ossMu.Unlock()
			buckets = allBuckets
		}
	}

	// Filter by Region
	var regionBuckets []string
	for _, b := range buckets {
		if b.Location == region {
			regionBuckets = append(regionBuckets, b.Name)
		}
	}

    if len(regionBuckets) > 0 {
        max := 5
        if len(regionBuckets) < max {
            max = len(regionBuckets)
        }
        preview := regionBuckets[:max]
        ctxLog.Infof("枚举OSS存储桶完成 数量=%d 预览=%v (Cached: %v)", len(regionBuckets), preview, valid)
    } else {
        ctxLog.Infof("枚举OSS存储桶完成 数量=%d (Cached: %v)", len(regionBuckets), valid)
    }
    return regionBuckets
}
