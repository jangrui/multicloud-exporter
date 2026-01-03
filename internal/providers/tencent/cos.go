package tencent

import (
	"context"
	"strings"
	"sync"
	"time"

	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/logger"
	"multicloud-exporter/internal/metrics"
	providerscommon "multicloud-exporter/internal/providers/common"
	"multicloud-exporter/internal/utils"

	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	monitor "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/monitor/v20180724"
	"github.com/tencentyun/cos-go-sdk-v5"
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

	ctxLog := logger.NewContextLogger("Tencent", "account_id", account.AccountID, "region", region, "rtype", "cos")

	// 产品级分片：获取集群配置用于产品级分片判断
	wTotal, wIndex := utils.ClusterConfig()
	for _, p := range prods {
		if p.Namespace != "QCE/COS" {
			continue
		}
		// 产品级分片判断：只有当前 Pod 应该处理的产品才进行采集
		// 分片键格式：AccountID|Region|Namespace
		productKey := account.AccountID + "|" + region + "|" + p.Namespace
		if !utils.ShouldProcess(productKey, wTotal, wIndex) {
			ctxLog.Debugf("COS 产品跳过（分片不匹配）")
			continue
		}
		buckets := t.listCOSBuckets(account, region)
		if len(buckets) == 0 {
			return
		}
		t.fetchCOSMonitor(account, region, p, buckets)
	}
}

// isCOSCapacityMetric 判断是否为容量类指标（每日更新）
// 腾讯云 COS 容量类指标每日更新一次，需要使用更长的 Period 和时间窗口
func isCOSCapacityMetric(metricName string) bool {
	capacityMetrics := map[string]bool{
		// 存储容量类指标（每日更新）
		"StdStorage":              true, // 标准存储用量
		"SiaStorage":              true, // 低频存储用量
		"ArcStorage":              true, // 归档存储用量
		"DeepArcStorage":          true, // 深度归档存储用量
		"ItFreqStorage":           true, // 智能分层高频用量
		"ItInfreqStorage":         true, // 智能分层低频用量
		"DeepArcMultipartStorage": true, // 深度归档碎片容量
		"DeepArcMultipartNumber":  true, // 深度归档碎片数量
		"DeepArcObjectNumber":     true, // 深度归档对象数量
		// 注意：StdMultipartStorage 和 SiaMultipartStorage 在腾讯云 COS 中不存在，已移除
	}
	return capacityMetrics[metricName]
}

func (t *Collector) listCOSBuckets(account config.CloudAccount, region string) []string {
	ctxLog := logger.NewContextLogger("Tencent", "account_id", account.AccountID, "region", region, "rtype", "cos")

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
		ctxLog.Errorf("COS 客户端创建失败，错误=%v", err)
		return []string{}
	}

	start := time.Now()
	// Get Service lists all buckets
	// 注意：腾讯云 COS GetService API 遵循 S3 兼容协议，一次性返回所有 bucket，不支持分页
	// 通常一个账号的 bucket 数量不会太多（通常 < 1000），所以单次返回是合理的
	var s *cos.ServiceGetResult
	var callErr error
	for attempt := 0; attempt < 3; attempt++ {
		s, _, callErr = client.GetService(context.Background())
		if callErr == nil {
			metrics.RequestTotal.WithLabelValues("tencent", "ListBuckets", "success").Inc()
			metrics.RequestDuration.WithLabelValues("tencent", "ListBuckets").Observe(time.Since(start).Seconds())
			metrics.RecordRequest("tencent", "ListBuckets", "success")
			break
		}
		status := providerscommon.ClassifyTencentError(callErr)
		metrics.RequestTotal.WithLabelValues("tencent", "ListBuckets", status).Inc()
		metrics.RecordRequest("tencent", "ListBuckets", status)
		if status == "limit_error" {
			// 记录限流指标
			metrics.RateLimitTotal.WithLabelValues("tencent", "ListBuckets").Inc()
		}
		if status == "auth_error" {
			ctxLog.Errorf("ListBuckets 认证错误: %v", callErr)
			return []string{}
		}
		// 指数退避重试
		sleep := time.Duration(200*(1<<attempt)) * time.Millisecond
		if sleep > 5*time.Second {
			sleep = 5 * time.Second
		}
		time.Sleep(sleep)
	}
	if callErr != nil {
		ctxLog.Errorf("ListBuckets API调用错误: %v", callErr)
		return []string{}
	}

	var buckets []string
	for _, b := range s.Buckets {
		// Filter by region
		// Bucket.Region is the region code, e.g., "ap-guangzhou"
		if b.Region == region {
			buckets = append(buckets, b.Name)
		}
	}

	t.setCachedIDs(account, region, "QCE/COS", "cos", buckets)

	// 更新区域状态
	if t.regionManager != nil {
		status := providerscommon.RegionStatusEmpty
		if len(buckets) > 0 {
			status = providerscommon.RegionStatusActive
		}
		t.regionManager.UpdateRegionStatus(account.AccountID, region, len(buckets), status)
		ctxLog.Debugf("更新区域状态，status=%s，count=%d", status, len(buckets))
	}

	if len(buckets) > 0 {
		max := 5
		if len(buckets) < max {
			max = len(buckets)
		}
		preview := buckets[:max]
		ctxLog.Debugf("COS 存储桶已枚举，数量=%d 预览=%v", len(buckets), preview)
	} else {
		ctxLog.Debugf("COS 存储桶已枚举，数量=%d", len(buckets))
	}
	return buckets
}

