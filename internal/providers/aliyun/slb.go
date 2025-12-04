package aliyun

import (
	"encoding/json"
	"log"
	"strings"
	"time"

	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/metrics"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/requests"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/responses"
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
	slbClient, slbErr := slb.NewClientWithAccessKey(region, account.AccessKeyID, account.AccessKeySecret)
	if slbErr != nil {
		return map[string]string{}
	}
	tagClient, tagErr := tag.NewClientWithAccessKey(region, account.AccessKeyID, account.AccessKeySecret)
	if tagErr != nil {
		tagClient = nil
	}
	out := make(map[string]string, len(ids))
	for _, lbID := range ids {
		reqAttr := slb.CreateDescribeLoadBalancerAttributeRequest()
		reqAttr.RegionId = region
		reqAttr.LoadBalancerId = lbID
		var attrResp *slb.DescribeLoadBalancerAttributeResponse
		var attrErr error
		for attempt := 0; attempt < 3; attempt++ {
			start := time.Now()
			attrResp, attrErr = slbClient.DescribeLoadBalancerAttribute(reqAttr)
			if attrErr == nil {
				metrics.RequestTotal.WithLabelValues("aliyun", "DescribeLoadBalancerAttribute", "success").Inc()
				metrics.RequestDuration.WithLabelValues("aliyun", "DescribeLoadBalancerAttribute").Observe(time.Since(start).Seconds())
				break
			}
			status := classifyAliyunError(attrErr)
			metrics.RequestTotal.WithLabelValues("aliyun", "DescribeLoadBalancerAttribute", status).Inc()
			if status == "auth_error" {
				break
			}
			time.Sleep(time.Duration(150*(attempt+1)) * time.Millisecond)
		}
		if attrErr == nil && attrResp != nil {
			var jr struct {
				Tags struct {
					Tag []struct {
						TagKey   string `json:"TagKey"`
						TagValue string `json:"TagValue"`
					} `json:"Tag"`
				} `json:"Tags"`
				LoadBalancerName string `json:"LoadBalancerName"`
			}
			_ = json.Unmarshal(attrResp.GetHttpContentBytes(), &jr)
			for _, tt := range jr.Tags.Tag {
				if strings.EqualFold(tt.TagKey, "CodeName") || strings.EqualFold(tt.TagKey, "code_name") {
					out[lbID] = tt.TagValue
				}
			}
			// 仅提取 CodeName，不再使用 LoadBalancerName 兜底
		}

		// 使用通用请求调用 SLB 的 DescribeTags 接口，避免依赖 ARN 与账号 ID
		if out[lbID] == "" {
			creq := requests.NewCommonRequest()
			creq.Method = "POST"
			creq.Scheme = "https"
			creq.Product = "Slb"
			creq.Version = "2014-05-15"
			creq.ApiName = "DescribeTags"
			creq.QueryParams["RegionId"] = region
			creq.QueryParams["LoadBalancerId"] = lbID
			var cresp *responses.CommonResponse
			var cerr error
			for attempt := 0; attempt < 3; attempt++ {
				start := time.Now()
				cresp, cerr = slbClient.ProcessCommonRequest(creq)
				if cerr == nil {
					metrics.RequestTotal.WithLabelValues("aliyun", "DescribeTags", "success").Inc()
					metrics.RequestDuration.WithLabelValues("aliyun", "DescribeTags").Observe(time.Since(start).Seconds())
					break
				}
				status := classifyAliyunError(cerr)
				metrics.RequestTotal.WithLabelValues("aliyun", "DescribeTags", status).Inc()
				if status == "auth_error" {
					break
				}
				time.Sleep(time.Duration(150*(attempt+1)) * time.Millisecond)
			}
			if cerr == nil && cresp != nil {
				var tj struct {
					TagSets struct {
						TagSet []struct {
							TagKey   string `json:"TagKey"`
							TagValue string `json:"TagValue"`
						} `json:"TagSet"`
					} `json:"TagSets"`
				}
				_ = json.Unmarshal(cresp.GetHttpContentBytes(), &tj)
				for _, ts := range tj.TagSets.TagSet {
					if strings.EqualFold(ts.TagKey, "CodeName") || strings.EqualFold(ts.TagKey, "code_name") {
						out[lbID] = ts.TagValue
					}
				}
			}
		}

		if out[lbID] == "" && tagClient != nil {
			req := tag.CreateListTagResourcesRequest()
			req.RegionId = region
			arn := "arn:acs:slb:" + region + ":" + account.AccountID + ":loadbalancer/" + lbID
			arns := []string{arn}
			req.ResourceARN = &arns
			var resp *tag.ListTagResourcesResponse
			var callErr error
			for attempt := 0; attempt < 3; attempt++ {
				startReq := time.Now()
				resp, callErr = tagClient.ListTagResources(req)
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
			if callErr == nil && resp != nil {
				var jrA struct {
					TagResources []struct {
						Tags []struct {
							Key      string `json:"Key"`
							Value    string `json:"Value"`
							TagKey   string `json:"TagKey"`
							TagValue string `json:"TagValue"`
						} `json:"Tags"`
					} `json:"TagResources"`
				}
				_ = json.Unmarshal(resp.GetHttpContentBytes(), &jrA)
				for _, tr := range jrA.TagResources {
					for _, kv := range tr.Tags {
						k := kv.Key
						if k == "" {
							k = kv.TagKey
						}
						v := kv.Value
						if v == "" {
							v = kv.TagValue
						}
						if strings.EqualFold(k, "CodeName") || strings.EqualFold(k, "code_name") {
							out[lbID] = v
						}
					}
				}
				if out[lbID] == "" {
					var jrB struct {
						TagResources struct {
							TagResource []struct {
								TagKey   string `json:"TagKey"`
								TagValue string `json:"TagValue"`
								Key      string `json:"Key"`
								Value    string `json:"Value"`
							} `json:"TagResource"`
						} `json:"TagResources"`
					}
					_ = json.Unmarshal(resp.GetHttpContentBytes(), &jrB)
					for _, tr := range jrB.TagResources.TagResource {
						k := tr.TagKey
						if k == "" {
							k = tr.Key
						}
						v := tr.TagValue
						if v == "" {
							v = tr.Value
						}
						if strings.EqualFold(k, "CodeName") || strings.EqualFold(k, "code_name") {
							out[lbID] = v
						}
					}
				}
			}
		}

		time.Sleep(20 * time.Millisecond)
	}
	assigned := 0
	for _, v := range out {
		if v != "" {
			assigned++
		}
	}
	log.Printf("Aliyun SLB 标签采集完成 account_id=%s region=%s 资源数=%d 有code_name=%d", account.AccountID, region, len(ids), assigned)
	return out
}
