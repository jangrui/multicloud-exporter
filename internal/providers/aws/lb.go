package aws

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/logger"
	"multicloud-exporter/internal/metrics"
	"multicloud-exporter/internal/providers/common"
	"multicloud-exporter/internal/utils"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancing"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
)

// ResourceLister 定义列出 AWS 资源的接口
type ResourceLister interface {
	List(ctx context.Context, region string, account config.CloudAccount) ([]lbInfo, error)
}

// lbInfo 表示负载均衡器的通用信息
type lbInfo struct {
	Name     string
	ARN      string // 用于 v2
	CodeName string // 从标签解析
}

// clbLister 实现 ResourceLister 接口，用于经典负载均衡器
type clbLister struct {
	c *Collector
}

func (l *clbLister) List(ctx context.Context, region string, account config.CloudAccount) ([]lbInfo, error) {
	client, err := l.c.clientFactory.NewELBClient(ctx, region, account.AccessKeyID, account.AccessKeySecret)
	if err != nil {
		if l.c.degradeMgr != nil {
			regionKey := account.Provider + ":" + account.AccountID + ":" + region
			l.c.degradeMgr.RecordFailure(regionKey, common.ResourceTypeRegion, err.Error())
		}
		return nil, err
	}
	var lbs []lbInfo
	regionKey := account.Provider + ":" + account.AccountID + ":" + region
	if l.c.degradeMgr != nil && l.c.degradeMgr.IsDisabled(regionKey, common.ResourceTypeRegion) {
		return []lbInfo{}, nil
	}
	// AWS SDK Paginator 自动处理分页：HasMorePages() 检查是否还有更多页，NextPage() 获取下一页
	// 边界情况处理：
	// - 空结果：HasMorePages() 返回 false，不会进入循环
	// - 单页结果：HasMorePages() 返回 true 一次，然后 false
	// - 多页结果：HasMorePages() 返回 true 直到所有页都被获取
	paginator := elasticloadbalancing.NewDescribeLoadBalancersPaginator(client, &elasticloadbalancing.DescribeLoadBalancersInput{})
	for paginator.HasMorePages() {
		start := time.Now()
		page, err := paginator.NextPage(ctx)
		if err != nil {
			status := common.ClassifyAWSError(err)
			metrics.RequestTotal.WithLabelValues("aws", "DescribeLoadBalancers", status).Inc()
			metrics.RecordRequest("aws", "DescribeLoadBalancers", status)
			metrics.RequestDuration.WithLabelValues("aws", "DescribeLoadBalancers").Observe(time.Since(start).Seconds())
			if status == "limit_error" {
				metrics.RateLimitTotal.WithLabelValues("aws", "DescribeLoadBalancers").Inc()
			}
			if l.c.degradeMgr != nil && status == "auth_error" {
				disabled := l.c.degradeMgr.RecordFailure(regionKey, common.ResourceTypeRegion, err.Error())
				if disabled {
					ctxLog := logger.NewContextLogger("AWS", "account_id", account.AccountID, "region", region)
					ctxLog.Warn("区域已降级")
				}
			}
			// API 调用失败时，返回已收集的数据和错误，允许上层决定如何处理
			return lbs, err
		}
		metrics.RequestTotal.WithLabelValues("aws", "DescribeLoadBalancers", "success").Inc()
		metrics.RecordRequest("aws", "DescribeLoadBalancers", "success")
		metrics.RequestDuration.WithLabelValues("aws", "DescribeLoadBalancers").Observe(time.Since(start).Seconds())
		if l.c.degradeMgr != nil {
			l.c.degradeMgr.RecordSuccess(regionKey, common.ResourceTypeRegion)
		}
		for _, lb := range page.LoadBalancerDescriptions {
			if lb.LoadBalancerName != nil {
				lbs = append(lbs, lbInfo{Name: *lb.LoadBalancerName, CodeName: *lb.LoadBalancerName})
			}
		}
	}

	// 获取 CLB 标签
	if len(lbs) > 0 {
		var names []string
		lbMap := make(map[string]*lbInfo)
		for i := range lbs {
			names = append(names, lbs[i].Name)
			lbMap[lbs[i].Name] = &lbs[i]
		}

		// 批量获取标签（限制 20 个）
		for i := 0; i < len(names); i += 20 {
			end := i + 20
			if end > len(names) {
				end = len(names)
			}
			batch := names[i:end]
			start := time.Now()
			out, err := client.DescribeTags(ctx, &elasticloadbalancing.DescribeTagsInput{
				LoadBalancerNames: batch,
			})
			if err != nil {
				status := common.ClassifyAWSError(err)
				metrics.RequestTotal.WithLabelValues("aws", "DescribeTags", status).Inc()
				metrics.RecordRequest("aws", "DescribeTags", status)
				metrics.RequestDuration.WithLabelValues("aws", "DescribeTags").Observe(time.Since(start).Seconds())
				if status == "limit_error" {
					metrics.RateLimitTotal.WithLabelValues("aws", "DescribeTags").Inc()
				}
				ctxLog := logger.NewContextLogger("AWS", "account_id", account.AccountID, "region", region, "resource_type", "CLB")
				ctxLog.Warnf("DescribeTags API调用失败: %v", err)
				continue
			}
			metrics.RequestTotal.WithLabelValues("aws", "DescribeTags", "success").Inc()
			metrics.RecordRequest("aws", "DescribeTags", "success")
			metrics.RequestDuration.WithLabelValues("aws", "DescribeTags").Observe(time.Since(start).Seconds())
			for _, desc := range out.TagDescriptions {
				if desc.LoadBalancerName != nil {
					if info, ok := lbMap[*desc.LoadBalancerName]; ok {
						tags := make(map[string]string)
						for _, t := range desc.Tags {
							if t.Key != nil && t.Value != nil {
								tags[*t.Key] = *t.Value
							}
						}
						info.CodeName = resolveCodeName(tags, info.Name)
					}
				}
			}
		}
	}

	return lbs, nil
}

