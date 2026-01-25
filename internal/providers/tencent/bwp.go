package tencent

import (
	"context"
	"time"

	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/logger"
	"multicloud-exporter/internal/metrics"
	providerscommon "multicloud-exporter/internal/providers/common"

	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	monitor "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/monitor/v20180724"
	vpc "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/vpc/v20170312"
)

func (t *Collector) listBWPIDs(account config.CloudAccount, region string) []string {
	ctxLog := logger.NewContextLogger("Tencent", "account_id", account.AccountID, "region", region, "resource_type", "BWP")
	bwpRM := t.getProductRegionManager(TencentProductBWP)

	regionKey := account.Provider + ":" + account.AccountID + ":" + region
	if t.degradeMgr != nil && t.degradeMgr.IsDisabled(regionKey, providerscommon.ResourceTypeRegion) {
		ctxLog.Debugf("BWP 枚举带宽包 - 区域已降级，跳过")
		return []string{}
	}

	if ids, hit := t.getCachedIDs(account, region, "QCE/BWP", "bwp"); hit {
		ctxLog.Debugf("BWP 枚举带宽包 - 缓存命中 - 数量=%d", len(ids))
		return ids
	}

	if bwpRM != nil && bwpRM.ShouldSkipRegion(account.AccountID, region) {
		ctxLog.Debugf("BWP 枚举带宽包 - 区域已跳过（空区域）")
		return []string{}
	}

	client, err := t.clientFactory.NewVPCClient(region, account.AccessKeyID, account.AccessKeySecret)
	if err != nil {
		if t.degradeMgr != nil {
			t.degradeMgr.RecordFailure(regionKey, providerscommon.ResourceTypeRegion, err.Error())
		}
		return []string{}
	}

	ctxLog.Debugf("BWP 枚举带宽包 - API: DescribeBandwidthPackages - 开始枚举")

	var ids []string
	limit := uint64(100)
	offset := uint64(0)

	// 使用通用重试配置
	retryConfig := providerscommon.DefaultRetryConfig()
	shouldRetry := providerscommon.ShouldRetryForLimitError(providerscommon.TencentClassifier)

	for {
		req := vpc.NewDescribeBandwidthPackagesRequest()
		req.Limit = common.Uint64Ptr(limit)
		req.Offset = common.Uint64Ptr(offset)

		start := time.Now()
		var resp *vpc.DescribeBandwidthPackagesResponse

		// 使用带超时的上下文
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		callErr := providerscommon.RetryWithBackoff(ctx, retryConfig, func() error {
			var err error
			resp, err = client.DescribeBandwidthPackages(req)

			// 记录指标
			if err != nil {
				status := providerscommon.ClassifyTencentError(err)
				metrics.RequestTotal.WithLabelValues("tencent", "DescribeBandwidthPackages", status).Inc()
				metrics.RecordRequest("tencent", "DescribeBandwidthPackages", status)
				if status == providerscommon.ErrorStatusLimit {
					metrics.RateLimitTotal.WithLabelValues("tencent", "DescribeBandwidthPackages").Inc()
				}
				return err
			}

			metrics.RequestTotal.WithLabelValues("tencent", "DescribeBandwidthPackages", "success").Inc()
			metrics.RecordRequest("tencent", "DescribeBandwidthPackages", "success")
			metrics.RequestDuration.WithLabelValues("tencent", "DescribeBandwidthPackages").Observe(time.Since(start).Seconds())
			if t.degradeMgr != nil {
				t.degradeMgr.RecordSuccess(regionKey, providerscommon.ResourceTypeRegion)
			}
			return nil
		}, shouldRetry)
		cancel()

		if callErr != nil {
			if providerscommon.ClassifyTencentError(callErr) == providerscommon.ErrorStatusAuth {
				if t.degradeMgr != nil {
					disabled := t.degradeMgr.RecordFailure(regionKey, providerscommon.ResourceTypeRegion, callErr.Error())
					if disabled {
						ctxLog.Warn("区域已降级")
					}
				}
				return []string{}
			}
			ctxLog.Warnf("BWP 枚举带宽包 - API: DescribeBandwidthPackages - 失败 offset=%d: %v", offset, callErr)
			if t.degradeMgr != nil {
				disabled := t.degradeMgr.RecordFailure(regionKey, providerscommon.ResourceTypeRegion, callErr.Error())
				if disabled {
					ctxLog.Warn("区域已降级")
				}
			}
			break
		}

		if resp == nil || resp.Response == nil || resp.Response.BandwidthPackageSet == nil {
			break
		}

		currentCount := uint64(len(resp.Response.BandwidthPackageSet))
		if currentCount == 0 {
			break
		}

		for _, bp := range resp.Response.BandwidthPackageSet {
			if bp == nil || bp.BandwidthPackageId == nil {
				continue
			}
			ids = append(ids, *bp.BandwidthPackageId)
		}

		// 使用 TotalCount 和当前已获取的数量来判断是否还有更多数据
		if resp.Response.TotalCount != nil && *resp.Response.TotalCount > 0 {
			totalCollected := uint64(len(ids))
			if totalCollected >= *resp.Response.TotalCount {
				ctxLog.Debugf("BWP 枚举带宽包 - 分页完成 offset=%d current_count=%d total_collected=%d total_count=%d",
					offset, currentCount, totalCollected, *resp.Response.TotalCount)
				break
			}
		}

		if currentCount < limit {
			break
		}

		offset += limit
		ctxLog.Debugf("BWP 枚举带宽包 - 分页 offset=%d current_count=%d total_collected=%d", offset, currentCount, len(ids))
		time.Sleep(50 * time.Millisecond)
	}

	t.setCachedIDs(account, region, "QCE/BWP", "bwp", ids)

	// 更新区域状态
	if bwpRM != nil {
		status := providerscommon.RegionStatusEmpty
		if len(ids) > 0 {
			status = providerscommon.RegionStatusActive
		}
		bwpRM.UpdateRegionStatus(account.AccountID, region, len(ids), status)
		ctxLog.Debugf("更新 BWP 区域状态, status=%s, count=%d",
			status, len(ids))
	}

	if len(ids) > 0 {
		max := 5
		if len(ids) < max {
			max = len(ids)
		}
		preview := ids[:max]
		ctxLog.Debugf("BWP 已枚举，数量=%d 预览=%v", len(ids), preview)
	} else {
		ctxLog.Debugf("BWP 已枚举，数量=%d", len(ids))
	}
	return ids
}

