package aws

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/logger"
	"multicloud-exporter/internal/metrics"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func (c *Collector) collectS3(account config.CloudAccount) {
	if c.cfg == nil {
		return
	}
	// 产品配置来自 discovery.Manager（source of truth）
	var prods []config.Product
	if c.disc != nil {
		if ps, ok := c.disc.Get()["aws"]; ok && len(ps) > 0 {
			prods = ps
		}
	}
	if len(prods) == 0 {
		return
	}
	var s3Prod *config.Product
	for i := range prods {
		if prods[i].Namespace == "AWS/S3" {
			s3Prod = &prods[i]
			break
		}
	}
	if s3Prod == nil || len(s3Prod.MetricInfo) == 0 {
		return
	}

	ctx := context.Background()

	// S3 ListBuckets 是全局接口，region 可用 us-east-1。
	s3Client, err := c.clientFactory.NewS3Client(ctx, "us-east-1", account.AccessKeyID, account.AccessKeySecret)
	if err != nil {
		logger.Log.Warnf("AWS S3 创建客户端失败 account=%s: %v", account.AccountID, err)
		return
	}
	start := time.Now()
	var bucketsOut *s3.ListBucketsOutput
	for attempt := 0; attempt < 5; attempt++ {
		bucketsOut, err = s3Client.ListBuckets(ctx, &s3.ListBucketsInput{})
		if err == nil {
			metrics.RequestTotal.WithLabelValues("aws", "ListBuckets", "success").Inc()
			metrics.RecordRequest("aws", "ListBuckets", "success")
			metrics.RequestDuration.WithLabelValues("aws", "ListBuckets").Observe(time.Since(start).Seconds())
			break
		}
		status := classifyAWSError(err)
		metrics.RequestTotal.WithLabelValues("aws", "ListBuckets", status).Inc()
		metrics.RecordRequest("aws", "ListBuckets", status)
		if status == "limit_error" {
			metrics.RateLimitTotal.WithLabelValues("aws", "ListBuckets").Inc()
		}
		if status == "auth_error" {
			logger.Log.Warnf("AWS S3 ListBuckets 认证失败 account=%s: %v", account.AccountID, err)
			return
		}
		if attempt < 4 {
			sleep := time.Duration(200*(1<<attempt)) * time.Millisecond
			if sleep > 5*time.Second {
				sleep = 5 * time.Second
			}
			time.Sleep(sleep)
		}
	}
	if err != nil {
		logger.Log.Warnf("AWS S3 ListBuckets 失败 account=%s: %v", account.AccountID, err)
		return
	}

	var buckets []string
	for _, b := range bucketsOut.Buckets {
		if b.Name != nil && *b.Name != "" {
			buckets = append(buckets, *b.Name)
		}
	}
	if len(buckets) == 0 {
		return
	}

	codeNames := c.fetchS3BucketCodeNames(ctx, s3Client, buckets)

	// CloudWatch S3 指标维度：BucketName + StorageType（对存储类指标必填）
	cwClient, err := c.clientFactory.NewCloudWatchClient(ctx, "us-east-1", account.AccessKeyID, account.AccessKeySecret)
	if err != nil {
		logger.Log.Warnf("AWS CloudWatch 创建客户端失败 account=%s: %v", account.AccountID, err)
		return
	}

	defaultPeriod := int32(86400) // 默认按存储类（天粒度）回退
	if s3Prod.Period != nil && *s3Prod.Period > 0 {
		defaultPeriod = int32(*s3Prod.Period)
	}

	// 统计指标收集情况
	totalMetricsAttempted := 0
	metricsCollected := make(map[string]int) // metricName -> bucket count
	metricsSkipped := make(map[string]int)   // metricName -> bucket count

	for _, group := range s3Prod.MetricInfo {
		localPeriod := defaultPeriod
		if group.Period != nil && *group.Period > 0 {
			localPeriod = int32(*group.Period)
		}
		for _, m := range group.MetricList {
			metricName := strings.TrimSpace(m)
			if metricName == "" {
				continue
			}
			totalMetricsAttempted++
			metricsCollected[metricName] = 0
			metricsSkipped[metricName] = 0
			needStorageType := metricName == "BucketSizeBytes" || metricName == "NumberOfObjects"
			needFilterID := !needStorageType
			storageType := "StandardStorage"
			filterID := "EntireBucket"
			if metricName == "NumberOfObjects" {
				storageType = "AllStorageTypes"
			}
			stat := statForS3Metric(metricName)
			// CloudWatch GetMetricData 一次最多查询 500 个指标，需要分批处理
			const maxQueriesPerRequest = 500
			allResults := make(map[string]cwtypes.MetricDataResult)

			for batchStart := 0; batchStart < len(buckets); batchStart += maxQueriesPerRequest {
				batchEnd := batchStart + maxQueriesPerRequest
				if batchEnd > len(buckets) {
					batchEnd = len(buckets)
				}
				batchBuckets := buckets[batchStart:batchEnd]

				queries := make([]cwtypes.MetricDataQuery, 0, len(batchBuckets))
				for i, bn := range batchBuckets {
					id := sanitizeCWQueryID(batchStart + i)
					dims := []cwtypes.Dimension{{Name: aws.String("BucketName"), Value: aws.String(bn)}}
					if needStorageType {
						dims = append(dims, cwtypes.Dimension{Name: aws.String("StorageType"), Value: aws.String(storageType)})
					}
					if needFilterID {
						dims = append(dims, cwtypes.Dimension{Name: aws.String("FilterId"), Value: aws.String(filterID)})
					}
					queries = append(queries, cwtypes.MetricDataQuery{
						Id: aws.String(id),
						MetricStat: &cwtypes.MetricStat{
							Metric: &cwtypes.Metric{
								Namespace:  aws.String(s3Prod.Namespace),
								MetricName: aws.String(metricName),
								Dimensions: dims,
							},
							Period: aws.Int32(localPeriod),
							Stat:   aws.String(stat),
						},
						ReturnData: aws.Bool(true),
					})
				}

				// 查询时间窗口：至少覆盖最近 30 分钟，确保能获取到有延迟的数据点
				// CloudWatch 指标延迟：正常 2-5 分钟，高负载时可能达到 10-15 分钟
				// 使用 30 分钟窗口更安全，且只取最新值（Values[0]），不影响数据新鲜度
				endTime := time.Now().UTC()
				minWindow := 30 * time.Minute
				window := time.Duration(localPeriod) * time.Second * 2 // 至少 2 个周期
				if window < minWindow {
					window = minWindow
				}
				startTime := endTime.Add(-window)
				reqStart := time.Now()
				var resp *cloudwatch.GetMetricDataOutput
				for attempt := 0; attempt < 5; attempt++ {
					resp, err = cwClient.GetMetricData(ctx, &cloudwatch.GetMetricDataInput{
						StartTime:         aws.Time(startTime),
						EndTime:           aws.Time(endTime),
						MetricDataQueries: queries,
						ScanBy:            cwtypes.ScanByTimestampDescending,
					})
					if err == nil {
						metrics.RequestTotal.WithLabelValues("aws", "GetMetricData", "success").Inc()
						metrics.RecordRequest("aws", "GetMetricData", "success")
						metrics.RequestDuration.WithLabelValues("aws", "GetMetricData").Observe(time.Since(reqStart).Seconds())
						break
					}
					status := classifyAWSError(err)
					metrics.RequestTotal.WithLabelValues("aws", "GetMetricData", status).Inc()
					metrics.RecordRequest("aws", "GetMetricData", status)
					if status == "limit_error" {
						metrics.RateLimitTotal.WithLabelValues("aws", "GetMetricData").Inc()
					}
					if status == "auth_error" {
						logger.Log.Warnf("AWS CloudWatch GetMetricData 认证失败 account=%s metric=%s: %v", account.AccountID, metricName, err)
						break
					}
					if attempt < 4 {
						sleep := time.Duration(200*(1<<attempt)) * time.Millisecond
						if sleep > 5*time.Second {
							sleep = 5 * time.Second
						}
						time.Sleep(sleep)
					}
				}
				if err != nil {
					logger.Log.Debugf("AWS CloudWatch GetMetricData 失败 account=%s metric=%s batch=%d-%d: %v", account.AccountID, metricName, batchStart, batchEnd, err)
					continue
				}

				// 将每个 query 的最新值写入结果集
				for _, r := range resp.MetricDataResults {
					if r.Id != nil {
						allResults[*r.Id] = r
					}
				}

				// 批次间轻微节流
				if batchEnd < len(buckets) {
					time.Sleep(50 * time.Millisecond)
				}
			}

			results := allResults
			// 在循环外确定 vecLabels，确保每个指标只调用一次 NamespaceGauge
			var vecLabels []string
			if needStorageType {
				vecLabels = []string{"BucketName", "StorageType"}
			} else {
				vecLabels = []string{"BucketName", "FilterId"}
			}
			vec, count := metrics.NamespaceGauge(s3Prod.Namespace, metricName, vecLabels...)
			rtype := metrics.GetNamespacePrefix(s3Prod.Namespace)
			if rtype == "" {
				rtype = "s3"
			}
			for i, bn := range buckets {
				id := sanitizeCWQueryID(i)
				r, ok := results[id]
				if !ok || len(r.Values) == 0 {
					metricsSkipped[metricName]++
					continue
				}
				val := r.Values[0]
				// 标准 labels: cloud_provider, account_id, region, resource_type, resource_id, namespace, metric_name, code_name
				// 然后加上动态维度值（BucketName 的值就是 resource_id，所以只需要添加其他维度值）
				labels := []string{"aws", account.AccountID, "global", rtype, bn, s3Prod.Namespace, metricName, codeNames[bn]}
				if needStorageType {
					// BucketName 维度值 = resource_id (bn)，StorageType 维度值 = storageType
					labels = append(labels, bn, storageType)
				} else {
					// BucketName 维度值 = resource_id (bn)，FilterId 维度值 = filterID
					labels = append(labels, bn, filterID)
				}
				// 确保 labels 数量与 GaugeVec 的标签数量匹配
				if len(labels) > count {
					labels = labels[:count]
				} else {
					for len(labels) < count {
						labels = append(labels, "")
					}
				}
				// CloudWatch 返回 float64，scale 统一通过 mappings 注册（若配置了）
				scaled := val * metrics.GetMetricScale(s3Prod.Namespace, metricName)
				vec.WithLabelValues(labels...).Set(scaled)
				metrics.IncSampleCount(s3Prod.Namespace, 1)
				metricsCollected[metricName]++
			}

			// 轻微节流，降低 CloudWatch 压力
			time.Sleep(50 * time.Millisecond)
		}
	}

	// 输出指标收集统计信息
	var collectedList, skippedList []string
	for metricName, count := range metricsCollected {
		if count > 0 {
			collectedList = append(collectedList, fmt.Sprintf("%s(%d buckets)", metricName, count))
		}
	}
	for metricName, count := range metricsSkipped {
		if count > 0 && count == len(buckets) {
			// 如果所有 bucket 都无数据，说明该指标可能未启用或需要前置条件
			skippedList = append(skippedList, metricName)
		}
	}
	if len(collectedList) > 0 {
		logger.Log.Infof("AWS S3 指标收集完成 account=%s: 已收集 %d/%d 个指标，详情: %s",
			account.AccountID, len(collectedList), totalMetricsAttempted, strings.Join(collectedList, ", "))
	}
	if len(skippedList) > 0 {
		logger.Log.Warnf("AWS S3 指标无数据 account=%s: %d 个指标在所有 bucket 上均无数据，可能原因：1) S3 Request Metrics 未启用（需要 FilterId 的指标：%s）；2) 指标尚未产生数据。建议：在 AWS 控制台为 S3 bucket 启用 Request Metrics 功能",
			account.AccountID, len(skippedList), strings.Join(skippedList, ", "))
	}
}

