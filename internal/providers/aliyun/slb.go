package aliyun

import (
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"time"

	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/logger"
	"multicloud-exporter/internal/metrics"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/requests"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/slb"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/tag"
)

func (a *Collector) listSLBIDs(account config.CloudAccount, region string) ([]string, map[string]interface{}) {
	ctxLog := logger.NewContextLogger("Aliyun", "account_id", account.AccountID, "region", region)
	client, err := a.clientFactory.NewSLBClient(region, account.AccessKeyID, account.AccessKeySecret)
	if err != nil {
		return []string{}, nil
	}
	var ids []string
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
			if status == "limit_error" {
				// 记录限流指标
				metrics.RateLimitTotal.WithLabelValues("aliyun", "DescribeLoadBalancers").Inc()
			}
			if status == "region_skip" || status == "auth_error" {
				ctxLog.Warnf("SLB describe error page=%d status=%s: %v", page, status, callErr)
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

	// 并发获取每个实例的监听器详情（用于补充 port/protocol 维度）
	if len(ids) > 0 {
		ctxLog.Debugf("开始获取SLB监听器详情 count=%d", len(ids))
		var wg sync.WaitGroup
		sem := make(chan struct{}, 5) // 控制并发度
		var mu sync.Mutex

		for _, id := range ids {
			wg.Add(1)
			sem <- struct{}{}
			go func(lbId string) {
				defer wg.Done()
				defer func() { <-sem }()

				req := slb.CreateDescribeLoadBalancerAttributeRequest()
				req.LoadBalancerId = lbId

				var resp *slb.DescribeLoadBalancerAttributeResponse
				var err error
				for i := 0; i < 3; i++ {
					startReq := time.Now()
					resp, err = client.DescribeLoadBalancerAttribute(req)
					if err == nil {
						metrics.RequestTotal.WithLabelValues("aliyun", "DescribeLoadBalancerAttribute", "success").Inc()
						metrics.RequestDuration.WithLabelValues("aliyun", "DescribeLoadBalancerAttribute").Observe(time.Since(startReq).Seconds())
						metrics.RecordRequest("aliyun", "DescribeLoadBalancerAttribute", "success")
						break
					}
					status := classifyAliyunError(err)
					metrics.RequestTotal.WithLabelValues("aliyun", "DescribeLoadBalancerAttribute", status).Inc()
					metrics.RecordRequest("aliyun", "DescribeLoadBalancerAttribute", status)
					if status == "limit_error" {
						// 记录限流指标
						metrics.RateLimitTotal.WithLabelValues("aliyun", "DescribeLoadBalancerAttribute").Inc()
					}
					if status == "auth_error" || status == "region_skip" {
						break
					}
					time.Sleep(time.Duration(100*(i+1)) * time.Millisecond)
				}

				if err != nil {
					ctxLog.Warnf("fetch SLB attribute failed id=%s: %v", lbId, err)
					return
				}

				var listeners []map[string]string
				// 尝试从 ListenerPortsAndProtocol 获取
				if resp.ListenerPortsAndProtocol.ListenerPortAndProtocol != nil {
					for _, lp := range resp.ListenerPortsAndProtocol.ListenerPortAndProtocol {
						if lp.ListenerPort > 0 && lp.ListenerProtocol != "" {
							listeners = append(listeners, map[string]string{
								"port":     strconv.Itoa(lp.ListenerPort),
								"protocol": lp.ListenerProtocol,
							})
						}
					}
				}

				if len(listeners) > 0 {
					mu.Lock()
					meta[lbId] = listeners
					mu.Unlock()
				}
			}(id)
		}
		wg.Wait()
	}

	ctxLog.Debugf("枚举SLB实例完成 实例数=%d 带监听器数=%d", len(ids), len(meta))
	return ids, meta
}

// fetchSLBTags 批量获取 SLB 标签（包含 code_name）
func (a *Collector) fetchSLBTags(account config.CloudAccount, region, namespace, metric string, ids []string) map[string]string {
	if len(ids) == 0 {
		return map[string]string{}
	}
	ctxLog := logger.NewContextLogger("Aliyun", "account_id", account.AccountID, "region", region, "namespace", namespace, "metric", metric)

	tagClient, tagErr := a.clientFactory.NewTagClient(region, account.AccessKeyID, account.AccessKeySecret)
	if tagErr != nil {
		ctxLog.Warnf("init tag client error: %v", tagErr)
		return map[string]string{}
	}

	out := make(map[string]string, len(ids))
	// 阿里云 ListTagResources 支持最多 50 个 ARN，这里使用 50
	batchSize := 50
	total := len(ids)

	// 获取真实的 Account UID 用于构建 ARN
	uid := a.getAccountUID(account, region)

	for start := 0; start < total; start += batchSize {
		end := start + batchSize
		if end > total {
			end = total
		}
		batchIDs := ids[start:end]
		var arns []string
		for _, id := range batchIDs {
			// 构建 ARN: arn:acs:slb:{region}:{uid}:loadbalancer/{lb_id}
			arn := "arn:acs:slb:" + region + ":" + uid + ":loadbalancer/" + id
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
				metrics.RecordRequest("aliyun", "ListTagResources", "success")
				break
			}
			status := classifyAliyunError(callErr)
			metrics.RequestTotal.WithLabelValues("aliyun", "ListTagResources", status).Inc()
			metrics.RecordRequest("aliyun", "ListTagResources", status)
			if status == "limit_error" {
				// 记录限流指标
				metrics.RateLimitTotal.WithLabelValues("aliyun", "ListTagResources").Inc()
			}
			if status == "auth_error" {
				break
			}
			time.Sleep(time.Duration(200*(attempt+1)) * time.Millisecond)
		}

		if callErr == nil && resp != nil {
			// 优先使用 SDK 解析好的结构
			if len(resp.TagResources) > 0 {
				for _, tr := range resp.TagResources {
					id := tr.ResourceId
					if id == "" && tr.ResourceARN != "" {
						parts := strings.Split(tr.ResourceARN, "/")
						if len(parts) > 0 {
							id = parts[len(parts)-1]
						}
					}
					if id == "" {
						continue
					}
					// 尝试遍历 Tags 列表 (jrA 格式)
					if len(tr.Tags) > 0 {
						for _, t := range tr.Tags {
							k := t.Key
							v := t.Value
							if strings.EqualFold(k, "CodeName") || strings.EqualFold(k, "code_name") {
								out[id] = v
							}
						}
					}
				}
			} else {
				content := resp.GetHttpContentBytes()
				parsed := parseSLBTagsContent(content)
				for k, v := range parsed {
					out[k] = v
				}
			}
		}

		// 进度日志
		if (end)%100 == 0 || end == total {
			ctxLog.Debugf("SLB 标签采集进度 progress=%d/%d (%.1f%%)", end, total, float64(end)/float64(total)*100)
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
	ctxLog.Debugf("SLB 标签采集完成 资源数=%d 有code_name=%d", len(ids), assigned)
	return out
}

func parseSLBTagsContent(content []byte) map[string]string {
	out := make(map[string]string)
	var jrA struct {
		TagResources []struct {
			ResourceId  string `json:"ResourceId"`
			ResourceARN string `json:"ResourceARN"`
			Tags        []struct {
				Key      string `json:"Key"`
				Value    string `json:"Value"`
				TagKey   string `json:"TagKey"`
				TagValue string `json:"TagValue"`
			} `json:"Tags"`
		} `json:"TagResources"`
	}
	if err := json.Unmarshal(content, &jrA); err == nil && len(jrA.TagResources) > 0 {
		for _, tr := range jrA.TagResources {
			id := tr.ResourceId
			if id == "" && tr.ResourceARN != "" {
				parts := strings.Split(tr.ResourceARN, "/")
				if len(parts) > 0 {
					id = parts[len(parts)-1]
				}
			}
			if id == "" {
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
					out[id] = v
				}
			}
		}
	}
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
	return out
}
