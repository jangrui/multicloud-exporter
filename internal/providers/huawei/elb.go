// 华为云 ELB 采集：枚举负载均衡器并采集 CES 监控指标
package huawei

import (
	"context"
	"time"

	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/logger"
	"multicloud-exporter/internal/metrics"
	providerscommon "multicloud-exporter/internal/providers/common"
	"multicloud-exporter/internal/utils"

	cesmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ces/v1/model"
	elbmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/elb/v3/model"
)

// elbInfo 负载均衡器信息
type elbInfo struct {
	ID   string
	Name string
}

// collectELB 采集 ELB 负载均衡资源
func (h *Collector) collectELB(account config.CloudAccount, region string) {
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

	ctxLog := logger.NewContextLogger("Huawei", "account_id", account.AccountID, "region", region, "rtype", "elb")

	// 产品级分片
	wTotal, wIndex := utils.ClusterConfig()
	for _, p := range prods {
		if p.Namespace != "SYS.ELB" {
			continue
		}
		productKey := account.AccountID + "|" + region + "|" + p.Namespace
		if !utils.ShouldProcess(productKey, wTotal, wIndex) {
			ctxLog.Debugf("ELB 产品跳过（分片不匹配）")
			continue
		}
		elbs := h.listELBInstances(account, region)
		if len(elbs) == 0 {
			continue
		}
		h.fetchELBMonitor(account, region, p, elbs)
	}
}

