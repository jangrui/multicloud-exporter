package aws

import (
	"context"
	"strings"

	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/logger"
	"multicloud-exporter/internal/providers"

	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"go.uber.org/zap"
)

func init() {
	providers.RegisterFourDimensionAdapter("aws", func(collector providers.Provider, logger *zap.Logger) providers.FourDimensionAdapter {
		c, ok := collector.(*Collector)
		if !ok {
			return nil
		}
		return NewFourDimensionAdapter(c, logger)
	})
}

// FourDimensionAdapter AWS 四维架构适配器
type FourDimensionAdapter struct {
	collector *Collector
	logger    *zap.Logger
}

// NewFourDimensionAdapter 创建 AWS 四维架构适配器
func NewFourDimensionAdapter(collector *Collector, logger *zap.Logger) *FourDimensionAdapter {
	return &FourDimensionAdapter{
		collector: collector,
		logger:    logger,
	}
}

// CollectAccountMetrics 采集账号级指标
func (a *FourDimensionAdapter) CollectAccountMetrics(ctx context.Context, account config.CloudAccount) error {
	ctxLog := logger.NewContextLogger("AWSAdapter", "account_id", account.AccountID)
	ctxLog.Info("开始采集账号级指标")

	// 注意：分片逻辑已下沉到产品级（collectS3/collectALB 等），此处不做账号级分片
	// 这样可以避免双重分片导致的任务丢失问题
	for _, resource := range account.Resources {
		r := strings.ToLower(strings.TrimSpace(resource))
		switch r {
		case "*":
			a.collector.collectS3(account)
			a.collector.collectALB(account)
			a.collector.collectCLB(account)
			a.collector.collectNLB(account)
			a.collector.collectGWLB(account)
		case "s3":
			a.collector.collectS3(account)
		case "alb":
			a.collector.collectALB(account)
		case "clb":
			a.collector.collectCLB(account)
		case "nlb":
			a.collector.collectNLB(account)
		case "gwlb":
			a.collector.collectGWLB(account)
		default:
			ctxLog := logger.NewContextLogger("AWS", "account_id", account.AccountID, "resource_type", resource)
			ctxLog.Warnf("资源类型尚未实现")
		}
	}

	ctxLog.Info("完成账号级指标采集")
	return nil
}

// CollectProductMetrics 采集产品级指标
func (a *FourDimensionAdapter) CollectProductMetrics(ctx context.Context, account config.CloudAccount, productID string) error {
	ctxLog := logger.NewContextLogger("AWSAdapter", "account_id", account.AccountID, "product_id", productID)
	ctxLog.Info("开始采集产品级指标")

	switch productID {
	case AWSProductS3:
		a.collector.collectS3(account)
	case AWSProductLB, "clb":
		a.collector.collectCLB(account)
	case "alb":
		a.collector.collectALB(account)
	case "nlb":
		a.collector.collectNLB(account)
	case "gwlb":
		a.collector.collectGWLB(account)
	default:
		ctxLog.Warnf("未知的AWS产品ID，跳过采集")
	}

	ctxLog.Info("完成产品级指标采集")
	return nil
}

// CollectRegionMetrics 采集区域级指标
func (a *FourDimensionAdapter) CollectRegionMetrics(ctx context.Context, account config.CloudAccount, productID, region string) error {
	ctxLog := logger.NewContextLogger("AWSAdapter", "account_id", account.AccountID, "product_id", productID, "region", region)
	ctxLog.Info("开始采集区域级指标")

	// 创建临时账号对象，仅包含当前区域，确保底层 collector 只采集该区域
	regionAccount := account
	regionAccount.Regions = []string{region}

	switch productID {
	case AWSProductS3:
		a.collector.collectS3(regionAccount)
	case AWSProductLB:
		a.collector.collectCLB(regionAccount)
	}

	ctxLog.Info("完成区域级指标采集")
	return nil
}

// CollectResourceMetrics 采集资源级指标
func (a *FourDimensionAdapter) CollectResourceMetrics(ctx context.Context, account config.CloudAccount, productID, region, resourceID string) error {
	ctxLog := logger.NewContextLogger("AWSAdapter", "account_id", account.AccountID, "product_id", productID, "region", region, "resource_id", resourceID)
	ctxLog.Info("开始采集资源级指标")

	switch productID {
	case AWSProductS3:
		a.collector.collectS3(account)
	case AWSProductLB:
		a.collector.collectCLB(account)
	case "alb":
		a.collector.collectALB(account)
	case "nlb":
		a.collector.collectNLB(account)
	case "gwlb":
		a.collector.collectGWLB(account)
	default:
		ctxLog := logger.NewContextLogger("AWS", "account_id", account.AccountID, "resource_type", resourceID)
		ctxLog.Warnf("资源类型尚未实现")
	}

	ctxLog.Info("完成资源级指标采集")
	return nil
}

