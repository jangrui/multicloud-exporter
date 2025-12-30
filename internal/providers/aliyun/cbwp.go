package aliyun

import (
	"strings"
	"time"

	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/logger"
	"multicloud-exporter/internal/metrics"
	"multicloud-exporter/internal/providers/common"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/requests"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/vpc"
)

func (a *Collector) listCBWPIDs(account config.CloudAccount, region string) []string {
	ctxLog := logger.NewContextLogger("Aliyun", "account_id", account.AccountID, "region", region)
	ctxLog.Debugf("枚举共享带宽包开始")
	client, err := a.clientFactory.NewVPCClient(region, account.AccessKeyID, account.AccessKeySecret)
	if err != nil {
		return []string{}
	}
	var ids []string
	pageSize := 50
	if a.cfg != nil {
		if a.cfg.Server != nil && a.cfg.Server.PageSize > 0 {
			if a.cfg.Server.PageSize < pageSize {
				pageSize = a.cfg.Server.PageSize
			}
		} else if server := a.cfg.GetServer(); server != nil && server.PageSize > 0 {
			if server.PageSize < pageSize {
				pageSize = server.PageSize
			}
		}
	}
	page := 1
	for {
		req := vpc.CreateDescribeCommonBandwidthPackagesRequest()
		req.RegionId = region
		req.PageSize = requests.NewInteger(pageSize)
		req.PageNumber = requests.NewInteger(page)
		var resp *vpc.DescribeCommonBandwidthPackagesResponse
		var callErr error
		for attempt := 0; attempt < 3; attempt++ {
			start := time.Now()
			resp, callErr = client.DescribeCommonBandwidthPackages(req)
			if callErr == nil {
				metrics.RequestTotal.WithLabelValues("aliyun", "DescribeCommonBandwidthPackages", "success").Inc()
				metrics.RecordRequest("aliyun", "DescribeCommonBandwidthPackages", "success")
				metrics.RequestDuration.WithLabelValues("aliyun", "DescribeCommonBandwidthPackages").Observe(time.Since(start).Seconds())
				break
			}
			msg := callErr.Error()
			if strings.Contains(msg, "InvalidParameter") || strings.Contains(msg, "InvalidPageSize") {
				req.PageSize = requests.NewInteger(pageSize)
				resp, callErr = client.DescribeCommonBandwidthPackages(req)
				if callErr == nil {
					metrics.RequestTotal.WithLabelValues("aliyun", "DescribeCommonBandwidthPackages", "success").Inc()
					metrics.RecordRequest("aliyun", "DescribeCommonBandwidthPackages", "success")
					metrics.RequestDuration.WithLabelValues("aliyun", "DescribeCommonBandwidthPackages").Observe(time.Since(start).Seconds())
					break
				}
			}
			status := common.ClassifyAliyunError(callErr)
			metrics.RequestTotal.WithLabelValues("aliyun", "DescribeCommonBandwidthPackages", status).Inc()
			metrics.RecordRequest("aliyun", "DescribeCommonBandwidthPackages", status)
			if status == "limit_error" {
				// 记录限流指标
				metrics.RateLimitTotal.WithLabelValues("aliyun", "DescribeCommonBandwidthPackages").Inc()
			}
			if status == "region_skip" || status == "auth_error" {
				ctxLog.Warnf("CBWP describe error page=%d status=%s: %v", page, status, callErr)
				break
			}
			// 指数退避重试
			sleep := time.Duration(200*(1<<attempt)) * time.Millisecond
			if sleep > 5*time.Second {
				sleep = 5 * time.Second
			}
			time.Sleep(sleep)
		}
		if callErr != nil {
			break
		}
		if resp == nil {
			break
		}
		if len(resp.CommonBandwidthPackages.CommonBandwidthPackage) == 0 {
			break
		}
		for _, pkg := range resp.CommonBandwidthPackages.CommonBandwidthPackage {
			ids = append(ids, pkg.BandwidthPackageId)
		}

		// 使用 TotalCount 和当前已获取的数量来判断是否还有更多数据
		// 如果返回的数据量小于 pageSize，说明已经是最后一页
		// 如果返回的数据量等于 pageSize，需要检查是否还有更多页
		currentCount := len(resp.CommonBandwidthPackages.CommonBandwidthPackage)

		// 检查是否还有更多页：如果返回的数据量小于 pageSize，说明已经是最后一页
		// 如果返回的数据量等于 pageSize，可能还有更多页，继续下一页
		// 使用 TotalCount 来验证（如果存在）
		if resp.TotalCount > 0 {
			// 如果 TotalCount 存在，可以用它来判断是否还有更多数据
			totalCollected := len(ids)
			if totalCollected >= resp.TotalCount {
				// 已收集的数量达到总数，停止分页
				ctxLog.Debugf("CBWP 分页采集完成 page=%d current_count=%d total_collected=%d total_count=%d",
					page, currentCount, totalCollected, resp.TotalCount)
				break
			}
		}

		if currentCount < pageSize {
			// 当前页数据量小于 pageSize，说明已经是最后一页
			break
		}

		// 继续下一页
		page++
		ctxLog.Debugf("CBWP 分页采集 page=%d current_count=%d total_collected=%d", page, currentCount, len(ids))
		time.Sleep(50 * time.Millisecond)
	}
	// 打印缩略的 ID 列表，便于定位
	if len(ids) > 0 {
		max := 5
		if len(ids) < max {
			max = len(ids)
		}
		preview := ids[:max]
		ctxLog.Debugf("枚举共享带宽包完成 数量=%d 预览=%v", len(ids), preview)
	} else {
		ctxLog.Debugf("枚举共享带宽包完成 数量=%d", len(ids))
	}

	// 更新区域状态
	if a.regionManager != nil {
		status := common.RegionStatusEmpty
		if len(ids) > 0 {
			status = common.RegionStatusActive
		}
		a.regionManager.UpdateRegionStatus(account.AccountID, region, len(ids), status)
		ctxLog.Debugf("更新区域状态 account=%s region=%s status=%s count=%d",
			account.AccountID, region, status, len(ids))
	}

	return ids
}

