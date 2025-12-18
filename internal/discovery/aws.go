package discovery

import (
	"context"
	"strings"

	"multicloud-exporter/internal/config"
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
	accounts = append(accounts, cfg.AccountsList...)
	if cfg.AccountsByProvider != nil {
		if xs, ok := cfg.AccountsByProvider["aws"]; ok {
			accounts = append(accounts, xs...)
		}
	}
	if cfg.AccountsByProviderLegacy != nil {
		if xs, ok := cfg.AccountsByProviderLegacy["aws"]; ok {
			accounts = append(accounts, xs...)
		}
	}
	if len(accounts) == 0 {
		return nil
	}

	needS3 := false
	for _, acc := range accounts {
		for _, r := range acc.Resources {
			rr := strings.ToLower(strings.TrimSpace(r))
			if rr == "s3" || rr == "*" {
				needS3 = true
			}
		}
	}
	if !needS3 {
		return nil
	}

	// S3 的 CloudWatch 指标属于 AWS/S3，存储类指标依赖 StorageType 维度。
	// 这里选择“最稳定且可跨云对齐”的指标集合：
	// - 存储/对象数：稳定、口径清晰（通常为日粒度）
	// - 请求/字节/5xx/延迟：依赖 S3 Request Metrics（FilterId=EntireBucket）；若未启用则可能无数据
	return []config.Product{
		{
			Namespace:    "AWS/S3",
			AutoDiscover: true,
			MetricInfo: []config.MetricGroup{
				// Storage / objects (daily)
				{Period: intPtr(86400), MetricList: []string{"BucketSizeBytes", "NumberOfObjects"}},
				// Requests / bytes / errors / latency (minute-level, requires Request Metrics)
				{Period: intPtr(60), MetricList: []string{
					"AllRequests", "GetRequests", "PutRequests", "HeadRequests",
					"BytesUploaded", "BytesDownloaded",
					"5xxErrors",
					"FirstByteLatency",
				}},
			},
		},
	}
}

func intPtr(v int) *int { return &v }
