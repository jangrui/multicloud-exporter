package aliyun

import (
	"encoding/json"
	"log"
	"strings"
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
	tagClient, tagErr := tag.NewClientWithAccessKey(region, account.AccessKeyID, account.AccessKeySecret)
	if tagErr != nil {
		log.Printf("Aliyun init tag client error: %v", tagErr)
		return map[string]string{}
	}

	out := make(map[string]string, len(ids))
	// 阿里云 ListTagResources 支持最多 50 个 ARN，这里使用 50
	batchSize := 50
	total := len(ids)

	for start := 0; start < total; start += batchSize {
		end := start + batchSize
		if end > total {
			end = total
		}
		batchIDs := ids[start:end]
		var arns []string
		for _, id := range batchIDs {
			// 构建 ARN: arn:acs:slb:{region}:{account}:loadbalancer/{lb_id}
			arn := "arn:acs:slb:" + region + ":" + account.AccountID + ":loadbalancer/" + id
			arns = append(arns, arn)
		}

		req := tag.CreateListTagResourcesRequest()
		req.RegionId = region
		// req.ResourceType = "loadbalancer" // SDK 中可能无此字段，依赖 ARN 推断
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
			content := resp.GetHttpContentBytes()
			// 解析格式 A: TagResources -> [] { ResourceId, Tags }
			var jrA struct {
				TagResources []struct {
					ResourceId string `json:"ResourceId"`
					Tags       []struct {
						Key      string `json:"Key"`
						Value    string `json:"Value"`
						TagKey   string `json:"TagKey"`
						TagValue string `json:"TagValue"`
					} `json:"Tags"`
				} `json:"TagResources"`
			}
			if err := json.Unmarshal(content, &jrA); err == nil && len(jrA.TagResources) > 0 {
				for _, tr := range jrA.TagResources {
					if tr.ResourceId == "" {
						continue
					}
					for _, t := range tr.Tags {
						k := t.Key
						if k == "" {
							k = t.TagKey
						}
						v := t.Value
						if v == "" {
							v = t.TagValue
						}
						if strings.EqualFold(k, "CodeName") || strings.EqualFold(k, "code_name") {
							out[tr.ResourceId] = v
						}
					}
				}
			}

			// 解析格式 B: TagResources -> TagResource -> [] { ResourceId, TagKey, TagValue }
			var jrB struct {
				TagResources struct {
					TagResource []struct {
						ResourceId string `json:"ResourceId"`
						TagKey     string `json:"TagKey"`
						TagValue   string `json:"TagValue"`
						Key        string `json:"Key"`
						Value      string `json:"Value"`
					} `json:"TagResource"`
				} `json:"TagResources"`
			}
			if err := json.Unmarshal(content, &jrB); err == nil && len(jrB.TagResources.TagResource) > 0 {
				for _, tr := range jrB.TagResources.TagResource {
					if tr.ResourceId == "" {
						continue
					}
					k := tr.TagKey
					if k == "" {
						k = tr.Key
					}
					v := tr.TagValue
					if v == "" {
						v = tr.Value
					}
					if strings.EqualFold(k, "CodeName") || strings.EqualFold(k, "code_name") {
						out[tr.ResourceId] = v
					}
				}
			}
		}

		// 进度日志
		if (end)%100 == 0 || end == total {
			log.Printf("Aliyun SLB 标签采集进度 account_id=%s region=%s progress=%d/%d", account.AccountID, region, end, total)
		}

		// 批次间隔
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
