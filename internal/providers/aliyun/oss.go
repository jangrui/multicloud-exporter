package aliyun

import (
	"strings"
	"sync"
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
		} else if server := a.cfg.GetServer(); server != nil && server.DiscoveryTTL != "" {
			if d, err := utils.ParseDuration(server.DiscoveryTTL); err == nil {
				ttlDur = d
			}
		}
	}

	valid := ok && time.Since(entry.UpdatedAt) < ttlDur

	if valid {
		buckets = entry.Buckets
	} else {
		// Use singleflight to prevent concurrent ListBuckets calls for the same account
		// regardless of which region triggered the call.
		key := "oss_list_buckets_" + account.AccountID
		val, err, _ := a.sf.Do(key, func() (interface{}, error) {
			// Double-check cache inside singleflight to ensure we don't fetch if just updated
			a.ossMu.Lock()
			if e, ok := a.ossCache[account.AccountID]; ok && time.Since(e.UpdatedAt) < ttlDur {
				a.ossMu.Unlock()
				return e.Buckets, nil
			}
			a.ossMu.Unlock()

			// Fetch from API
			// OSS ListBuckets is a global operation, but we need an endpoint.
			// Using the current region's endpoint is fine.
			client, err := a.clientFactory.NewOSSClient(region, account.AccessKeyID, account.AccessKeySecret)
			if err != nil {
				ctxLog.Errorf("Init OSS client error: %v", err)
				return nil, err
			}

			var allBuckets []ossBucketInfo
			marker := ""
			for {
				start := time.Now()
				lsRes, err := client.ListBuckets(oss.Marker(marker), oss.MaxKeys(100))
				if err != nil {
					metrics.RequestTotal.WithLabelValues("aliyun", "ListBuckets", "error").Inc()
					metrics.RecordRequest("aliyun", "ListBuckets", "error")
					ctxLog.Errorf("ListBuckets error: %v", err)
					return nil, err
				}
				metrics.RequestTotal.WithLabelValues("aliyun", "ListBuckets", "success").Inc()
				metrics.RecordRequest("aliyun", "ListBuckets", "success")
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
			}
			return allBuckets, nil
		})

		if err == nil {
			if b, ok := val.([]ossBucketInfo); ok {
				buckets = b
			}
		}
	}

	// Filter by Region
	var regionBuckets []string
	for _, b := range buckets {
		if b.Location == region {
			regionBuckets = append(regionBuckets, b.Name)
		}
	}
	ctxLog.Debugf("ListOSSIDs total=%d region_match=%d region=%s", len(buckets), len(regionBuckets), region)

	if len(regionBuckets) > 0 {
		max := 5
		if len(regionBuckets) < max {
			max = len(regionBuckets)
		}
		preview := regionBuckets[:max]
		ctxLog.Debugf("枚举OSS存储桶完成 数量=%d 预览=%v (Cached: %v)", len(regionBuckets), preview, valid)
	} else {
		ctxLog.Debugf("枚举OSS存储桶完成 数量=%d (Cached: %v)", len(regionBuckets), valid)
	}
	return regionBuckets
}

func (a *Collector) fetchOSSBucketTags(account config.CloudAccount, region string, buckets []string) map[string]string {
	out := make(map[string]string, len(buckets))
	var mu sync.Mutex
	client, err := a.clientFactory.NewOSSClient(region, account.AccessKeyID, account.AccessKeySecret)
	if err != nil {
		return out
	}
	limit := 5
	sem := make(chan struct{}, limit)
	var wg sync.WaitGroup
	for _, b := range buckets {
		wg.Add(1)
		sem <- struct{}{}
		go func(bucket string) {
			defer wg.Done()
			defer func() { <-sem }()
			res, err := client.GetBucketTagging(bucket)
			if err != nil {
				return
			}
			for _, t := range res.Tags {
				if strings.EqualFold(t.Key, "CodeName") || strings.EqualFold(t.Key, "code_name") {
					mu.Lock()
					out[bucket] = t.Value
					mu.Unlock()
					break
				}
			}
		}(b)
	}
	wg.Wait()
	return out
}
