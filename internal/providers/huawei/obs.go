// 华为云 OBS 采集：枚举存储桶并采集 CES 监控指标
package huawei

import (
	"strings"
	"time"

	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/logger"
	"multicloud-exporter/internal/metrics"
	_ "multicloud-exporter/internal/metrics/huawei" // 注册指标别名
	providerscommon "multicloud-exporter/internal/providers/common"
	"multicloud-exporter/internal/utils"

	"github.com/huaweicloud/huaweicloud-sdk-go-obs/obs"
	cesmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ces/v1/model"
)

// obsInfo 存储桶信息
type obsInfo struct {
	Name     string
	Location string
}

// collectOBS 采集 OBS 存储桶资源
func (h *Collector) collectOBS(account config.CloudAccount, region string) {
	if h.cfg == nil {
		return
	}
	var prods []config.Product
	if h.disc != nil {
		if ps, ok := h.disc.Get()["huawei"]; ok && len(ps) > 0 {
			prods = ps
		}
	}
	if len(prods) == 0 {
		return
	}

	// 产品级分片
	wTotal, wIndex := utils.ClusterConfig()
	for _, p := range prods {
		if p.Namespace != "SYS.OBS" {
			continue
		}
		productKey := account.AccountID + "|" + region + "|" + p.Namespace
		if !utils.ShouldProcess(productKey, wTotal, wIndex) {
			logger.Log.Debugf("Huawei OBS 产品跳过（分片不匹配）account=%s region=%s namespace=%s", account.AccountID, region, p.Namespace)
			continue
		}
		buckets := h.listOBSBuckets(account, region)
		if len(buckets) == 0 {
			continue
		}
		h.fetchOBSMonitor(account, region, p, buckets)
	}
}

// listOBSBuckets 枚举 OBS 存储桶
func (h *Collector) listOBSBuckets(account config.CloudAccount, region string) []obsInfo {
	if ids, hit := h.getCachedIDs(account, region, "SYS.OBS", "obs"); hit {
		var buckets []obsInfo
		for _, id := range ids {
			buckets = append(buckets, obsInfo{Name: id, Location: region})
		}
		logger.Log.Debugf("Huawei OBS 缓存命中，账号ID=%s 区域=%s 数量=%d", account.AccountID, region, len(ids))
		return buckets
	}

	client, err := h.clientFactory.NewOBSClient(region, account.AccessKeyID, account.AccessKeySecret)
	if err != nil {
		logger.Log.Errorf("Huawei OBS 客户端创建失败，区域=%s 错误=%v", region, err)
		return nil
	}
	defer client.Close()

	ctxLog := logger.NewContextLogger("Huawei", "account_id", account.AccountID, "region", region, "rtype", "obs")
	ctxLog.Debugf("开始枚举 OBS 存储桶")

	start := time.Now()
	var output *obs.ListBucketsOutput
	var callErr error
	for attempt := 0; attempt < 3; attempt++ {
		output, callErr = client.ListBuckets(&obs.ListBucketsInput{QueryLocation: true})
		if callErr == nil {
			metrics.RequestTotal.WithLabelValues("huawei", "ListBuckets", "success").Inc()
			metrics.RecordRequest("huawei", "ListBuckets", "success")
			metrics.RequestDuration.WithLabelValues("huawei", "ListBuckets").Observe(time.Since(start).Seconds())
			break
		}
		status := providerscommon.ClassifyHuaweiError(callErr)
		metrics.RequestTotal.WithLabelValues("huawei", "ListBuckets", status).Inc()
		metrics.RecordRequest("huawei", "ListBuckets", status)
		if status == "limit_error" {
			metrics.RateLimitTotal.WithLabelValues("huawei", "ListBuckets").Inc()
		}
		if status == "auth_error" {
			return nil
		}
		// 指数退避重试
		sleep := time.Duration(200*(1<<attempt)) * time.Millisecond
		if sleep > 5*time.Second {
			sleep = 5 * time.Second
		}
		time.Sleep(sleep)
	}
	if callErr != nil {
		ctxLog.Warnf("OBS ListBuckets 失败: %v", callErr)
		return nil
	}

	if output == nil || output.Buckets == nil {
		return nil
	}

	// 诊断日志：打印 API 返回的原始存储桶信息
	totalBuckets := len(output.Buckets)
	if totalBuckets > 0 {
		// 收集所有唯一的 Location 用于诊断
		locationSet := make(map[string]struct{})
		for _, bucket := range output.Buckets {
			locationSet[bucket.Location] = struct{}{}
		}
		var locations []string
		for loc := range locationSet {
			locations = append(locations, loc)
		}
		ctxLog.Debugf("OBS ListBuckets 返回，总数=%d 区域列表=%v 当前区域=%s", totalBuckets, locations, region)
	} else {
		ctxLog.Debugf("OBS ListBuckets 返回，总数=0（账号无存储桶）")
	}

	var buckets []obsInfo
	for _, bucket := range output.Buckets {
		// 只处理当前区域的存储桶（大小写不敏感比较）
		if strings.EqualFold(bucket.Location, region) {
			buckets = append(buckets, obsInfo{Name: bucket.Name, Location: bucket.Location})
		}
	}

	// 缓存 ID 列表
	var ids []string
	for _, bucket := range buckets {
		ids = append(ids, bucket.Name)
	}
	h.setCachedIDs(account, region, "SYS.OBS", "obs", ids)

	// 更新区域状态
	if h.regionManager != nil {
		status := providerscommon.RegionStatusEmpty
		if len(ids) > 0 {
			status = providerscommon.RegionStatusActive
		}
		h.regionManager.UpdateRegionStatus(account.AccountID, region, len(ids), status)
		ctxLog.Debugf("更新区域状态 account=%s region=%s status=%s count=%d",
			account.AccountID, region, status, len(ids))
	}

	if len(buckets) > 0 {
		max := 5
		if len(buckets) < max {
			max = len(buckets)
		}
		var preview []string
		for i := 0; i < max; i++ {
			preview = append(preview, buckets[i].Name)
		}
		logger.Log.Debugf("Huawei OBS 已枚举，账号ID=%s 区域=%s 数量=%d 预览=%v", account.AccountID, region, len(buckets), preview)
	} else {
		logger.Log.Debugf("Huawei OBS 已枚举，账号ID=%s 区域=%s 数量=%d", account.AccountID, region, len(buckets))
	}
	return buckets
}

