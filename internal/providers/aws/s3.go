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

	period := int32(86400) // S3 存储类指标通常以天为粒度
	if s3Prod.Period != nil && *s3Prod.Period > 0 {
		period = int32(*s3Prod.Period)
	}

	for _, group := range s3Prod.MetricInfo {
		localPeriod := period
		if group.Period != nil && *group.Period > 0 {
			localPeriod = int32(*group.Period)
		}
		for _, m := range group.MetricList {
			metricName := strings.TrimSpace(m)
			if metricName == "" {
				continue
			}
			// 针对存储类指标，默认取 StandardStorage；后续可扩展为多 StorageType。
			needStorageType := metricName == "BucketSizeBytes" || metricName == "NumberOfObjects"
			storageType := "StandardStorage"
			if metricName == "NumberOfObjects" {
				storageType = "AllStorageTypes"
			}
			queries := make([]cwtypes.MetricDataQuery, 0, len(buckets))
			ids := make([]string, 0, len(buckets))
			for i, bn := range buckets {
				id := sanitizeCWQueryID(i)
				ids = append(ids, id)
				dims := []cwtypes.Dimension{{Name: aws.String("BucketName"), Value: aws.String(bn)}}
				if needStorageType {
					dims = append(dims, cwtypes.Dimension{Name: aws.String("StorageType"), Value: aws.String(storageType)})
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
						Stat:   aws.String("Average"),
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
			for i, bn := range buckets {
				id := ids[i]
				r, ok := results[id]
				if !ok || len(r.Values) == 0 {
					continue
				}
				val := r.Values[0]
				vec, count := metrics.NamespaceGauge(s3Prod.Namespace, metricName, "BucketName", "StorageType")
				rtype := metrics.GetNamespacePrefix(s3Prod.Namespace)
				if rtype == "" {
					rtype = "s3"
				}
				labels := []string{"aws", account.AccountID, "global", rtype, bn, s3Prod.Namespace, metricName, codeNames[bn], bn, storageType}
				for len(labels) < count {
					labels = append(labels, "")
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
