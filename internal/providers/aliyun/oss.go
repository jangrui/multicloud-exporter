package aliyun

import (
	"context"
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
	ctxLog := logger.NewContextLogger("Aliyun", "account_id", account.AccountID, "region", region, "resource_type", "OSS")

	// 检查区域级缓存优先（与其他资源一致）
	ids, _, hit := a.getCachedIDs(account, region, "acs_oss_dashboard", "oss")
	if hit {
		ctxLog.Debugf("OSS 枚举存储桶 - 缓存命中 - API: ListBuckets - region=%s 数量=%d", region, len(ids))
		if len(ids) > 0 {
			max := 5
			if len(ids) < max {
				max = len(ids)
			}
			preview := ids[:max]
			ctxLog.Debugf("OSS 枚举存储桶 - 缓存命中 - API: ListBuckets - 已枚举 数量=%d 预览=%v", len(ids), preview)
		} else {
			ctxLog.Debugf("OSS 枚举存储桶 - 缓存命中 - API: ListBuckets - 已枚举 数量=%d", len(ids))
		}
		return ids
	}

	// 使用账号级缓存避免跨区域重复 ListBuckets 调用
	// OSS ListBuckets 是全局操作，所以我们在账号级别缓存所有 bucket
	a.ossMu.Lock()
	entry, ok := a.ossCache[account.AccountID]
	a.ossMu.Unlock()

	var allBuckets []ossBucketInfo
	cachedFromAccountLevel := false

	// TTL 逻辑
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
		ctxLog.Debugf("OSS 枚举存储桶 - 账号级缓存命中 - API: ListBuckets - account=%s total_buckets=%d", account.AccountID, len(allBuckets))
	} else {
		// 使用 singleflight 防止同一账号并发调用 ListBuckets
		// 无论哪个区域触发调用
		key := "oss_list_buckets_" + account.AccountID
		val, err, _ := a.sf.Do(key, func() (interface{}, error) {
			// 在 singleflight 内部双重检查缓存，确保如果刚刚更新过就不再获取
			a.ossMu.Lock()
			if e, ok := a.ossCache[account.AccountID]; ok && time.Since(e.UpdatedAt) < ttlDur {
				a.ossMu.Unlock()
				return e.Buckets, nil
			}
			a.ossMu.Unlock()

			// 从 API 获取
			// OSS ListBuckets 是全局操作，但我们需要一个 endpoint
			// 使用当前区域的 endpoint 是可以的
			client, err := a.clientFactory.NewOSSClient(region, account.AccessKeyID, account.AccessKeySecret)
			if err != nil {
				ctxLog.Errorf("Init OSS client error: %v", err)
				return nil, err
			}

			var buckets []ossBucketInfo
			marker := ""
			for {
				var lsRes oss.ListBucketsResult
				start := time.Now()

				callErr := common.RetryWithBackoff(context.Background(), common.DefaultRetryConfig(), func() error {
					var err error
					lsRes, err = client.ListBuckets(oss.Marker(marker), oss.MaxKeys(100))
					if err != nil {
						status := common.ClassifyAliyunError(err)
						metrics.RequestTotal.WithLabelValues("aliyun", "ListBuckets", status).Inc()
						metrics.RecordRequest("aliyun", "ListBuckets", status)
						if status == "limit_error" {
							metrics.RateLimitTotal.WithLabelValues("aliyun", "ListBuckets").Inc()
						}
						return err
					}
					return nil
				}, common.ShouldRetryForLimitError(common.AliyunClassifier))

				if callErr == nil {
					metrics.RequestTotal.WithLabelValues("aliyun", "ListBuckets", "success").Inc()
					metrics.RecordRequest("aliyun", "ListBuckets", "success")
					metrics.RequestDuration.WithLabelValues("aliyun", "ListBuckets").Observe(time.Since(start).Seconds())
				} else {
					status := common.ClassifyAliyunError(callErr)
					if status == "auth_error" {
						ctxLog.Errorf("OSS ListBuckets 认证失败 account=%s region=%s: %v", account.AccountID, region, callErr)
					}
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

	// 检查区域是否应跳过（产品级 RegionManager）
	rm := a.getProductRegionManager(AliyunProductOSS)
	if rm != nil && rm.ShouldSkipRegion(account.AccountID, region) {
		ctxLog.Debugf("OSS 枚举存储桶 - 区域已跳过（空区域）")
		return []string{}
	}

	// 分片过滤
	wTotal, wIndex := utils.ClusterConfig()
	if wTotal > 1 {
		filteredBuckets := []string{}
		for _, bucketName := range regionBuckets {
			instanceKey := account.AccountID + "|" + region + "|" + "acs_oss_dashboard" + "|" + bucketName
			if utils.ShouldProcess(instanceKey, wTotal, wIndex) {
				filteredBuckets = append(filteredBuckets, bucketName)
			}
		}
		regionBuckets = filteredBuckets
	}

	// 缓存过滤结果（consistent with other resources）
	a.setCachedIDs(account, region, "acs_oss_dashboard", "oss", regionBuckets, nil)

	// 更新区域状态
	if rm != nil {
		status := common.RegionStatusEmpty
		if len(regionBuckets) > 0 {
			status = common.RegionStatusActive
		}
		rm.UpdateRegionStatus(account.AccountID, region, len(regionBuckets), status)
		ctxLog.Debugf("更新区域状态 account=%s region=%s status=%s count=%d",
			account.AccountID, region, status, len(regionBuckets))
	}

	ctxLog.Debugf("OSS 枚举存储桶 - API: ListBuckets - 资源枚举完成 account=%s region=%s total_buckets=%d region_buckets=%d (account_cache=%v)",
		account.AccountID, region, len(allBuckets), len(regionBuckets), cachedFromAccountLevel)

	if len(regionBuckets) > 0 {
		max := 5
		if len(regionBuckets) < max {
			max = len(regionBuckets)
		}
		preview := regionBuckets[:max]
		ctxLog.Debugf("OSS 枚举存储桶 - API: ListBuckets - 已枚举 数量=%d 预览=%v", len(regionBuckets), preview)
	} else {
		ctxLog.Debugf("OSS 枚举存储桶 - API: ListBuckets - 已枚举 数量=%d (该区域无存储桶)", len(regionBuckets))
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
			defer func() {
				if r := recover(); r != nil {
					logger.Log.Errorf("OSS fetchOSSBucketTags panic: %v", r)
				}
			}()
			// 添加超时控制
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			// 注意：阿里云 SDK 某些旧版本可能不支持 context，这里仅作最佳实践
			// 如果 SDK 不支持 context，这个超时可能无法中断请求，但能防止 goroutine 永久泄漏
			_ = ctx

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