func (a *Collector) fetchCBWPTags(account config.CloudAccount, region string, ids []string) map[string]string {
	// 仅提取共享带宽包的 CodeName 标签：
	// 返回值为带宽包ID到 CodeName 文本的映射，用于在指标的 code_name 标签中展示。
	if len(ids) == 0 {
		return map[string]string{}
	}

	// 添加超时保护：整个标签获取操作最多 30 秒
	const timeout = 30 * time.Second
	deadline := time.Now().Add(timeout)

	ctxLog := logger.NewContextLogger("Aliyun", "account_id", account.AccountID, "region", region, "rtype", "cbwp")
	ctxLog.Debugf("开始获取 BWP CodeName 标签 total_ids=%d timeout=%v", len(ids), timeout)
	client, err := a.clientFactory.NewVPCClient(region, account.AccessKeyID, account.AccessKeySecret)
	if err != nil {
		ctxLog.Warnf("创建 VPC 客户端失败，无法获取 CodeName 标签: %v", err)
		return map[string]string{}
	}
	out := make(map[string]string, len(ids))
	// 预先初始化所有 ID 为空字符串，确保所有 ID 都有记录
	for _, id := range ids {
		out[id] = ""
	}
	const batchSize = 20
	batchCount := 0
	for start := 0; start < len(ids); start += batchSize {
		// 检查是否超时
		if time.Now().After(deadline) {
			ctxLog.Warnf("获取 CodeName 标签超时，已处理 %d/%d 批次，返回部分结果", batchCount, (len(ids)+batchSize-1)/batchSize)
			break
		}
		batchCount++
		end := start + batchSize
		if end > len(ids) {
			end = len(ids)
		}
		batch := ids[start:end]
		ctxLog.Debugf("处理 CodeName 标签批次 %d/%d batch_size=%d ids=%v", batchCount, (len(ids)+batchSize-1)/batchSize, len(batch), batch)
		req := vpc.CreateListTagResourcesRequest()
		req.RegionId = region
		req.ResourceType = "COMMONBANDWIDTHPACKAGE"
		req.MaxResults = requests.NewInteger(50)
		req.ResourceId = &batch
		nextToken := ""
		batchSuccess := false
		for {
			// 检查是否超时
			if time.Now().After(deadline) {
				ctxLog.Warnf("批次 %d 获取标签时超时，跳过后续处理", batchCount)
				break
			}

			if nextToken != "" {
				req.NextToken = nextToken
			}
			var resp *vpc.ListTagResourcesResponse
			var callErr error
			for attempt := 0; attempt < 3; attempt++ {
				// 每次重试前检查超时
				if time.Now().After(deadline) {
					ctxLog.Warnf("批次 %d 重试第 %d 次时超时，放弃", batchCount, attempt+1)
					break
				}
				startReq := time.Now()
				resp, callErr = client.ListTagResources(req)
				if callErr == nil {
					metrics.RequestTotal.WithLabelValues("aliyun", "ListTagResources", "success").Inc()
					metrics.RequestDuration.WithLabelValues("aliyun", "ListTagResources").Observe(time.Since(startReq).Seconds())
					break
				}
				msg := callErr.Error()
				status := "error"
				if strings.Contains(msg, "Throttling") || strings.Contains(msg, "flow control") {
					status = "limit_error"
					// 记录限流指标
					metrics.RateLimitTotal.WithLabelValues("aliyun", "ListTagResources").Inc()
				} else if strings.Contains(msg, "timeout") || strings.Contains(msg, "Temporary network") || strings.Contains(msg, "unreachable") {
					status = "network_error"
				} else if strings.Contains(msg, "InvalidAccessKeyId") || strings.Contains(msg, "Forbidden") || strings.Contains(msg, "SignatureDoesNotMatch") {
					status = "auth_error"
				}
				metrics.RequestTotal.WithLabelValues("aliyun", "ListTagResources", status).Inc()
				if status == "auth_error" {
					break
				}
				// 指数退避重试，但考虑剩余时间
				sleep := time.Duration(200*(1<<attempt)) * time.Millisecond
				if sleep > 5*time.Second {
					sleep = 5 * time.Second
				}
				// 如果 sleep 会超过 deadline，缩短 sleep 时间
				if time.Now().Add(sleep).After(deadline) {
					sleep = deadline.Sub(time.Now())
					if sleep < 0 {
						sleep = 0
					}
				}
				time.Sleep(sleep)
			}
			if callErr != nil {
				ctxLog.Warnf("批次 %d 获取 CodeName 标签失败 error=%v batch=%v", batchCount, callErr, batch)
				break
			}
			if len(resp.TagResources.TagResource) == 0 {
				ctxLog.Debugf("批次 %d 无标签资源返回，可能资源未配置 CodeName 标签", batchCount)
				break
			}
			batchSuccess = true
			// 遍历标签资源，仅保留 TagKey=="CodeName" 的键值
			for _, tr := range resp.TagResources.TagResource {
				rid := tr.ResourceId
				if rid == "" {
					continue
				}
				if tr.TagKey == "CodeName" {
					out[rid] = tr.TagValue
					ctxLog.Debugf("找到 CodeName 标签 resource_id=%s code_name=%s", rid, tr.TagValue)
				}
			}
			if resp.NextToken == "" {
				break
			}
			nextToken = resp.NextToken
			time.Sleep(25 * time.Millisecond)
		}
		if !batchSuccess {
			ctxLog.Warnf("批次 %d 完全失败，这些 ID 的 code_name 将为空: %v", batchCount, batch)
		}
		time.Sleep(25 * time.Millisecond)
	}
	withCodeName := 0
	withoutCodeName := 0
	emptyIDs := []string{}
	for _, id := range ids {
		if val, ok := out[id]; ok && val != "" {
			withCodeName++
		} else {
			withoutCodeName++
			emptyIDs = append(emptyIDs, id)
		}
	}
	if len(emptyIDs) > 0 && len(emptyIDs) <= 10 {
		ctxLog.Debugf("无 CodeName 的 BWP ID: %v", emptyIDs)
	}
	return out
}