// elbv2Lister 实现 ResourceLister 接口，用于 ALB、NLB 和 GWLB
type elbv2Lister struct {
	c      *Collector
	lbType elbv2types.LoadBalancerTypeEnum
}

func (l *elbv2Lister) List(ctx context.Context, region string, account config.CloudAccount) ([]lbInfo, error) {
	client, err := l.c.clientFactory.NewELBv2Client(ctx, region, account.AccessKeyID, account.AccessKeySecret)
	if err != nil {
		if l.c.degradeMgr != nil {
			regionKey := account.Provider + ":" + account.AccountID + ":" + region
			l.c.degradeMgr.RecordFailure(regionKey, common.ResourceTypeRegion, err.Error())
		}
		return nil, err
	}
	var lbs []lbInfo
	regionKey := account.Provider + ":" + account.AccountID + ":" + region
	if l.c.degradeMgr != nil && l.c.degradeMgr.IsDisabled(regionKey, common.ResourceTypeRegion) {
		return []lbInfo{}, nil
	}
	// AWS SDK Paginator 自动处理分页：HasMorePages() 检查是否还有更多页，NextPage() 获取下一页
	// 边界情况处理：
	// - 空结果：HasMorePages() 返回 false，不会进入循环
	// - 单页结果：HasMorePages() 返回 true 一次，然后 false
	// - 多页结果：HasMorePages() 返回 true 直到所有页都被获取
	paginator := elasticloadbalancingv2.NewDescribeLoadBalancersPaginator(client, &elasticloadbalancingv2.DescribeLoadBalancersInput{})
	for paginator.HasMorePages() {
		start := time.Now()
		page, err := paginator.NextPage(ctx)
		if err != nil {
			status := common.ClassifyAWSError(err)
			metrics.RequestTotal.WithLabelValues("aws", "DescribeLoadBalancers", status).Inc()
			metrics.RecordRequest("aws", "DescribeLoadBalancers", status)
			metrics.RequestDuration.WithLabelValues("aws", "DescribeLoadBalancers").Observe(time.Since(start).Seconds())
			if status == "limit_error" {
				metrics.RateLimitTotal.WithLabelValues("aws", "DescribeLoadBalancers").Inc()
			}
			if l.c.degradeMgr != nil && status == "auth_error" {
				disabled := l.c.degradeMgr.RecordFailure(regionKey, common.ResourceTypeRegion, err.Error())
				if disabled {
					ctxLog := logger.NewContextLogger("AWS", "account_id", account.AccountID, "region", region)
					ctxLog.Warn("区域已降级")
				}
			}
			// API 调用失败时，返回已收集的数据和错误，允许上层决定如何处理
			return lbs, err
		}
		metrics.RequestTotal.WithLabelValues("aws", "DescribeLoadBalancers", "success").Inc()
		metrics.RecordRequest("aws", "DescribeLoadBalancers", "success")
		metrics.RequestDuration.WithLabelValues("aws", "DescribeLoadBalancers").Observe(time.Since(start).Seconds())
		if l.c.degradeMgr != nil {
			l.c.degradeMgr.RecordSuccess(regionKey, common.ResourceTypeRegion)
		}
		for _, lb := range page.LoadBalancers {
			if lb.Type == l.lbType && lb.LoadBalancerName != nil && lb.LoadBalancerArn != nil {
				lbs = append(lbs, lbInfo{Name: *lb.LoadBalancerName, ARN: *lb.LoadBalancerArn, CodeName: *lb.LoadBalancerName})
			}
		}
	}
	ctxLog := logger.NewContextLogger("AWS", "account_id", account.AccountID, "region", region, "resource_type", string(l.lbType))
	ctxLog.Debugf("发现负载均衡器，数量=%d，类型=%s，区域=%s", len(lbs), l.lbType, region)

	// 获取 ELBv2 标签
	if len(lbs) > 0 {
		var arns []string
		lbMap := make(map[string]*lbInfo)
		for i := range lbs {
			arns = append(arns, lbs[i].ARN)
			lbMap[lbs[i].ARN] = &lbs[i]
		}

		// 批量获取标签（限制 20 个）
		for i := 0; i < len(arns); i += 20 {
			end := i + 20
			if end > len(arns) {
				end = len(arns)
			}
			batch := arns[i:end]
			start := time.Now()
			out, err := client.DescribeTags(ctx, &elasticloadbalancingv2.DescribeTagsInput{
				ResourceArns: batch,
			})
			if err != nil {
				status := common.ClassifyAWSError(err)
				metrics.RequestTotal.WithLabelValues("aws", "DescribeTags", status).Inc()
				metrics.RecordRequest("aws", "DescribeTags", status)
				metrics.RequestDuration.WithLabelValues("aws", "DescribeTags").Observe(time.Since(start).Seconds())
				if status == "limit_error" {
					metrics.RateLimitTotal.WithLabelValues("aws", "DescribeTags").Inc()
				}
				ctxLog := logger.NewContextLogger("AWS", "account_id", account.AccountID, "region", region, "resource_type", string(l.lbType))
				ctxLog.Warnf("DescribeTags API调用失败: %v", err)
				continue
			}
			metrics.RequestTotal.WithLabelValues("aws", "DescribeTags", "success").Inc()
			metrics.RecordRequest("aws", "DescribeTags", "success")
			metrics.RequestDuration.WithLabelValues("aws", "DescribeTags").Observe(time.Since(start).Seconds())
			for _, desc := range out.TagDescriptions {
				if desc.ResourceArn != nil {
					if info, ok := lbMap[*desc.ResourceArn]; ok {
						tags := make(map[string]string)
						for _, t := range desc.Tags {
							if t.Key != nil && t.Value != nil {
								tags[*t.Key] = *t.Value
							}
						}
						info.CodeName = resolveCodeName(tags, info.Name)
					}
				}
			}
		}
	}

	return lbs, nil
}

