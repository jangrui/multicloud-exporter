package aliyun

import (
	"log"
	"strings"
	"time"

	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/metrics"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/requests"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/vpc"
)

func (a *Collector) listCBWPIDs(account config.CloudAccount, region string) []string {
	log.Printf("Aliyun 枚举共享带宽包开始 account_id=%s region=%s", account.AccountID, region)
	client, err := vpc.NewClientWithAccessKey(region, account.AccessKeyID, account.AccessKeySecret)
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
		} else if a.cfg.ServerConf != nil && a.cfg.ServerConf.PageSize > 0 {
			if a.cfg.ServerConf.PageSize < pageSize {
				pageSize = a.cfg.ServerConf.PageSize
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
				metrics.RequestDuration.WithLabelValues("aliyun", "DescribeCommonBandwidthPackages").Observe(time.Since(start).Seconds())
				break
			}
			msg := callErr.Error()
			if strings.Contains(msg, "InvalidParameter") || strings.Contains(msg, "InvalidPageSize") {
				req.PageSize = requests.NewInteger(pageSize)
				resp, callErr = client.DescribeCommonBandwidthPackages(req)
				if callErr == nil {
					metrics.RequestTotal.WithLabelValues("aliyun", "DescribeCommonBandwidthPackages", "success").Inc()
					metrics.RequestDuration.WithLabelValues("aliyun", "DescribeCommonBandwidthPackages").Observe(time.Since(start).Seconds())
					break
				}
			}
			status := classifyAliyunError(callErr)
			metrics.RequestTotal.WithLabelValues("aliyun", "DescribeCommonBandwidthPackages", status).Inc()
			if status == "region_skip" || status == "auth_error" {
				log.Printf("Aliyun CBWP describe error region=%s page=%d status=%s: %v", region, page, status, callErr)
				break
			}
			time.Sleep(time.Duration(200*(attempt+1)) * time.Millisecond)
		}
		if callErr != nil {
			break
		}
		if resp.CommonBandwidthPackages.CommonBandwidthPackage == nil || len(resp.CommonBandwidthPackages.CommonBandwidthPackage) == 0 {
			break
		}
		for _, pkg := range resp.CommonBandwidthPackages.CommonBandwidthPackage {
			ids = append(ids, pkg.BandwidthPackageId)
		}
		if len(resp.CommonBandwidthPackages.CommonBandwidthPackage) < pageSize {
			break
		}
		page++
		time.Sleep(50 * time.Millisecond)
	}
	log.Printf("Aliyun 枚举共享带宽包完成 account_id=%s region=%s 数量=%d", account.AccountID, region, len(ids))
	return ids
}

func (a *Collector) fetchCBWPTags(account config.CloudAccount, region string, ids []string) map[string]string {
	// 仅提取共享带宽包的 CodeName 标签：
	// 返回值为带宽包ID到 CodeName 文本的映射，用于在指标的 code_name 标签中展示。
	if len(ids) == 0 {
		return map[string]string{}
	}
	client, err := vpc.NewClientWithAccessKey(region, account.AccessKeyID, account.AccessKeySecret)
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
		req := vpc.CreateListTagResourcesRequest()
		req.RegionId = region
		req.ResourceType = "COMMONBANDWIDTHPACKAGE"
		req.MaxResults = requests.NewInteger(50)
		req.ResourceId = &batch
		nextToken := ""
		for {
			if nextToken != "" {
				req.NextToken = nextToken
			}
			var resp *vpc.ListTagResourcesResponse
			var callErr error
			for attempt := 0; attempt < 3; attempt++ {
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
				} else if strings.Contains(msg, "timeout") || strings.Contains(msg, "Temporary network") || strings.Contains(msg, "unreachable") {
					status = "network_error"
				} else if strings.Contains(msg, "InvalidAccessKeyId") || strings.Contains(msg, "Forbidden") || strings.Contains(msg, "SignatureDoesNotMatch") {
					status = "auth_error"
				}
				metrics.RequestTotal.WithLabelValues("aliyun", "ListTagResources", status).Inc()
				if status == "auth_error" {
					break
				}
				time.Sleep(time.Duration(200*(attempt+1)) * time.Millisecond)
			}
			if callErr != nil {
				break
			}
			if resp.TagResources.TagResource == nil {
				break
			}
			// 遍历标签资源，仅保留 TagKey=="CodeName" 的键值
			for _, tr := range resp.TagResources.TagResource {
				rid := tr.ResourceId
				if rid == "" {
					continue
				}
				if tr.TagKey == "CodeName" {
					out[rid] = tr.TagValue
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
