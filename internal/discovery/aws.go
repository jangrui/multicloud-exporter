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
		// 检查是否存在自定义配置
		var allMetricGroups [][]config.MetricGroup
		hasCustomConfig := false

		for _, acc := range accounts {
			if acc.ProductMetric != nil && len(acc.ProductMetric["s3"]) > 0 {
				hasCustomConfig = true
				customProduct := buildS3ProductWithCustomConfig(acc.ProductMetric["s3"])
				allMetricGroups = append(allMetricGroups, customProduct.MetricInfo)
			}
		}

		if hasCustomConfig {
			// 使用 union 策略合并多个账号的配置
			mergedMetricGroups := unionMetricGroups(allMetricGroups)
			prods = append(prods, config.Product{
				Namespace:    "AWS/S3",
				AutoDiscover: true,
				MetricInfo:   mergedMetricGroups,
			})
		} else {
			// 使用默认配置
			prods = append(prods, buildS3ProductDefault())
		}
	}

	if needALB {
		var allMetricGroups [][]config.MetricGroup
		hasCustomConfig := false

		for _, acc := range accounts {
			if acc.ProductMetric != nil && len(acc.ProductMetric["alb"]) > 0 {
				hasCustomConfig = true
				customProduct := buildAWSALBProductWithCustomConfig(acc.ProductMetric["alb"])
				allMetricGroups = append(allMetricGroups, customProduct.MetricInfo)
			}
		}

		if hasCustomConfig {
			mergedMetricGroups := unionMetricGroups(allMetricGroups)
			prods = append(prods, config.Product{
				Namespace:    "AWS/ApplicationELB",
				AutoDiscover: true,
				MetricInfo:   mergedMetricGroups,
			})
		} else {
			prods = append(prods, buildAWSALBProductDefault())
		}
	}

	if needCLB {
		var allMetricGroups [][]config.MetricGroup
		hasCustomConfig := false

		for _, acc := range accounts {
			if acc.ProductMetric != nil && len(acc.ProductMetric["clb"]) > 0 {
				hasCustomConfig = true
				customProduct := buildAWSCLBProductWithCustomConfig(acc.ProductMetric["clb"])
				allMetricGroups = append(allMetricGroups, customProduct.MetricInfo)
			}
		}

		if hasCustomConfig {
			mergedMetricGroups := unionMetricGroups(allMetricGroups)
			prods = append(prods, config.Product{
				Namespace:    "AWS/ELB",
				AutoDiscover: true,
				MetricInfo:   mergedMetricGroups,
			})
		} else {
			prods = append(prods, buildAWSCLBProductDefault())
		}
	}

	if needNLB {
		var allMetricGroups [][]config.MetricGroup
		hasCustomConfig := false

		for _, acc := range accounts {
			if acc.ProductMetric != nil && len(acc.ProductMetric["nlb"]) > 0 {
				hasCustomConfig = true
				customProduct := buildAWSNLBProductWithCustomConfig(acc.ProductMetric["nlb"])
				allMetricGroups = append(allMetricGroups, customProduct.MetricInfo)
			}
		}

		if hasCustomConfig {
			mergedMetricGroups := unionMetricGroups(allMetricGroups)
			prods = append(prods, config.Product{
				Namespace:    "AWS/NetworkELB",
				AutoDiscover: true,
				MetricInfo:   mergedMetricGroups,
			})
		} else {
			prods = append(prods, buildAWSNLBProductDefault())
		}
	}

	if needGWLB {
		var allMetricGroups [][]config.MetricGroup
		hasCustomConfig := false

		for _, acc := range accounts {
			if acc.ProductMetric != nil && len(acc.ProductMetric["gwlb"]) > 0 {
				hasCustomConfig = true
				customProduct := buildAWSGWLBProductWithCustomConfig(acc.ProductMetric["gwlb"])
				allMetricGroups = append(allMetricGroups, customProduct.MetricInfo)
			}
		}

		if hasCustomConfig {
			mergedMetricGroups := unionMetricGroups(allMetricGroups)
			prods = append(prods, config.Product{
				Namespace:    "AWS/GatewayELB",
				AutoDiscover: true,
				MetricInfo:   mergedMetricGroups,
			})
		} else {
			prods = append(prods, buildAWSGWLBProductDefault())
		}
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

// buildS3ProductDefault 返回默认的 S3 产品配置
func buildS3ProductDefault() config.Product {
	return config.Product{
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
	}
}

// buildS3ProductWithCustomConfig 从 ProductMetric 构建 S3 产品配置
func buildS3ProductWithCustomConfig(metricGroups []config.MetricGroupConfig) config.Product {
	var metricInfo []config.MetricGroup
	for _, mg := range metricGroups {
		metricInfo = append(metricInfo, config.MetricGroup{
			Period:     mg.Period,
			MetricList: mg.MetricList,
		})
	}
	return config.Product{
		Namespace:    "AWS/S3",
		AutoDiscover: true,
		MetricInfo:   metricInfo,
	}
}

func buildAWSALBProductDefault() config.Product {
	return config.Product{
		Namespace:    "AWS/ApplicationELB",
		AutoDiscover: true,
		MetricInfo: []config.MetricGroup{
			{Period: intPtr(60), MetricList: []string{
				"ActiveConnectionCount", "NewConnectionCount", "RejectedConnectionCount",
				"ProcessedBytes", "RequestCount",
				"TargetResponseTime", "HTTPCode_Target_2XX_Count", "HTTPCode_Target_3XX_Count",
				"HTTPCode_Target_4XX_Count", "HTTPCode_Target_5XX_Count",
			}},
		},
	}
}

func buildAWSALBProductWithCustomConfig(metricGroups []config.MetricGroupConfig) config.Product {
	var metricInfo []config.MetricGroup
	for _, mg := range metricGroups {
		metricInfo = append(metricInfo, config.MetricGroup{
			Period:     mg.Period,
			MetricList: mg.MetricList,
		})
	}
	return config.Product{
		Namespace:    "AWS/ApplicationELB",
		AutoDiscover: true,
		MetricInfo:   metricInfo,
	}
}

func buildAWSCLBProductDefault() config.Product {
	return config.Product{
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
	}
}

func buildAWSCLBProductWithCustomConfig(metricGroups []config.MetricGroupConfig) config.Product {
	var metricInfo []config.MetricGroup
	for _, mg := range metricGroups {
		metricInfo = append(metricInfo, config.MetricGroup{
			Period:     mg.Period,
			MetricList: mg.MetricList,
		})
	}
	return config.Product{
		Namespace:    "AWS/ELB",
		AutoDiscover: true,
		MetricInfo:   metricInfo,
	}
}

func buildAWSNLBProductDefault() config.Product {
	return config.Product{
		Namespace:    "AWS/NetworkELB",
		AutoDiscover: true,
		MetricInfo: []config.MetricGroup{
			{Period: intPtr(60), MetricList: []string{
				"ActiveFlowCount", "NewFlowCount", "ProcessedBytes",
				"TCP_Client_Reset_Count", "TCP_ELB_Reset_Count", "TCP_Target_Reset_Count",
				"HealthyHostCount", "UnHealthyHostCount",
			}},
		},
	}
}

func buildAWSNLBProductWithCustomConfig(metricGroups []config.MetricGroupConfig) config.Product {
	var metricInfo []config.MetricGroup
	for _, mg := range metricGroups {
		metricInfo = append(metricInfo, config.MetricGroup{
			Period:     mg.Period,
			MetricList: mg.MetricList,
		})
	}
	return config.Product{
		Namespace:    "AWS/NetworkELB",
		AutoDiscover: true,
		MetricInfo:   metricInfo,
	}
}

func buildAWSGWLBProductDefault() config.Product {
	return config.Product{
		Namespace:    "AWS/GatewayELB",
		AutoDiscover: true,
		MetricInfo: []config.MetricGroup{
			{Period: intPtr(60), MetricList: []string{
				"ActiveFlowCount", "NewFlowCount", "ProcessedBytes",
				"HealthyHostCount", "UnHealthyHostCount",
			}},
		},
	}
}

func buildAWSGWLBProductWithCustomConfig(metricGroups []config.MetricGroupConfig) config.Product {
	var metricInfo []config.MetricGroup
	for _, mg := range metricGroups {
		metricInfo = append(metricInfo, config.MetricGroup{
			Period:     mg.Period,
			MetricList: mg.MetricList,
		})
	}
	return config.Product{
		Namespace:    "AWS/GatewayELB",
		AutoDiscover: true,
		MetricInfo:   metricInfo,
	}
}

// unionMetricGroups 合并多个账号的指标配置（union 策略）
func unionMetricGroups(allProducts [][]config.MetricGroup) []config.MetricGroup {
	seen := make(map[string]bool)
	var result []config.MetricGroup

	for _, products := range allProducts {
		for _, mg := range products {
			// 检查是否已存在相同 Period 和 MetricList 的组合
			key := ""
			for _, m := range mg.MetricList {
				key += m + ","
			}
			periodKey := ""
			if mg.Period != nil {
				periodKey = string(rune(*mg.Period))
			}
			fullKey := key + periodKey

			if !seen[fullKey] {
				seen[fullKey] = true
				result = append(result, mg)
			}
		}
	}
	return result
}