func resolveCodeName(tags map[string]string, defaultName string) string {
	// 优先使用 k8s service name，因为它通常包含业务信息 (namespace/service)
	if v, ok := tags["kubernetes.io/service-name"]; ok && v != "" {
		return v
	}
	if v, ok := tags["Name"]; ok && v != "" {
		return v
	}
	return defaultName
}

func (c *Collector) collectCLB(account config.CloudAccount) {
	c.collectLBGeneric(account, "AWS/ELB", &clbLister{c: c})
}

func (c *Collector) collectALB(account config.CloudAccount) {
	c.collectLBGeneric(account, "AWS/ApplicationELB", &elbv2Lister{c: c, lbType: elbv2types.LoadBalancerTypeEnumApplication})
}

func (c *Collector) collectNLB(account config.CloudAccount) {
	c.collectLBGeneric(account, "AWS/NetworkELB", &elbv2Lister{c: c, lbType: elbv2types.LoadBalancerTypeEnumNetwork})
}

func (c *Collector) collectGWLB(account config.CloudAccount) {
	c.collectLBGeneric(account, "AWS/GatewayELB", &elbv2Lister{c: c, lbType: elbv2types.LoadBalancerTypeEnumGateway})
}

func (c *Collector) getProductConfig(namespace string) *config.Product {
	if c.disc == nil {
		return nil
	}
	if ps, ok := c.disc.Get()["aws"]; ok {
		for i := range ps {
			if ps[i].Namespace == namespace {
				return &ps[i]
			}
		}
	}
	return nil
}