// DiscoverResources 发现该账号下所有资源
func (a *FourDimensionAdapter) DiscoverResources(ctx context.Context, account config.CloudAccount) (map[string]map[string][]string, error) {
	ctxLog := logger.NewContextLogger("AWSAdapter", "account_id", account.AccountID)
	ctxLog.Info("开始发现资源")

	result := make(map[string]map[string][]string)

	regions := a.collector.getAllRegions(account)
	if len(regions) == 0 {
		return result, nil
	}

	for _, region := range regions {
		regionResult := make(map[string][]string)

		for _, resource := range account.Resources {
			r := strings.ToLower(strings.TrimSpace(resource))
			if r == "*" {
				regionResult["s3"] = a.discoverS3Resources(account, region)
				regionResult["lb"] = a.discoverLBResources(account, region)
				continue
			}
			switch r {
			case "s3":
				ids := a.discoverS3Resources(account, region)
				regionResult["s3"] = ids
			case "lb", "clb", "alb", "nlb", "gwlb":
				ids := a.discoverLBResources(account, region)
				regionResult["lb"] = ids
			}
		}

		for productID, resourceIDs := range regionResult {
			if _, ok := result[productID]; !ok {
				result[productID] = make(map[string][]string)
			}
			result[productID][region] = resourceIDs
		}
	}

	ctxLog.Infof("完成资源发现 products=%d regions=%d", len(result), len(regions))
	return result, nil
}

// GetRegions 获取可用区域
func (a *FourDimensionAdapter) GetRegions(ctx context.Context, account config.CloudAccount) ([]string, error) {
	regions := account.Regions
	if len(regions) == 0 || (len(regions) == 1 && regions[0] == "*") {
		regions = a.collector.getAllRegions(account)
	}
	if len(regions) == 0 {
		return []string{}, nil
	}
	return regions, nil
}

// discoverS3Resources 发现 S3 资源
func (a *FourDimensionAdapter) discoverS3Resources(account config.CloudAccount, _ string) []string {
	// S3 is global, use us-east-1
	ctx := context.Background()
	client, err := a.collector.clientFactory.NewS3Client(ctx, "us-east-1", account.AccessKeyID, account.AccessKeySecret)
	if err != nil {
		a.logger.Warn("Failed to create S3 client for discovery", zap.Error(err))
		return []string{}
	}

	out, err := client.ListBuckets(ctx, &s3.ListBucketsInput{})
	if err != nil {
		a.logger.Warn("Failed to list S3 buckets for discovery", zap.Error(err))
		return []string{}
	}

	var buckets []string
	for _, b := range out.Buckets {
		if b.Name != nil {
			buckets = append(buckets, *b.Name)
		}
	}
	return buckets
}

// discoverLBResources 发现 LB 资源
func (a *FourDimensionAdapter) discoverLBResources(account config.CloudAccount, region string) []string {
	ctx := context.Background()
	var ids []string

	// 1. CLB
	clb := &clbLister{c: a.collector}
	if clbs, err := clb.List(ctx, region, account); err == nil {
		for _, lb := range clbs {
			ids = append(ids, lb.Name)
		}
	} else {
		a.logger.Warn("Failed to list CLBs for discovery", zap.String("region", region), zap.Error(err))
	}

	// 2. ALB
	alb := &elbv2Lister{c: a.collector, lbType: elbv2types.LoadBalancerTypeEnumApplication}
	if albs, err := alb.List(ctx, region, account); err == nil {
		for _, lb := range albs {
			ids = append(ids, lb.ARN)
		}
	} else {
		a.logger.Warn("Failed to list ALBs for discovery", zap.String("region", region), zap.Error(err))
	}

	// 3. NLB
	nlb := &elbv2Lister{c: a.collector, lbType: elbv2types.LoadBalancerTypeEnumNetwork}
	if nlbs, err := nlb.List(ctx, region, account); err == nil {
		for _, lb := range nlbs {
			ids = append(ids, lb.ARN)
		}
	} else {
		a.logger.Warn("Failed to list NLBs for discovery", zap.String("region", region), zap.Error(err))
	}

	// 4. GWLB
	gwlb := &elbv2Lister{c: a.collector, lbType: elbv2types.LoadBalancerTypeEnumGateway}
	if gwlbs, err := gwlb.List(ctx, region, account); err == nil {
		for _, lb := range gwlbs {
			ids = append(ids, lb.ARN)
		}
	} else {
		a.logger.Warn("Failed to list GWLBs for discovery", zap.String("region", region), zap.Error(err))
	}

	return ids
}
