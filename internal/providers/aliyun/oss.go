package aliyun

import (
	"strings"
	"sync"
	"time"

	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/logger"
	"multicloud-exporter/internal/metrics"
	"multicloud-exporter/internal/providers/common"
	"multicloud-exporter/internal/utils"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
)

func (a *Collector) listOSSIDs(account config.CloudAccount, region string) []string {
	ctxLog := logger.NewContextLogger("Aliyun", "account_id", account.AccountID, "region", region)

	// Check region-level cache first (consistent with other resources)
	ids, _, hit := a.getCachedIDs(account, region, "acs_oss_dashboard", "oss")
	if hit {
		ctxLog.Debugf("OSS 资源缓存命中 region=%s 数量=%d", region, len(ids))
		if len(ids) > 0 {
			max := 5
			if len(ids) < max {
				max = len(ids)
			}
			preview := ids[:max]
			ctxLog.Debugf("枚举OSS存储桶完成（缓存） 数量=%d 预览=%v", len(ids), preview)
		} else {
			ctxLog.Debugf("枚举OSS存储桶完成（缓存） 数量=%d", len(ids))
		}
		return ids
	}

	// Use account-level cache to avoid duplicate ListBuckets calls across regions
	// OSS ListBuckets is a global operation, so we cache all buckets at account level
	a.ossMu.Lock()
	entry, ok := a.ossCache[account.AccountID]
	a.ossMu.Unlock()

	var allBuckets []ossBucketInfo
	cachedFromAccountLevel := false

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
		allBuckets = entry.Buckets
		cachedFromAccountLevel = true
		ctxLog.Debugf("OSS 账号级缓存命中 account=%s total_buckets=%d", account.AccountID, len(allBuckets))
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

			var buckets []ossBucketInfo
			marker := ""
			for {
				var lsRes oss.ListBucketsResult
				var callErr error
				start := time.Now()

				// Retry logic with exponential backoff (consistent with AWS S3)
				for attempt := 0; attempt < 5; attempt++ {
					lsRes, callErr = client.ListBuckets(oss.Marker(marker), oss.MaxKeys(100))
					if callErr == nil {
						metrics.RequestTotal.WithLabelValues("aliyun", "ListBuckets", "success").Inc()
						metrics.RecordRequest("aliyun", "ListBuckets", "success")
						metrics.RequestDuration.WithLabelValues("aliyun", "ListBuckets").Observe(time.Since(start).Seconds())
						break
					}
					status := common.ClassifyAliyunError(callErr)
					metrics.RequestTotal.WithLabelValues("aliyun", "ListBuckets", status).Inc()
					metrics.RecordRequest("aliyun", "ListBuckets", status)
					if status == "limit_error" {
						metrics.RateLimitTotal.WithLabelValues("aliyun", "ListBuckets").Inc()
					}
					if status == "auth_error" {
						ctxLog.Errorf("OSS ListBuckets 认证失败 account=%s region=%s: %v", account.AccountID, region, callErr)
						return nil, callErr
					}
					if attempt < 4 {
						sleep := time.Duration(200*(1<<attempt)) * time.Millisecond
						if sleep > 5*time.Second {
							sleep = 5 * time.Second
						}
						ctxLog.Debugf("OSS ListBuckets 重试 account=%s region=%s attempt=%d/%d sleep=%v",
							account.AccountID, region, attempt+1, 5, sleep)
						time.Sleep(sleep)
					}
				}

				if callErr != nil {
					ctxLog.Errorf("OSS ListBuckets 失败 account=%s region=%s: %v", account.AccountID, region, callErr)
					return nil, callErr
				}

				for _, bucket := range lsRes.Buckets {
					buckets = append(buckets, ossBucketInfo{
						Name:     bucket.Name,
						Location: strings.TrimPrefix(bucket.Location, "oss-"),
					})
				}

				if !lsRes.IsTruncated {
					break
				}
				marker = lsRes.NextMarker
			}

			if len(buckets) > 0 {
				a.ossMu.Lock()
				a.ossCache[account.AccountID] = ossCacheEntry{
					Buckets:   buckets,
					UpdatedAt: time.Now(),
				}
				a.ossMu.Unlock()
				ctxLog.Debugf("OSS ListBuckets API 调用成功 account=%s total_buckets=%d", account.AccountID, len(buckets))
			} else {
				ctxLog.Debugf("OSS ListBuckets API 调用成功 account=%s total_buckets=0", account.AccountID)
			}
			return buckets, nil
		})

		if err == nil {
			if b, ok := val.([]ossBucketInfo); ok {
				allBuckets = b
			}
		} else {
			ctxLog.Errorf("OSS ListBuckets 失败 account=%s region=%s error=%v", account.AccountID, region, err)
			// Cache empty result to avoid repeated API calls
			a.setCachedIDs(account, region, "acs_oss_dashboard", "oss", []string{}, nil)
			return []string{}
		}
	}

	// Filter by Region
	var regionBuckets []string
	for _, b := range allBuckets {
		if b.Location == region {
			regionBuckets = append(regionBuckets, b.Name)
		}
	}

	// Cache the filtered result at region level (consistent with other resources)
	a.setCachedIDs(account, region, "acs_oss_dashboard", "oss", regionBuckets, nil)

	ctxLog.Debugf("OSS 资源枚举完成 account=%s region=%s total_buckets=%d region_buckets=%d (account_cache=%v)",
		account.AccountID, region, len(allBuckets), len(regionBuckets), cachedFromAccountLevel)

	if len(regionBuckets) > 0 {
		max := 5
		if len(regionBuckets) < max {
			max = len(regionBuckets)
		}
		preview := regionBuckets[:max]
		ctxLog.Debugf("枚举OSS存储桶完成 数量=%d 预览=%v", len(regionBuckets), preview)
	} else {
		ctxLog.Debugf("枚举OSS存储桶完成 数量=%d (该区域无存储桶)", len(regionBuckets))
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