// listELBInstances 枚举 ELB 实例
func (h *Collector) listELBInstances(account config.CloudAccount, region string) []elbInfo {
	ctxLog := logger.NewContextLogger("Huawei", "account_id", account.AccountID, "region", region, "resource_type", "ELB")

	elbRM := h.getProductRegionManager(HuaweiProductELB)
	if elbRM != nil && elbRM.ShouldSkipRegion(account.AccountID, region) {
		ctxLog.Debugf("ELB 枚举实例 - 区域已跳过（产品级 RegionManager）")
		return []elbInfo{}
	}

	regionKey := account.Provider + ":" + account.AccountID + ":" + region
	if h.degradeMgr != nil && h.degradeMgr.IsDisabled(regionKey, providerscommon.ResourceTypeRegion) {
		ctxLog.Debugf("ELB 枚举实例 - 区域已降级，跳过")
		return []elbInfo{}
	}

	if ids, hit := h.getCachedIDs(account, region, "SYS.ELB", "elb"); hit {
		var elbs []elbInfo
		for _, id := range ids {
			elbs = append(elbs, elbInfo{ID: id, Name: id})
		}
		ctxLog.Debugf("ELB 枚举实例 - 缓存命中 - 数量=%d", len(ids))
		return elbs
	}

	client, err := h.clientFactory.NewELBClient(region, account.AccessKeyID, account.AccessKeySecret)
	if err != nil {
		if h.degradeMgr != nil {
			h.degradeMgr.RecordFailure(regionKey, providerscommon.ResourceTypeRegion, err.Error())
		}
		ctxLog.Errorf("ELB 枚举实例 - 客户端创建失败 - 错误=%v", err)
		return nil
	}

	ctxLog.Debugf("ELB 枚举实例 - API: ListLoadBalancers - 开始枚举")

	var elbs []elbInfo
	limit := int32(100)
	var marker *string

	for {
		req := &elbmodel.ListLoadBalancersRequest{
			Limit:  &limit,
			Marker: marker,
		}

		start := time.Now()
		var resp *elbmodel.ListLoadBalancersResponse

		// 使用通用重试机制
		retryConfig := providerscommon.DefaultRetryConfig()
		shouldRetry := providerscommon.ShouldRetryForLimitError(providerscommon.HuaweiClassifier)

		callErr := providerscommon.RetryWithBackoff(context.TODO(), retryConfig, func() error {
			var err error
			resp, err = client.ListLoadBalancers(req)

			// 记录指标
			if err != nil {
				status := providerscommon.ClassifyHuaweiError(err)
				metrics.RequestTotal.WithLabelValues("huawei", "ListLoadBalancers", status).Inc()
				metrics.RecordRequest("huawei", "ListLoadBalancers", status)
				if status == providerscommon.ErrorStatusLimit {
					metrics.RateLimitTotal.WithLabelValues("huawei", "ListLoadBalancers").Inc()
					ctxLog.Warnf("ELB 枚举实例 - API: ListLoadBalancers - 限流，将重试")
				}
				return err
			}

			metrics.RequestTotal.WithLabelValues("huawei", "ListLoadBalancers", "success").Inc()
			metrics.RecordRequest("huawei", "ListLoadBalancers", "success")
			metrics.RequestDuration.WithLabelValues("huawei", "ListLoadBalancers").Observe(time.Since(start).Seconds())
			if h.degradeMgr != nil {
				h.degradeMgr.RecordSuccess(regionKey, providerscommon.ResourceTypeRegion)
			}
			return nil
		}, shouldRetry)

		if callErr != nil {
			if h.degradeMgr != nil {
				if providerscommon.ClassifyHuaweiError(callErr) == providerscommon.ErrorStatusAuth {
					disabled := h.degradeMgr.RecordFailure(regionKey, providerscommon.ResourceTypeRegion, callErr.Error())
					if disabled {
						ctxLog.Warn("区域已降级")
					}
				} else {
					h.degradeMgr.RecordFailure(regionKey, providerscommon.ResourceTypeRegion, callErr.Error())
				}
			}
			ctxLog.Warnf("ELB 枚举实例 - API: ListLoadBalancers - 失败: %v", callErr)
			break
		}

		if resp == nil || resp.Loadbalancers == nil {
			break
		}

		for _, lb := range *resp.Loadbalancers {
			name := lb.Id
			if lb.Name != "" {
				name = lb.Name
			}
			elbs = append(elbs, elbInfo{ID: lb.Id, Name: name})
		}

		// 检查分页
		if resp.PageInfo == nil || resp.PageInfo.NextMarker == nil || *resp.PageInfo.NextMarker == "" {
			break
		}
		marker = resp.PageInfo.NextMarker
		time.Sleep(50 * time.Millisecond)
	}

	// 缓存 ID 列表
	var ids []string
	for _, elb := range elbs {
		ids = append(ids, elb.ID)
	}

	wTotal, wIndex := utils.ClusterConfig()
	if wTotal > 1 {
		filteredIDs := []string{}
		for _, id := range ids {
			instanceKey := account.AccountID + "|" + region + "|" + "SYS.ELB" + "|" + id
			if utils.ShouldProcess(instanceKey, wTotal, wIndex) {
				filteredIDs = append(filteredIDs, id)
			}
		}
		ids = filteredIDs
		filteredELBs := []elbInfo{}
		for _, elb := range elbs {
			for _, id := range ids {
				if elb.ID == id {
					filteredELBs = append(filteredELBs, elb)
					break
				}
			}
		}
		elbs = filteredELBs
	}
	h.setCachedIDs(account, region, "SYS.ELB", "elb", ids)

	// 更新产品级区域状态
	if elbRM != nil {
		status := providerscommon.RegionStatusEmpty
		if len(ids) > 0 {
			status = providerscommon.RegionStatusActive
		}
		elbRM.UpdateRegionStatus(account.AccountID, region, len(ids), status)
		ctxLog.Debugf("更新 ELB 区域状态，status=%s，count=%d",
			status, len(ids))
	}

	if len(elbs) > 0 {
		max := 5
		if len(elbs) < max {
			max = len(elbs)
		}
		var preview []string
		for i := 0; i < max; i++ {
			preview = append(preview, elbs[i].ID)
		}
		ctxLog.Debugf("ELB 已枚举，数量=%d 预览=%v", len(elbs), preview)
	} else {
		ctxLog.Debugf("ELB 已枚举，数量=%d", len(elbs))
	}
	return elbs
}