// isOBSCapacityMetric 判断是否为容量类指标（每日更新）
// 华为云 OBS 容量类指标每日更新一次，需要使用更长的 Period 和时间窗口
func isOBSCapacityMetric(metricName string) bool {
	return strings.HasPrefix(metricName, "capacity_") ||
		strings.HasPrefix(metricName, "object_num_")
}

// fetchOBSMonitor 采集 OBS 监控指标
func (h *Collector) fetchOBSMonitor(account config.CloudAccount, region string, prod config.Product, buckets []obsInfo) {
	client, err := h.clientFactory.NewCESClient(region, account.AccessKeyID, account.AccessKeySecret)
	if err != nil {
		logger.Log.Errorf("Huawei CES 客户端创建失败，区域=%s 错误=%v", region, err)
		return
	}

	ctxLog := logger.NewContextLogger("Huawei", "account_id", account.AccountID, "region", region, "rtype", "obs")

	basePeriod := int32(300) // 默认 5 分钟
	if prod.Period != nil {
		basePeriod = int32(*prod.Period)
	}

	// 批量查询指标，每批最多 10 个资源
	batchSize := 10

	for _, group := range prod.MetricInfo {
		groupPeriod := basePeriod
		if group.Period != nil {
			groupPeriod = int32(*group.Period)
		}
		for _, metricName := range group.MetricList {
			// 跳过需要多维度的指标（如 api_name, http_code）
			if strings.Contains(metricName, "api_request_count_per_second") ||
				strings.Contains(metricName, "request_code_count") {
				continue
			}

			// 根据指标类型设置不同的 Period 和时间窗口
			var period int32
			var periodStr string
			var startT, endT time.Time
			now := time.Now()

			if isOBSCapacityMetric(metricName) {
				// 容量类指标：每日更新，使用 86400 秒 Period，时间窗口回溯 48 小时
				period = 86400
				periodStr = "86400"
				startT = now.Add(-48 * time.Hour)
				endT = now
			} else {
				// 请求类指标：使用配置的 Period，时间窗口回溯 2 个周期
				period = groupPeriod
				startT = now.Add(-time.Duration(period*2) * time.Second)
				endT = now.Add(-time.Duration(period) * time.Second)
				if period >= 300 {
					periodStr = "300"
				} else if period >= 60 {
					periodStr = "1"
				} else {
					periodStr = "1"
				}
			}

			for i := 0; i < len(buckets); i += batchSize {
				end := i + batchSize
				if end > len(buckets) {
					end = len(buckets)
				}
				batch := buckets[i:end]

				var metricInfos []cesmodel.MetricInfo
				for _, bucket := range batch {
					metricInfos = append(metricInfos, cesmodel.MetricInfo{
						Namespace:  "SYS.OBS",
						MetricName: metricName,
						Dimensions: []cesmodel.MetricsDimension{
							{Name: "bucket_name", Value: bucket.Name},
						},
					})
				}

				fromT := startT.UnixMilli()
				toT := endT.UnixMilli()

				req := &cesmodel.BatchListMetricDataRequest{
					Body: &cesmodel.BatchListMetricDataRequestBody{
						Metrics: metricInfos,
						From:    fromT,
						To:      toT,
						Period:  periodStr,
						Filter:  "average",
					},
				}

				reqStart := time.Now()
				resp, err := client.BatchListMetricData(req)
				if err != nil {
					status := providerscommon.ClassifyHuaweiError(err)
					metrics.RequestTotal.WithLabelValues("huawei", "BatchListMetricData", status).Inc()
					metrics.RecordRequest("huawei", "BatchListMetricData", status)
					if status == "limit_error" {
						metrics.RateLimitTotal.WithLabelValues("huawei", "BatchListMetricData").Inc()
					}
					ctxLog.Warnf("OBS BatchListMetricData 错误，指标=%s period=%s 错误=%v", metricName, periodStr, err)
					continue
				}
				metrics.RequestTotal.WithLabelValues("huawei", "BatchListMetricData", "success").Inc()
				metrics.RecordRequest("huawei", "BatchListMetricData", "success")
				metrics.RequestDuration.WithLabelValues("huawei", "BatchListMetricData").Observe(time.Since(reqStart).Seconds())

				if resp == nil || resp.Metrics == nil || len(*resp.Metrics) == 0 {
					ctxLog.Debugf("OBS BatchListMetricData 无数据，指标=%s period=%s", metricName, periodStr)
					continue
				}

				ctxLog.Debugf("OBS BatchListMetricData 返回，指标=%s period=%s 数据条数=%d", metricName, periodStr, len(*resp.Metrics))

				for _, metricData := range *resp.Metrics {
					if len(metricData.Datapoints) == 0 {
						// 获取 bucket 名称用于日志
						bucketName := ""
						if metricData.Dimensions != nil {
							for _, dim := range *metricData.Dimensions {
								if dim.Name == "bucket_name" {
									bucketName = dim.Value
									break
								}
							}
						}
						ctxLog.Debugf("OBS 指标无数据点，指标=%s bucket=%s period=%s", metricName, bucketName, periodStr)
						continue
					}

					// 获取资源 ID（bucket_name）
					var resourceID string
					if metricData.Dimensions != nil {
						for _, dim := range *metricData.Dimensions {
							if dim.Name == "bucket_name" {
								resourceID = dim.Value
								break
							}
						}
					}
					if resourceID == "" {
						continue
					}

					// 获取最新数据点
					datapoints := metricData.Datapoints
					lastPoint := datapoints[len(datapoints)-1]
					var val float64
					if lastPoint.Average != nil {
						val = *lastPoint.Average
					}

					vec, count := metrics.NamespaceGauge(prod.Namespace, metricName)
					rtype := metrics.GetNamespacePrefix(prod.Namespace)
					if rtype == "" {
						rtype = "s3"
					}

					labels := []string{"huawei", account.AccountID, region, rtype, resourceID, prod.Namespace, metricName, resourceID}
					for len(labels) < count {
						labels = append(labels, "")
					}
					vec.WithLabelValues(labels...).Set(val)
					metrics.IncSampleCount(prod.Namespace, 1)

					ctxLog.Debugf("OBS 暴露指标，指标=%s bucket=%s period=%s 值=%.2f", metricName, resourceID, periodStr, val)
				}

				time.Sleep(50 * time.Millisecond)
			}
		}
	}
}
