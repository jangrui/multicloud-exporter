package aws

import (
	"context"
	"strings"
	"time"

	"multicloud-exporter/internal/config"
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
		return
	}
	start := time.Now()
	bucketsOut, err := s3Client.ListBuckets(ctx, &s3.ListBucketsInput{})
	if err != nil {
		status := classifyAWSError(err)
		metrics.RequestTotal.WithLabelValues("aws", "ListBuckets", status).Inc()
		metrics.RecordRequest("aws", "ListBuckets", status)
		if status == "limit_error" {
			metrics.RateLimitTotal.WithLabelValues("aws", "ListBuckets").Inc()
		}
		return
	}
	metrics.RequestTotal.WithLabelValues("aws", "ListBuckets", "success").Inc()
	metrics.RecordRequest("aws", "ListBuckets", "success")
	metrics.RequestDuration.WithLabelValues("aws", "ListBuckets").Observe(time.Since(start).Seconds())

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
		return
	}

	defaultPeriod := int32(86400) // 默认按存储类（天粒度）回退
	if s3Prod.Period != nil && *s3Prod.Period > 0 {
		defaultPeriod = int32(*s3Prod.Period)
	}

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
			needStorageType := metricName == "BucketSizeBytes" || metricName == "NumberOfObjects"
			needFilterID := !needStorageType
			storageType := "StandardStorage"
			filterID := "EntireBucket"
			if metricName == "NumberOfObjects" {
				storageType = "AllStorageTypes"
			}
			stat := statForS3Metric(metricName)
			queries := make([]cwtypes.MetricDataQuery, 0, len(buckets))
			ids := make([]string, 0, len(buckets))
			for i, bn := range buckets {
				id := sanitizeCWQueryID(i)
				ids = append(ids, id)
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

			// 取最近 2 个周期窗口，避免无数据点
			end := time.Now().UTC()
			startTime := end.Add(-2 * time.Duration(localPeriod) * time.Second)
			reqStart := time.Now()
			resp, err := cwClient.GetMetricData(ctx, &cloudwatch.GetMetricDataInput{
				StartTime:         aws.Time(startTime),
				EndTime:           aws.Time(end),
				MetricDataQueries: queries,
				ScanBy:            cwtypes.ScanByTimestampDescending,
			})
			if err != nil {
				status := classifyAWSError(err)
				metrics.RequestTotal.WithLabelValues("aws", "GetMetricData", status).Inc()
				metrics.RecordRequest("aws", "GetMetricData", status)
				if status == "limit_error" {
					metrics.RateLimitTotal.WithLabelValues("aws", "GetMetricData").Inc()
				}
				continue
			}
			metrics.RequestTotal.WithLabelValues("aws", "GetMetricData", "success").Inc()
			metrics.RecordRequest("aws", "GetMetricData", "success")
			metrics.RequestDuration.WithLabelValues("aws", "GetMetricData").Observe(time.Since(reqStart).Seconds())

			// 将每个 query 的最新值写入指标
			results := make(map[string]cwtypes.MetricDataResult, len(resp.MetricDataResults))
			for _, r := range resp.MetricDataResults {
				if r.Id != nil {
					results[*r.Id] = r
				}
			}
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
				id := ids[i]
				r, ok := results[id]
				if !ok || len(r.Values) == 0 {
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
			}

			// 轻微节流，降低 CloudWatch 压力
			time.Sleep(50 * time.Millisecond)
		}
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
	for _, b := range buckets {
		reqStart := time.Now()
		resp, err := client.GetBucketTagging(ctx, &s3.GetBucketTaggingInput{Bucket: aws.String(b)})
		if err != nil {
			status := classifyAWSError(err)
			metrics.RequestTotal.WithLabelValues("aws", "GetBucketTagging", status).Inc()
			metrics.RecordRequest("aws", "GetBucketTagging", status)
			if status == "limit_error" {
				metrics.RateLimitTotal.WithLabelValues("aws", "GetBucketTagging").Inc()
			}
			continue
		}
		metrics.RequestTotal.WithLabelValues("aws", "GetBucketTagging", "success").Inc()
		metrics.RecordRequest("aws", "GetBucketTagging", "success")
		metrics.RequestDuration.WithLabelValues("aws", "GetBucketTagging").Observe(time.Since(reqStart).Seconds())
		for _, t := range resp.TagSet {
			if strings.EqualFold(aws.ToString(t.Key), "CodeName") || strings.EqualFold(aws.ToString(t.Key), "code_name") {
				out[b] = aws.ToString(t.Value)
				break
			}
		}
	}
	return out
}

func sanitizeCWQueryID(i int) string {
	// CloudWatch 查询 ID 仅允许小写字母开头 + 字母数字/下划线
	// 这里用 q0,q1... 足够且稳定
	return "q" + strconvItoa(i)
}

func strconvItoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := false
	if i < 0 {
		neg = true
		i = -i
	}
	var buf [32]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
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
