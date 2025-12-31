package discovery

import (
	"context"
	"strings"

	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/logger"
)

// AWSDiscoverer 基于 accounts.yaml 中的 resources 决定需要启用的 AWS 命名空间。
// AWS 的指标列表（MetricInfo）目前采用最小可用集合（兜底），避免依赖 CloudWatch ListMetrics 的高成本扫描。
type AWSDiscoverer struct{}

func (d *AWSDiscoverer) Discover(ctx context.Context, cfg *config.Config) []config.Product {
	_ = ctx
	if cfg == nil {
		return nil
	}
	var accounts []config.CloudAccount
	if cfg.AccountsByProvider != nil {
		if xs, ok := cfg.AccountsByProvider["aws"]; ok {
			accounts = append(accounts, xs...)
		}
	}
	if len(accounts) == 0 {
		return nil
	}

	needS3 := false
	needALB := false
	needCLB := false
	needNLB := false
	needGWLB := false

	for _, acc := range accounts {
		for _, r := range acc.Resources {
			rr := strings.ToLower(strings.TrimSpace(r))
			switch rr {
			case "*":
				needS3 = true
				needALB = true
				needCLB = true
				needNLB = true
				needGWLB = true
			case "s3":
				needS3 = true
			case "alb":
				needALB = true
			case "clb":
				needCLB = true
			case "nlb":
				needNLB = true
			case "gwlb":
				needGWLB = true
			}
		}
	}

	var prods []config.Product

	if needS3 {
		// S3 的 CloudWatch 指标属于 AWS/S3，存储类指标依赖 StorageType 维度。
		// 这里选择"最稳定且可跨云对齐"的指标集合：
		// - 存储/对象数：稳定、口径清晰（通常为日粒度）
		// - 请求/字节/错误/延迟：依赖 S3 Request Metrics（FilterId=EntireBucket）；若未启用则可能无数据
		prods = append(prods, config.Product{
			Namespace:    "AWS/S3",
			AutoDiscover: true,
			MetricInfo: []config.MetricGroup{
				// Storage / objects (daily)
				{Period: intPtr(86400), MetricList: []string{"BucketSizeBytes", "NumberOfObjects"}},
				// Requests / bytes / errors / latency (minute-level, requires Request Metrics)
				{Period: intPtr(60), MetricList: []string{
					"AllRequests", "GetRequests", "PutRequests", "HeadRequests", "ListRequests", "PostRequests",
					"BytesUploaded", "BytesDownloaded",
					"4xxErrors", "5xxErrors",
					"FirstByteLatency", "TotalRequestLatency",
				}},
			},
		})
	}

	if needALB {
		prods = append(prods, config.Product{
			Namespace:    "AWS/ApplicationELB",
			AutoDiscover: true,
			MetricInfo: []config.MetricGroup{
				{Period: intPtr(60), MetricList: []string{
					"ActiveConnectionCount", "NewConnectionCount", "RejectedConnectionCount",
					"ProcessedBytes", "RequestCount",
					"TargetResponseTime", "HTTPCode_Target_2XX_Count", "HTTPCode_Target_3XX_Count",
					"HTTPCode_Target_4XX_Count", "HTTPCode_Target_5XX_Count",
					// "UnHealthyHostCount", "HealthyHostCount", // Requires TargetGroup dimension, not supported at LoadBalancer level for ALB
				}},
			},
		})
	}

	if needCLB {
		prods = append(prods, config.Product{
			Namespace:    "AWS/ELB",
			AutoDiscover: true,
			MetricInfo: []config.MetricGroup{
				{Period: intPtr(60), MetricList: []string{
					"RequestCount", "Latency",
					"HTTPCode_Backend_2XX", "HTTPCode_Backend_3XX", "HTTPCode_Backend_4XX", "HTTPCode_Backend_5XX",
					"SurgeQueueLength", "SpilloverCount",
					"HealthyHostCount", "UnHealthyHostCount",
				}},
			},
		})
	}

	if needNLB {
		prods = append(prods, config.Product{
			Namespace:    "AWS/NetworkELB",
			AutoDiscover: true,
			MetricInfo: []config.MetricGroup{
				{Period: intPtr(60), MetricList: []string{
					"ActiveFlowCount", "NewFlowCount", "ProcessedBytes",
					"TCP_Client_Reset_Count", "TCP_ELB_Reset_Count", "TCP_Target_Reset_Count",
					"HealthyHostCount", "UnHealthyHostCount",
				}},
			},
		})
	}

	if needGWLB {
		prods = append(prods, config.Product{
			Namespace:    "AWS/GatewayELB",
			AutoDiscover: true,
			MetricInfo: []config.MetricGroup{
				{Period: intPtr(60), MetricList: []string{
					"ActiveFlowCount", "NewFlowCount", "ProcessedBytes",
					"HealthyHostCount", "UnHealthyHostCount",
				}},
			},
		})
	}

	if len(prods) == 0 {
		return nil
	}

	totalMetrics := 0
	for _, p := range prods {
		for _, g := range p.MetricInfo {
			totalMetrics += len(g.MetricList)
		}
	}
	ctxLog := logger.NewContextLogger("AWS", "resource_type", "Discovery")
	ctxLog.Infof("发现服务完成，产品数=%d，指标总数=%d", len(prods), totalMetrics)
	return prods
}

func intPtr(v int) *int { return &v }