func (c *Collector) collectLBGeneric(account config.CloudAccount, namespace string, lister ResourceLister) {
	prod := c.getProductConfig(namespace)
	if prod == nil {
		return
	}

	// 产品级分片：获取集群配置用于产品级分片判断
	wTotal, wIndex := utils.ClusterConfig()

	var wg sync.WaitGroup
	// 限制区域并发数
	sem := make(chan struct{}, 5)

	regions := account.Regions
	if len(regions) == 0 || (len(regions) == 1 && regions[0] == "*") {
		regions = c.getAllRegions(account)
	}

	for _, region := range regions {
		// 产品级分片判断：只有当前 Pod 应该处理的产品才进行采集
		// 分片键格式：AccountID|Region|Namespace
		productKey := account.AccountID + "|" + region + "|" + namespace
		if !utils.ShouldProcess(productKey, wTotal, wIndex) {
			ctxLog := logger.NewContextLogger("AWS", "account_id", account.AccountID, "region", region, "namespace", namespace)
			ctxLog.Debugf("产品跳过（分片不匹配）")
			continue
		}
		wg.Add(1)
		// 限制并发数：必须在 go func 外部获取信号量
		sem <- struct{}{}
		go func(region string) {
			defer wg.Done()
			defer func() { <-sem }()
			// 捕获 panic
			defer func() {
				if r := recover(); r != nil {
					logger.NewContextLogger("AWS", "account_id", account.AccountID, "region", region, "namespace", namespace).Errorf("LB collection panic: %v", r)
				}
			}()
			c.processRegionLB(account, region, prod, lister)
		}(region)
	}
	wg.Wait()
}

