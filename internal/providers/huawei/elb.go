// 华为云 ELB 采集：枚举负载均衡器并采集 CES 监控指标
package huawei

import (
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

	// 产品级分片
	wTotal, wIndex := utils.ClusterConfig()
	for _, p := range prods {
		if p.Namespace != "SYS.ELB" {
			continue
		}
		productKey := account.AccountID + "|" + region + "|" + p.Namespace
		if !utils.ShouldProcess(productKey, wTotal, wIndex) {
			logger.Log.Debugf("Huawei ELB 产品跳过（分片不匹配）account=%s region=%s namespace=%s", account.AccountID, region, p.Namespace)
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
	if ids, hit := h.getCachedIDs(account, region, "SYS.ELB", "elb"); hit {
		var elbs []elbInfo
		for _, id := range ids {
			elbs = append(elbs, elbInfo{ID: id, Name: id})
		}
		logger.Log.Debugf("Huawei ELB 缓存命中，账号ID=%s 区域=%s 数量=%d", account.AccountID, region, len(ids))
		return elbs
	}

	client, err := h.clientFactory.NewELBClient(region, account.AccessKeyID, account.AccessKeySecret)
	if err != nil {
		logger.Log.Errorf("Huawei ELB 客户端创建失败，区域=%s 错误=%v", region, err)
		return nil
	}

	ctxLog := logger.NewContextLogger("Huawei", "account_id", account.AccountID, "region", region, "rtype", "elb")
	ctxLog.Debugf("开始枚举 ELB 实例")

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
		var callErr error
		for attempt := 0; attempt < 3; attempt++ {
			resp, callErr = client.ListLoadBalancers(req)
			if callErr == nil {
				metrics.RequestTotal.WithLabelValues("huawei", "ListLoadBalancers", "success").Inc()
				metrics.RecordRequest("huawei", "ListLoadBalancers", "success")
				metrics.RequestDuration.WithLabelValues("huawei", "ListLoadBalancers").Observe(time.Since(start).Seconds())
				break
			}
			status := providerscommon.ClassifyHuaweiError(callErr)
			metrics.RequestTotal.WithLabelValues("huawei", "ListLoadBalancers", status).Inc()
			metrics.RecordRequest("huawei", "ListLoadBalancers", status)
			if status == "limit_error" {
				metrics.RateLimitTotal.WithLabelValues("huawei", "ListLoadBalancers").Inc()
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
			ctxLog.Warnf("ELB ListLoadBalancers 失败: %v", callErr)
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
	h.setCachedIDs(account, region, "SYS.ELB", "elb", ids)

	// 更新区域状态
	if h.regionManager != nil {
		status := providerscommon.RegionStatusEmpty
		if len(ids) > 0 {
			status = providerscommon.RegionStatusActive
		}
		h.regionManager.UpdateRegionStatus(account.AccountID, region, len(ids), status)
		logger.Log.Debugf("更新区域状态 account=%s region=%s status=%s count=%d",
			account.AccountID, region, status, len(ids))
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
		logger.Log.Debugf("Huawei ELB 已枚举，账号ID=%s 区域=%s 数量=%d 预览=%v", account.AccountID, region, len(elbs), preview)
	} else {
		logger.Log.Debugf("Huawei ELB 已枚举，账号ID=%s 区域=%s 数量=%d", account.AccountID, region, len(elbs))
	}
	return elbs
}

// fetchELBMonitor 采集 ELB 监控指标
func (h *Collector) fetchELBMonitor(account config.CloudAccount, region string, prod config.Product, elbs []elbInfo) {
	client, err := h.clientFactory.NewCESClient(region, account.AccessKeyID, account.AccessKeySecret)
	if err != nil {
		logger.Log.Errorf("Huawei CES 客户端创建失败，区域=%s 错误=%v", region, err)
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
				resp, err := client.BatchListMetricData(req)
				if err != nil {
					status := providerscommon.ClassifyHuaweiError(err)
					metrics.RequestTotal.WithLabelValues("huawei", "BatchListMetricData", status).Inc()
					metrics.RecordRequest("huawei", "BatchListMetricData", status)
					if status == "limit_error" {
						metrics.RateLimitTotal.WithLabelValues("huawei", "BatchListMetricData").Inc()
					}
					logger.Log.Warnf("BatchListMetricData 错误，指标=%s 错误=%v", metricName, err)
					continue
				}
				metrics.RequestTotal.WithLabelValues("huawei", "BatchListMetricData", "success").Inc()
				metrics.RecordRequest("huawei", "BatchListMetricData", "success")
				metrics.RequestDuration.WithLabelValues("huawei", "BatchListMetricData").Observe(time.Since(reqStart).Seconds())

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

				time.Sleep(50 * time.Millisecond)
			}
		}
	}
}
