package aliyun

import (
	"fmt"
	"log"
	"time"

	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/metrics"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/requests"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/slb"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/tag"
)

func (a *Collector) listSLBIDs(account config.CloudAccount, region string) []string {
	client, err := slb.NewClientWithAccessKey(region, account.AccessKeyID, account.AccessKeySecret)
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
		req := slb.CreateDescribeLoadBalancersRequest()
		req.RegionId = region
		req.PageSize = requests.NewInteger(pageSize)
		req.PageNumber = requests.NewInteger(page)
		var resp *slb.DescribeLoadBalancersResponse
		var callErr error
		for attempt := 0; attempt < 3; attempt++ {
			start := time.Now()
			resp, callErr = client.DescribeLoadBalancers(req)
			if callErr == nil {
				metrics.RequestTotal.WithLabelValues("aliyun", "DescribeLoadBalancers", "success").Inc()
				metrics.RequestDuration.WithLabelValues("aliyun", "DescribeLoadBalancers").Observe(time.Since(start).Seconds())
				break
			}
			status := classifyAliyunError(callErr)
			metrics.RequestTotal.WithLabelValues("aliyun", "DescribeLoadBalancers", status).Inc()
			if status == "region_skip" || status == "auth_error" {
				log.Printf("Aliyun SLB describe error region=%s page=%d status=%s: %v", region, page, status, callErr)
				break
			}
			time.Sleep(time.Duration(200*(attempt+1)) * time.Millisecond)
		}
		if callErr != nil {
			break
		}
		if len(resp.LoadBalancers.LoadBalancer) == 0 {
			break
		}
		for _, lb := range resp.LoadBalancers.LoadBalancer {
			if lb.LoadBalancerId != "" {
				ids = append(ids, lb.LoadBalancerId)
			}
		}
		if len(resp.LoadBalancers.LoadBalancer) < pageSize {
			break
		}
		page++
		time.Sleep(50 * time.Millisecond)
	}
	log.Printf("Aliyun 枚举SLB实例完成 account_id=%s region=%s 数量=%d", account.AccountID, region, len(ids))
	return ids
}

func (a *Collector) fetchSLBTags(account config.CloudAccount, region string, ids []string) map[string]string {
	if len(ids) == 0 {
		return map[string]string{}
	}
	client, err := tag.NewClientWithAccessKey(region, account.AccessKeyID, account.AccessKeySecret)
	if err != nil {
		return map[string]string{}
	}
	out := make(map[string]string, len(ids))
	for _, lbID := range ids {
		req := tag.CreateListTagResourcesRequest()
		req.RegionId = region
		arns := []string{fmt.Sprintf("arn:acs:slb:%s:%s:loadbalancer/%s", region, account.AccountID, lbID)}
		req.ResourceARN = &arns
		var resp *tag.ListTagResourcesResponse
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
			continue
		}
		if len(resp.TagResources) == 0 {
			continue
		}
		for _, tr := range resp.TagResources {
			if len(tr.Tags) == 0 {
				continue
			}
			for _, kv := range tr.Tags {
				if kv.Key == "CodeName" {
					out[lbID] = kv.Value
				}
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	return out
}