func (c *Collector) processRegionLB(account config.CloudAccount, region string, prod *config.Product, lister ResourceLister) {
	// 创建上下文用于 LB 采集，设置 60s 超时防止挂起
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// 获取 LB 产品的 RegionManager
	lbRM := c.getProductRegionManager(AWSProductLB)
	if lbRM != nil && lbRM.ShouldSkipRegion(account.AccountID, region) {
		ctxLog := logger.NewContextLogger("AWS", "account_id", account.AccountID, "region", region, "namespace", prod.Namespace)
		ctxLog.Debugf("LB 采集 - 区域已跳过（产品级 RegionManager）")
		return
	}

	lbs, err := lister.List(ctx, region, account)
	if err != nil {
		ctxLog := logger.NewContextLogger("AWS", "account_id", account.AccountID, "region", region, "namespace", prod.Namespace)
		ctxLog.Errorf("ListLB API调用失败: %v", err)
		return
	}

	// 更新产品级区域状态
	if lbRM != nil {
		status := common.RegionStatusEmpty
		if len(lbs) > 0 {
			status = common.RegionStatusActive
		}
		lbRM.UpdateRegionStatus(account.AccountID, region, len(lbs), status)
		ctxLog := logger.NewContextLogger("AWS", "account_id", account.AccountID, "region", region, "namespace", prod.Namespace)
		ctxLog.Debugf("更新 LB 区域状态，status=%s，count=%d", status, len(lbs))
	}

	if len(lbs) == 0 {
		return
	}

	cwClient, err := c.clientFactory.NewCloudWatchClient(ctx, region, account.AccessKeyID, account.AccessKeySecret)
	if err != nil {
		ctxLog := logger.NewContextLogger("AWS", "account_id", account.AccountID, "region", region, "namespace", prod.Namespace)
		ctxLog.Errorf("CloudWatch客户端创建失败: %v", err)
		return
	}

	// 批量指标采集
	// CloudWatch GetMetricData 每次请求最多支持 500 个指标
	// 我们有 N 个 LB * M 个指标
	// 需要批量查询

	var queries []cwtypes.MetricDataQuery
	var queryMap = make(map[string]struct {
		LBName     string
		MetricName string
		Stat       string
		CodeName   string
	})

	period := int32(60) // 默认 60s
	// 尝试从配置中找到最小周期或使用默认值
	// 这里简单使用 60s 或配置默认值
	// 未来应尊重映射配置周期

	now := time.Now()
	endTime := now
	startTime := now.Add(-time.Duration(period) * time.Second)

	// 构建查询
	for _, lb := range lbs {
		for _, mGroup := range prod.MetricInfo {
			for _, metricName := range mGroup.MetricList {
				// ID 必须以小写字母开头，仅包含字母数字字符和下划线
				id := fmt.Sprintf("q%d", len(queries))

				var dims []cwtypes.Dimension
				// 映射维度
				// CLB: LoadBalancerName
				// ALB/NLB/GWLB: LoadBalancer (ARN 后缀)
				// 对于 ALB/NLB，维度是 "LoadBalancer"。值是 ARN 的 "app/my-load-balancer/50dc6c495c0c9188" 部分
				// 对于 CLB，维度是 "LoadBalancerName"。值是 Name
				// ARN 格式: arn:aws:elasticloadbalancing:region:account-id:loadbalancer/app/my-load-balancer/50dc6c495c0c9188

				dimValue := lb.Name
				dimName := "LoadBalancerName"
				if prod.Namespace != "AWS/ELB" {
					dimName = "LoadBalancer"
					// 对于 v2，值是 ARN 的资源 ID 部分
					parts := strings.Split(lb.ARN, ":loadbalancer/")
					if len(parts) == 2 {
						dimValue = parts[1]
					} else {
						dimValue = lb.Name
					}
				}

				dims = append(dims, cwtypes.Dimension{
					Name:  aws.String(dimName),
					Value: aws.String(dimValue),
				})

				stat := "Sum" // 默认
				// 确定统计方式 (Sum, Average, Max, SampleCount)
				// 通常计数/字节用 Sum，延迟/并发用 Average
				if strings.Contains(metricName, "ActiveConnection") || strings.Contains(metricName, "ActiveFlow") || strings.Contains(metricName, "Latency") || strings.Contains(metricName, "Time") || strings.Contains(metricName, "HostCount") {
					stat = "Average"
				}

				// 初始化 gauge 为 0，确保即使 CloudWatch 没有返回数据也能暴露指标
				vec, labelCount := metrics.NamespaceGauge(prod.Namespace, metricName)
				codeName := lb.CodeName
				if codeName == "" {
					codeName = lb.Name
				}

				// 构建标签值数组以匹配预期的标签数量
				labelValues := []string{
					"aws",
					account.AccountID,
					region,
					metrics.GetNamespacePrefix(prod.Namespace),
					lb.Name,
					prod.Namespace,
					metricName,
					codeName,
				}

				// 如果预期有更多标签（用于额外维度），用空字符串填充
				for len(labelValues) < labelCount {
					labelValues = append(labelValues, "")
				}

				vec.WithLabelValues(labelValues...).Set(0)

				queries = append(queries, cwtypes.MetricDataQuery{
					Id: aws.String(id),
					MetricStat: &cwtypes.MetricStat{
						Metric: &cwtypes.Metric{
							Namespace:  aws.String(prod.Namespace),
							MetricName: aws.String(metricName),
							Dimensions: dims,
						},
						Period: aws.Int32(period),
						Stat:   aws.String(stat),
					},
				})
				queryMap[id] = struct {
					LBName     string
					MetricName string
					Stat       string
					CodeName   string
				}{LBName: lb.Name, MetricName: metricName, Stat: stat, CodeName: lb.CodeName}
			}
		}
	}

	// 分批执行查询（每批 500 个）
	batchSize := 500
	for i := 0; i < len(queries); i += batchSize {
		end := i + batchSize
		if end > len(queries) {
			end = len(queries)
		}
		batch := queries[i:end]

		input := &cloudwatch.GetMetricDataInput{
			MetricDataQueries: batch,
			StartTime:         aws.Time(startTime),
			EndTime:           aws.Time(endTime),
		}

		start := time.Now()
		out, err := cwClient.GetMetricData(ctx, input)
		if err != nil {
			status := common.ClassifyAWSError(err)
			metrics.RequestTotal.WithLabelValues("aws", "GetMetricData", status).Inc()
			metrics.RecordRequest("aws", "GetMetricData", status)
			metrics.RequestDuration.WithLabelValues("aws", "GetMetricData").Observe(time.Since(start).Seconds())
			if status == "limit_error" {
				metrics.RateLimitTotal.WithLabelValues("aws", "GetMetricData").Inc()
			}
			ctxLog := logger.NewContextLogger("AWS", "account_id", account.AccountID, "region", region, "namespace", prod.Namespace)
			ctxLog.Warnf("GetMetricData API调用失败: %v", err)
			continue
		}
		metrics.RequestTotal.WithLabelValues("aws", "GetMetricData", "success").Inc()
		metrics.RecordRequest("aws", "GetMetricData", "success")
		metrics.RequestDuration.WithLabelValues("aws", "GetMetricData").Observe(time.Since(start).Seconds())

		if len(out.MetricDataResults) == 0 {
			ctxLog := logger.NewContextLogger("AWS", "account_id", account.AccountID, "region", region, "namespace", prod.Namespace)
			ctxLog.Warnf("GetMetricData API调用返回0结果")
		}

		for _, result := range out.MetricDataResults {
			if len(result.Values) > 0 {
				info, ok := queryMap[*result.Id]
				if ok {
					val := result.Values[0] // Take the latest

					// If the statistic is Sum (e.g. RequestCount, ProcessedBytes), CloudWatch returns the total over the period.
					// We typically want a rate (per second) for Prometheus gauges.
					if info.Stat == "Sum" && period > 0 {
						val = val / float64(period)
					}

					// Apply scale if needed
					scale := metrics.GetMetricScale(prod.Namespace, info.MetricName)
					if scale != 0 && scale != 1 {
						val = val * scale
					}

					// Get GaugeVec
					vec, labelCount := metrics.NamespaceGauge(prod.Namespace, info.MetricName)

					// Set labels: cloud_provider, account_id, region, resource_type, resource_id, namespace, metric_name, code_name
					codeName := info.CodeName
					if codeName == "" {
						codeName = info.LBName
					}

					// Build label values array matching the expected label count
					labelValues := []string{
						"aws",
						account.AccountID,
						region,
						metrics.GetNamespacePrefix(prod.Namespace),
						info.LBName,
						prod.Namespace,
						info.MetricName,
						codeName,
					}

					// Pad with empty strings if more labels are expected (for extra dimensions)
					for len(labelValues) < labelCount {
						labelValues = append(labelValues, "")
					}

					vec.WithLabelValues(labelValues...).Set(val)
				}
			}
		}
	}
}
