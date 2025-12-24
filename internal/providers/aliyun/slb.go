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
	"multicloud-exporter/internal/providers/common"

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
		} else if server := a.cfg.GetServer(); server != nil && server.PageSize > 0 {
			if server.PageSize < pageSize {
				pageSize = server.PageSize
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
			status := common.ClassifyAliyunError(callErr)
			metrics.RequestTotal.WithLabelValues("aliyun", "DescribeLoadBalancers", status).Inc()
			if status == "limit_error" {
				// 记录限流指标
				metrics.RateLimitTotal.WithLabelValues("aliyun", "DescribeLoadBalancers").Inc()
			}
			if status == "region_skip" || status == "auth_error" {
				ctxLog.Warnf("SLB describe error page=%d status=%s: %v", page, status, callErr)
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
		if len(resp.LoadBalancers.LoadBalancer) == 0 {
			break
		}
		for _, lb := range resp.LoadBalancers.LoadBalancer {
			if lb.LoadBalancerId != "" {
				ids = append(ids, lb.LoadBalancerId)
			}
		}

		// 使用 TotalCount 和当前已获取的数量来判断是否还有更多数据
		// 如果返回的数据量小于 pageSize，说明已经是最后一页
		// 如果返回的数据量等于 pageSize，需要检查是否还有更多页
		currentCount := len(resp.LoadBalancers.LoadBalancer)

		// 检查是否还有更多页：如果返回的数据量小于 pageSize，说明已经是最后一页
		// 如果返回的数据量等于 pageSize，可能还有更多页，继续下一页
		// 使用 TotalCount 来验证（如果存在）
		if resp.TotalCount > 0 {
			// 如果 TotalCount 存在，可以用它来判断是否还有更多数据
			totalCollected := len(ids)
			if totalCollected >= resp.TotalCount {
				// 已收集的数量达到总数，停止分页
				ctxLog.Debugf("SLB 分页采集完成 page=%d current_count=%d total_collected=%d total_count=%d",
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
		ctxLog.Debugf("SLB 分页采集 page=%d current_count=%d total_collected=%d", page, currentCount, len(ids))
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
					status := common.ClassifyAliyunError(err)
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
			status := common.ClassifyAliyunError(callErr)
			metrics.RequestTotal.WithLabelValues("aliyun", "ListTagResources", status).Inc()
			metrics.RecordRequest("aliyun", "ListTagResources", status)
			if status == "limit_error" {
				// 记录限流指标
				metrics.RateLimitTotal.WithLabelValues("aliyun", "ListTagResources").Inc()
			}
			if status == "auth_error" {
				break
			}
			// 指数退避重试
			sleep := time.Duration(200*(1<<attempt)) * time.Millisecond
			if sleep > 5*time.Second {
				sleep = 5 * time.Second
			}
			time.Sleep(sleep)
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
							// 提取带宽上限 (BandwidthCapBps) 标签，格式为数字字符串
							if strings.EqualFold(k, "BandwidthCapBps") || strings.EqualFold(k, "bandwidth_cap_bps") {
								// 使用特殊前缀存储，以便后续解析
								out["_cap_"+id] = v
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
				// 提取带宽上限 (BandwidthCapBps) 标签，格式为数字字符串
				if strings.EqualFold(k, "BandwidthCapBps") || strings.EqualFold(k, "bandwidth_cap_bps") {
					// 使用特殊前缀存储，以便后续解析
					out["_cap_"+id] = v
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
			// 提取带宽上限 (BandwidthCapBps) 标签，格式为数字字符串
			if strings.EqualFold(k, "BandwidthCapBps") || strings.EqualFold(k, "bandwidth_cap_bps") {
				// 使用特殊前缀存储，以便后续解析
				out["_cap_"+tr.ResourceId] = v
			}
		}
	}
	return out
}