// fetchELBMonitor 采集 ELB 监控指标
func (h *Collector) fetchELBMonitor(account config.CloudAccount, region string, prod config.Product, elbs []elbInfo) {
	ctxLog := logger.NewContextLogger("Huawei", "account_id", account.AccountID, "region", region, "rtype", "elb")

	regionKey := account.Provider + ":" + account.AccountID + ":" + region
	if h.degradeMgr != nil && h.degradeMgr.IsDisabled(regionKey, providerscommon.ResourceTypeRegion) {
		ctxLog.Debugf("ELB 监控采集 - 区域已降级，跳过")
		return
	}

	client, err := h.clientFactory.NewCESClient(region, account.AccessKeyID, account.AccessKeySecret)
	if err != nil {
		if h.degradeMgr != nil {
			h.degradeMgr.RecordFailure(regionKey, providerscommon.ResourceTypeRegion, err.Error())
		}
		ctxLog.Errorf("CES 客户端创建失败，错误=%v", err)
		return
	}

	period := int32(300) // 默认 5 分钟
	if prod.Period != nil {
		period = int32(*prod.Period)
	}

	// 批量查询指标，每批最多 10 个资源
	batchSize := 10

	for _, group := range prod.MetricInfo {
		if group.Period != nil {
			period = int32(*group.Period)
		}
		for _, metricName := range group.MetricList {
			for i := 0; i < len(elbs); i += batchSize {
				end := i + batchSize
				if end > len(elbs) {
					end = len(elbs)
				}
				batch := elbs[i:end]

				var metricInfos []cesmodel.MetricInfo
				for _, elb := range batch {
					metricInfos = append(metricInfos, cesmodel.MetricInfo{
						Namespace:  "SYS.ELB",
						MetricName: metricName,
						Dimensions: []cesmodel.MetricsDimension{
							{Name: "lbaas_instance_id", Value: elb.ID},
						},
					})
				}

				now := time.Now()
				// 使用更大的时间窗口确保数据可用
				startT := now.Add(-time.Duration(period*2) * time.Second)
				endT := now.Add(-time.Duration(period) * time.Second)

				fromT := startT.UnixMilli()
				toT := endT.UnixMilli()
				periodStr := "1"
				if period >= 300 {
					periodStr = "300"
				} else if period >= 60 {
					periodStr = "1"
				}

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
				var resp *cesmodel.BatchListMetricDataResponse

				// 使用通用重试机制
				retryConfig := providerscommon.DefaultRetryConfig()
				shouldRetry := providerscommon.ShouldRetryForLimitError(providerscommon.HuaweiClassifier)

				err := providerscommon.RetryWithBackoff(context.TODO(), retryConfig, func() error {
					var apiErr error
					resp, apiErr = client.BatchListMetricData(req)

					// 记录指标
					if apiErr != nil {
						status := providerscommon.ClassifyHuaweiError(apiErr)
						metrics.RequestTotal.WithLabelValues("huawei", "BatchListMetricData", status).Inc()
						metrics.RecordRequest("huawei", "BatchListMetricData", status)
						if status == providerscommon.ErrorStatusLimit {
							metrics.RateLimitTotal.WithLabelValues("huawei", "BatchListMetricData").Inc()
							ctxLog.Warnf("ELB BatchListMetricData 限流，将重试")
						}
						return apiErr
					}

					metrics.RequestTotal.WithLabelValues("huawei", "BatchListMetricData", "success").Inc()
					metrics.RecordRequest("huawei", "BatchListMetricData", "success")
					metrics.RequestDuration.WithLabelValues("huawei", "BatchListMetricData").Observe(time.Since(reqStart).Seconds())
					if h.degradeMgr != nil {
						h.degradeMgr.RecordSuccess(regionKey, providerscommon.ResourceTypeRegion)
					}
					return nil
				}, shouldRetry)

				if err != nil {
					if h.degradeMgr != nil {
						if providerscommon.ClassifyHuaweiError(err) == providerscommon.ErrorStatusAuth {
							disabled := h.degradeMgr.RecordFailure(regionKey, providerscommon.ResourceTypeRegion, err.Error())
							if disabled {
								ctxLog.Warn("区域已降级")
							}
						} else {
							h.degradeMgr.RecordFailure(regionKey, providerscommon.ResourceTypeRegion, err.Error())
						}
					}
					ctxLog.Warnf("BatchListMetricData 错误，指标=%s 错误=%v", metricName, err)
					continue
				}

				if resp == nil || resp.Metrics == nil || len(*resp.Metrics) == 0 {
					continue
				}

				// 构建 ELB ID 到名称的映射
				elbNameMap := make(map[string]string)
				for _, elb := range batch {
					elbNameMap[elb.ID] = elb.Name
				}

				for _, metricData := range *resp.Metrics {
					if len(metricData.Datapoints) == 0 {
						continue
					}

					// 获取资源 ID
					var resourceID string
					if metricData.Dimensions != nil {
						for _, dim := range *metricData.Dimensions {
							if dim.Name == "lbaas_instance_id" {
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
						rtype = "clb"
					}

					codeName := elbNameMap[resourceID]
					if codeName == "" {
						codeName = resourceID
					}

					labels := []string{"huawei", account.AccountID, region, rtype, resourceID, prod.Namespace, metricName, codeName}
					for len(labels) < count {
						labels = append(labels, "")
					}
					vec.WithLabelValues(labels...).Set(val)
					metrics.IncSampleCount(prod.Namespace, 1)
				}

				// 华为云 API 限流控制：300 次/分钟
				// 为避免触发限流，在每次批量请求后添加延迟
				// 计算：300 次/分钟 = 5 次/秒，安全起见使用 250ms 延迟（4 次/秒）
				time.Sleep(250 * time.Millisecond)
			}
		}
	}
}
