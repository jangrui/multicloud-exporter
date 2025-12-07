package aliyun

import (
	"time"

	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/logger"
	"multicloud-exporter/internal/metrics"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/requests"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/ecs"
)

func (a *Collector) listECSInstanceIDs(account config.CloudAccount, region string) ([]string, map[string]interface{}) {
	ctxLog := logger.NewContextLogger("Aliyun", "account_id", account.AccountID, "region", region)
	ctxLog.Debugf("枚举ECS实例开始")
	client, err := ecs.NewClientWithAccessKey(region, account.AccessKeyID, account.AccessKeySecret)
	if err != nil {
		return []string{}, nil
	}
	var ids []string
	// meta 用于存储 ECS 相关的额外维度信息（如磁盘ID、网卡ID等），暂留空
	// 如果需要支持磁盘/网卡维度，需在此处（或单独函数）获取并填充
	meta := make(map[string]interface{})

	pageSize := 50
	if a.cfg != nil {
		if a.cfg.Server != nil && a.cfg.Server.PageSize > 0 {
			if a.cfg.Server.PageSize < pageSize {
				pageSize = a.cfg.Server.PageSize
			}
		} else if a.cfg.ServerConf != nil && a.cfg.ServerConf.PageSize > 0 {
			if a.cfg.ServerConf.PageSize < pageSize {
				pageSize = a.cfg.ServerConf.PageSize
			}
		}
	}
	page := 1
	for {
		req := ecs.CreateDescribeInstancesRequest()
		req.RegionId = region
		req.PageSize = requests.NewInteger(pageSize)
		req.PageNumber = requests.NewInteger(page)
		var resp *ecs.DescribeInstancesResponse
		var callErr error
		for attempt := 0; attempt < 3; attempt++ {
			start := time.Now()
			resp, callErr = client.DescribeInstances(req)
			if callErr == nil {
				metrics.RequestTotal.WithLabelValues("aliyun", "DescribeInstances", "success").Inc()
				metrics.RequestDuration.WithLabelValues("aliyun", "DescribeInstances").Observe(time.Since(start).Seconds())
				break
			}
			status := classifyAliyunError(callErr)
			metrics.RequestTotal.WithLabelValues("aliyun", "DescribeInstances", status).Inc()
			if status == "region_skip" || status == "auth_error" {
				ctxLog.Warnf("ECS describe error page=%d status=%s: %v", page, status, callErr)
				break
			}
			time.Sleep(time.Duration(200*(attempt+1)) * time.Millisecond)
		}
		if callErr != nil {
			break
		}
		if len(resp.Instances.Instance) == 0 {
			break
		}
		for _, ins := range resp.Instances.Instance {
			if ins.InstanceId != "" {
				ids = append(ids, ins.InstanceId)
			}
		}
		if len(resp.Instances.Instance) < pageSize {
			break
		}
		page++
		time.Sleep(50 * time.Millisecond)
	}
	ctxLog.Debugf("枚举ECS实例完成 数量=%d", len(ids))
	return ids, meta
}

func (a *Collector) fetchECSTags(account config.CloudAccount, region string, ids []string) map[string]string {
	// 仅提取 ECS 实例的 CodeName 标签：
	// 返回值为实例ID到 CodeName 文本的映射，用于在指标的 code_name 标签中展示。
	if len(ids) == 0 {
		return map[string]string{}
	}
	client, err := ecs.NewClientWithAccessKey(region, account.AccessKeyID, account.AccessKeySecret)
	if err != nil {
		return map[string]string{}
	}
	out := make(map[string]string, len(ids))
	const batchSize = 20
	for start := 0; start < len(ids); start += batchSize {
		end := start + batchSize
		if end > len(ids) {
			end = len(ids)
		}
		batch := ids[start:end]
		req := ecs.CreateListTagResourcesRequest()
		req.RegionId = region
		req.ResourceType = "instance"
		req.ResourceId = &batch
		nextToken := ""
		for {
			if nextToken != "" {
				req.NextToken = nextToken
			}
			var resp *ecs.ListTagResourcesResponse
			var callErr error
			for attempt := 0; attempt < 3; attempt++ {
				startReq := time.Now()
				resp, callErr = client.ListTagResources(req)
				if callErr == nil {
					metrics.RequestTotal.WithLabelValues("aliyun", "ListTagResources", "success").Inc()
					metrics.RequestDuration.WithLabelValues("aliyun", "ListTagResources").Observe(time.Since(startReq).Seconds())
					break
				}
				status := classifyAliyunError(callErr)
				metrics.RequestTotal.WithLabelValues("aliyun", "ListTagResources", status).Inc()
				if status == "auth_error" {
					break
				}
				time.Sleep(time.Duration(200*(attempt+1)) * time.Millisecond)
			}
			if callErr != nil {
				break
			}
			if len(resp.TagResources.TagResource) == 0 {
				break
			}
			// 遍历标签资源，仅保留 TagKey=="CodeName" 的键值
			for _, tr := range resp.TagResources.TagResource {
				if tr.ResourceId == "" {
					continue
				}
				if tr.TagKey == "CodeName" {
					out[tr.ResourceId] = tr.TagValue
				}
			}
			if resp.NextToken == "" {
				break
			}
			nextToken = resp.NextToken
			time.Sleep(25 * time.Millisecond)
		}
		time.Sleep(25 * time.Millisecond)
	}
	return out
}