func (t *Collector) fetchCOSBucketCodeNames(account config.CloudAccount, region string, buckets []string) map[string]string {
	out := make(map[string]string, len(buckets))
	client, err := t.clientFactory.NewCOSClient(region, account.AccessKeyID, account.AccessKeySecret)
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
			var tags map[string]string
			var callErr error
			for attempt := 0; attempt < 3; attempt++ {
				tags, callErr = client.GetBucketTagging(context.Background(), bucket, region)
				if callErr == nil {
					break
				}
				status := providerscommon.ClassifyTencentError(callErr)
				if status == "limit_error" {
					// 记录限流指标
					metrics.RateLimitTotal.WithLabelValues("tencent", "GetBucketTagging").Inc()
				}
				if status == "auth_error" {
					return
				}
				// 指数退避重试
				sleep := time.Duration(200*(1<<attempt)) * time.Millisecond
				if sleep > 5*time.Second {
					sleep = 5 * time.Second
				}
				time.Sleep(sleep)
			}
			if callErr != nil || len(tags) == 0 {
				return
			}
			for k, v := range tags {
				if strings.EqualFold(k, "CodeName") || strings.EqualFold(k, "code_name") {
					out[bucket] = v
					break
				}
			}
		}(b)
	}
	wg.Wait()
	return out
}

func (t *Collector) fetchCOSMonitor(account config.CloudAccount, region string, prod config.Product, buckets []string) {
	ctxLog := logger.NewContextLogger("Tencent", "account_id", account.AccountID, "region", region, "rtype", "cos")

	client, err := t.clientFactory.NewMonitorClient(region, account.AccessKeyID, account.AccessKeySecret)
	if err != nil {
		ctxLog.Errorf("Monitor 客户端创建失败，错误=%v", err)
		return
	}

	period := int64(300) // Default 5 minutes for COS usually
	if prod.Period != nil {
		period = int64(*prod.Period)
	}

	// Monitor API has limits on number of instances per request.
	// We need to batch the buckets.
	batchSize := 10 // Safe batch size

	codeNames := t.fetchCOSBucketCodeNames(account, region, buckets)

	for _, group := range prod.MetricInfo {
		groupPeriod := period
		if group.Period != nil {
			groupPeriod = int64(*group.Period)
		}
		for _, m := range group.MetricList {
			// 根据指标类型动态调整 Period 和时间窗口
			var localPeriod int64
			var startT, endT time.Time
			now := time.Now()

			if isCOSCapacityMetric(m) {
				// 容量类指标：每日更新，使用 86400 秒 Period，时间窗口回溯 48 小时
				localPeriod = 86400
				startT = now.Add(-48 * time.Hour)
				endT = now
				ctxLog.Debugf("COS 容量类指标，metric=%s，period=%d", m, localPeriod)
			} else {
				// 请求类指标：使用配置的 Period，时间窗口回溯 2 个周期
				localPeriod = groupPeriod
				startT = now.Add(-time.Duration(localPeriod*2) * time.Second)
				endT = now.Add(-time.Duration(localPeriod) * time.Second)
			}

			for i := 0; i < len(buckets); i += batchSize {
				end := i + batchSize
				if end > len(buckets) {
					end = len(buckets)
				}
				batch := buckets[i:end]

				req := monitor.NewGetMonitorDataRequest()
				req.Namespace = common.StringPtr(prod.Namespace)
				req.MetricName = common.StringPtr(m)
				req.Period = common.Uint64Ptr(uint64(localPeriod))

				var inst []*monitor.Instance
				for _, bucket := range batch {
					inst = append(inst, &monitor.Instance{
						Dimensions: []*monitor.Dimension{
							{Name: common.StringPtr("bucket"), Value: common.StringPtr(bucket)},
						},
					})
				}
				req.Instances = inst

				req.StartTime = common.StringPtr(startT.UTC().Format("2006-01-02T15:04:05Z"))
				req.EndTime = common.StringPtr(endT.UTC().Format("2006-01-02T15:04:05Z"))

				reqStart := time.Now()
				resp, err := client.GetMonitorData(req)
				if err != nil {
					status := providerscommon.ClassifyTencentError(err)
					metrics.RequestTotal.WithLabelValues("tencent", "GetMonitorData", status).Inc()
					metrics.RecordRequest("tencent", "GetMonitorData", status)
					if status == "limit_error" {
						// 记录限流指标
						metrics.RateLimitTotal.WithLabelValues("tencent", "GetMonitorData").Inc()
					}
					ctxLog.Warnf("GetMonitorData API 调用错误，指标=%s: %v", m, err)
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
					// 如果最后一个值为 nil，表示没有数据，跳过指标（而不是设置为 0）
					lastVal := point.Values[len(point.Values)-1]
					if lastVal == nil {
						continue
					}
					val := *lastVal

					vec, count := metrics.NamespaceGauge("QCE/COS", m)
					codeName := codeNames[bucketName]
					labels := []string{"tencent", account.AccountID, region, "cos", bucketName, "QCE/COS", m, codeName}
					for len(labels) < count {
						labels = append(labels, "")
					}
					vec.WithLabelValues(labels...).Set(val)
				}
				// 优化：移除指标间延迟，降低云API压力
				// 原代码: time.Sleep(50 * time.Millisecond)
				// 优化后: 连续处理下一个指标，总耗时减少 N指标 × 50ms
			}
		}
	}
}