func (t *Collector) fetchBWPMonitor(account config.CloudAccount, region string, prod config.Product, ids []string) {
	client, err := t.clientFactory.NewMonitorClient(region, account.AccessKeyID, account.AccessKeySecret)
	if err != nil {
		return
	}
	period := int64(60)
	if prod.Period != nil {
		period = int64(*prod.Period)
	}
	for _, group := range prod.MetricInfo {
		if group.Period != nil {
			period = int64(*group.Period)
		}
		for _, m := range group.MetricList {
			req := monitor.NewGetMonitorDataRequest()
			req.Namespace = common.StringPtr("QCE/BWP")
			req.MetricName = common.StringPtr(m)
			per := period
			if prod.Period == nil && group.Period == nil {
				fallback := int64(60)
				if server := t.cfg.GetServer(); server != nil && server.PeriodFallback > 0 {
					fallback = int64(server.PeriodFallback)
				}
				per = minPeriodForMetric(region, account, "QCE/BWP", m, fallback)
			}
			req.Period = common.Uint64Ptr(uint64(per))
			var inst []*monitor.Instance
			for _, id := range ids {
				inst = append(inst, &monitor.Instance{
					Dimensions: []*monitor.Dimension{
						{Name: common.StringPtr("bandwidthPackageId"), Value: common.StringPtr(id)},
					},
				})
			}
			req.Instances = inst
			start := time.Now().Add(-time.Duration(per) * time.Second)
			end := time.Now()
			req.StartTime = common.StringPtr(start.UTC().Format("2006-01-02T15:04:05Z"))
			req.EndTime = common.StringPtr(end.UTC().Format("2006-01-02T15:04:05Z"))

			reqStart := time.Now()
			var resp *monitor.GetMonitorDataResponse

			retryConfig := providerscommon.DefaultRetryConfig()
			shouldRetry := providerscommon.ShouldRetryForLimitError(providerscommon.TencentClassifier)

			// 使用带超时的上下文
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			err := providerscommon.RetryWithBackoff(ctx, retryConfig, func() error {
				var callErr error
				resp, callErr = client.GetMonitorData(req)
				if callErr != nil {
					status := providerscommon.ClassifyTencentError(callErr)
					metrics.RequestTotal.WithLabelValues("tencent", "GetMonitorData", status).Inc()
					metrics.RecordRequest("tencent", "GetMonitorData", status)
					if status == providerscommon.ErrorStatusLimit {
						metrics.RateLimitTotal.WithLabelValues("tencent", "GetMonitorData").Inc()
					}
					return callErr
				}
				metrics.RequestTotal.WithLabelValues("tencent", "GetMonitorData", "success").Inc()
				metrics.RecordRequest("tencent", "GetMonitorData", "success")
				metrics.RequestDuration.WithLabelValues("tencent", "GetMonitorData").Observe(time.Since(reqStart).Seconds())
				return nil
			}, shouldRetry)
			cancel()

			if err != nil {
				continue
			}

			if resp == nil || resp.Response == nil || resp.Response.DataPoints == nil || len(resp.Response.DataPoints) == 0 {
				continue
			}
			for _, dp := range resp.Response.DataPoints {
				if dp == nil || len(dp.Dimensions) == 0 || len(dp.Values) == 0 {
					continue
				}
				rid := extractDimension(dp.Dimensions, "bandwidthPackageId")
				if rid == "" {
					continue
				}
				v := dp.Values[len(dp.Values)-1]
				if v == nil {
					continue
				}
				val := *v
				alias, count := metrics.NamespaceGauge("QCE/BWP", m)
				scaled := scaleBWPMetric(m, val)
				metricAlias := metrics.GetMetricAlias("QCE/BWP", m)
				if metricAlias != "" {
					ctxLog := logger.NewContextLogger("Tencent", "account_id", account.AccountID, "region", region, "resource_type", "BWP")
					ctxLog.Debugf("BWP指标映射: 命名空间=QCE/BWP 原始=%s 别名=%s 最终名称=bwp_%s", m, metricAlias, metricAlias)
				}
				labels := []string{"tencent", account.AccountID, region, "bwp", rid, "QCE/BWP", m, ""}
				for len(labels) < count {
					labels = append(labels, "")
				}
				alias.WithLabelValues(labels...).Set(scaled)
				metrics.IncSampleCountWithLabels(account.AccountID, region, "bwp", "QCE/BWP", 1)
			}
		}
	}
}

func scaleBWPMetric(metric string, val float64) float64 {
	if s := metrics.GetMetricScale("QCE/BWP", metric); s != 0 && s != 1 {
		return val * s
	}
	// 兼容多种指标名称的流量指标（单位从 Mbps 转换为 bit/s）
	if metric == "InTraffic" || metric == "OutTraffic" {
		return val * 1000000
	}
	return val
}