func statForS3Metric(metricName string) string {
	// CloudWatch 口径选择：
	// - 存储/对象数：Average（最新值）
	// - 请求/字节/错误：Sum（区间内累计）
	// - 延迟：Average
	switch metricName {
	case "BucketSizeBytes", "NumberOfObjects":
		return "Average"
	case "FirstByteLatency", "TotalRequestLatency":
		return "Average"
	default:
		return "Sum"
	}
}

func (c *Collector) fetchS3BucketCodeNames(ctx context.Context, client *s3.Client, buckets []string) map[string]string {
	out := make(map[string]string, len(buckets))
	var mu sync.Mutex
	const maxConcurrency = 10
	sem := make(chan struct{}, maxConcurrency)
	var wg sync.WaitGroup

	for _, b := range buckets {
		wg.Add(1)
		go func(bucket string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			reqStart := time.Now()
			var resp *s3.GetBucketTaggingOutput
			var err error
			for attempt := 0; attempt < 3; attempt++ {
				resp, err = client.GetBucketTagging(ctx, &s3.GetBucketTaggingInput{Bucket: aws.String(bucket)})
				if err == nil {
					metrics.RequestTotal.WithLabelValues("aws", "GetBucketTagging", "success").Inc()
					metrics.RecordRequest("aws", "GetBucketTagging", "success")
					metrics.RequestDuration.WithLabelValues("aws", "GetBucketTagging").Observe(time.Since(reqStart).Seconds())
					break
				}
				status := classifyAWSError(err)
				metrics.RequestTotal.WithLabelValues("aws", "GetBucketTagging", status).Inc()
				metrics.RecordRequest("aws", "GetBucketTagging", status)
				if status == "limit_error" {
					metrics.RateLimitTotal.WithLabelValues("aws", "GetBucketTagging").Inc()
				}
				// NoSuchTagSet 是正常情况（bucket 没有标签），不需要重试
				if strings.Contains(err.Error(), "NoSuchTagSet") {
					return
				}
				if status == "auth_error" {
					return
				}
				if attempt < 2 {
					time.Sleep(time.Duration(200*(attempt+1)) * time.Millisecond)
				}
			}
			if err != nil {
				return
			}

			// 检查 resp 和 TagSet 是否为 nil
			if resp == nil || resp.TagSet == nil {
				return
			}

			mu.Lock()
			for _, t := range resp.TagSet {
				if strings.EqualFold(aws.ToString(t.Key), "CodeName") || strings.EqualFold(aws.ToString(t.Key), "code_name") {
					out[bucket] = aws.ToString(t.Value)
					break
				}
			}
			mu.Unlock()
		}(b)
	}
	wg.Wait()
	return out
}

func sanitizeCWQueryID(i int) string {
	// CloudWatch 查询 ID 仅允许小写字母开头 + 字母数字/下划线
	// 这里用 q0,q1... 足够且稳定
	return "q" + strconv.Itoa(i)
}

func classifyAWSError(err error) string {
	msg := err.Error()
	if strings.Contains(msg, "ExpiredToken") || strings.Contains(msg, "InvalidClientTokenId") || strings.Contains(msg, "AccessDenied") {
		return "auth_error"
	}
	if strings.Contains(msg, "Throttling") || strings.Contains(msg, "Rate exceeded") || strings.Contains(msg, "TooManyRequests") {
		return "limit_error"
	}
	if strings.Contains(msg, "timeout") || strings.Contains(msg, "network") {
		return "network_error"
	}
	return "error"
}
