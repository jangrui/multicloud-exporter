package aliyun

import (
	"context"

	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/logger"
	"multicloud-exporter/internal/providers"

	"go.uber.org/zap"
)

func init() {
	providers.RegisterFourDimensionAdapter("aliyun", func(collector providers.Provider, logger *zap.Logger) providers.FourDimensionAdapter {
		c, ok := collector.(*Collector)
		if !ok {
			return nil
		}
		return NewFourDimensionAdapter(c, logger)
	})
}

// FourDimensionAdapter 阿里云四维架构适配器
type FourDimensionAdapter struct {
	collector *Collector
	logger    *zap.Logger
}

// NewFourDimensionAdapter 创建阿里云四维架构适配器
func NewFourDimensionAdapter(collector *Collector, logger *zap.Logger) *FourDimensionAdapter {
	return &FourDimensionAdapter{
		collector: collector,
		logger:    logger,
	}
}

// CollectAccountMetrics 采集账号级指标
func (a *FourDimensionAdapter) CollectAccountMetrics(ctx context.Context, account config.CloudAccount) error {
	ctxLog := logger.NewContextLogger("AliyunAdapter", "account_id", account.AccountID)
	ctxLog.Info("开始采集账号级指标")

	regions := account.Regions
	if len(regions) == 0 || (len(regions) == 1 && regions[0] == "*") {
		regions = a.collector.getAllRegions(account)
	}

	if len(regions) == 0 {
		ctxLog.Warn("无可用区域")
		return nil
	}

	for _, region := range regions {
		regionLog := ctxLog.With("region", region)
		regionLog.Debug("采集区域指标")

		a.collector.collectCMSMetrics(account, region, "")
	}

	ctxLog.Infof("完成账号级指标采集 regions=%d", len(regions))
	return nil
}

// CollectProductMetrics 采集产品级指标
func (a *FourDimensionAdapter) CollectProductMetrics(ctx context.Context, account config.CloudAccount, productID string) error {
	ctxLog := logger.NewContextLogger("AliyunAdapter", "account_id", account.AccountID, "product_id", productID)
	ctxLog.Info("开始采集产品级指标")

	regions := account.Regions
	if len(regions) == 0 || (len(regions) == 1 && regions[0] == "*") {
		regions = a.collector.getAllRegions(account)
	}

	if len(regions) == 0 {
		ctxLog.Warn("无可用区域")
		return nil
	}

	for _, region := range regions {
		regionLog := ctxLog.With("region", region)
		regionLog.Debug("采集产品指标")

		namespace := a.mapProductIDToNamespace(productID)
		if namespace == "" {
			regionLog.Warnf("未知的阿里云产品ID，跳过采集")
			continue
		}
		a.collector.collectCMSMetrics(account, region, namespace)
	}

	ctxLog.Infof("完成产品级指标采集 regions=%d", len(regions))
	return nil
}

// CollectRegionMetrics 采集区域级指标
func (a *FourDimensionAdapter) CollectRegionMetrics(ctx context.Context, account config.CloudAccount, productID, region string) error {
	ctxLog := logger.NewContextLogger("AliyunAdapter", "account_id", account.AccountID, "product_id", productID, "region", region)
	ctxLog.Info("开始采集区域级指标")

	namespace := a.mapProductIDToNamespace(productID)
	if namespace == "" {
		ctxLog.Warnf("未知的阿里云产品ID，跳过采集")
		return nil
	}
	a.collector.collectCMSMetrics(account, region, namespace)

	ctxLog.Info("完成区域级指标采集")
	return nil
}

// CollectResourceMetrics 采集资源级指标
func (a *FourDimensionAdapter) CollectResourceMetrics(ctx context.Context, account config.CloudAccount, productID, region, resourceID string) error {
	ctxLog := logger.NewContextLogger("AliyunAdapter", "account_id", account.AccountID, "product_id", productID, "region", region, "resource_id", resourceID)
	ctxLog.Info("开始采集资源级指标")

	namespace := a.mapProductIDToNamespace(productID)
	if namespace == "" {
		ctxLog.Warnf("未知的阿里云产品ID，跳过采集")
		return nil
	}
	a.collector.collectCMSMetrics(account, region, namespace)

	ctxLog.Info("完成资源级指标采集")
	return nil
}

func (a *FourDimensionAdapter) mapProductIDToNamespace(productID string) string {
	switch productID {
	case AliyunProductSLB, "clb":
		return "acs_slb_dashboard"
	case AliyunProductOSS, "s3":
		return "acs_oss_dashboard"
	case AliyunProductCBWP, "bwp":
		return "acs_bandwidth_package"
	case AliyunProductALB:
		return "acs_alb"
	case AliyunProductNLB:
		return "acs_nlb"
	case AliyunProductAliGWLB, "gwlb":
		return "acs_gwlb"
	default:
		return ""
	}
}

// DiscoverResources 发现该账号下所有资源
func (a *FourDimensionAdapter) DiscoverResources(ctx context.Context, account config.CloudAccount) (map[string]map[string][]string, error) {
	ctxLog := logger.NewContextLogger("AliyunAdapter", "account_id", account.AccountID)
	ctxLog.Info("开始发现资源")

	result := make(map[string]map[string][]string)

	regions := account.Regions
	if len(regions) == 0 || (len(regions) == 1 && regions[0] == "*") {
		regions = a.collector.getAllRegions(account)
	}

	if len(regions) == 0 {
		return result, nil
	}

	for _, region := range regions {
		regionResult := make(map[string][]string)

		if err := a.discoverResourcesInRegion(account, region, regionResult); err != nil {
			ctxLog.Warnf("区域资源发现失败 region=%s error=%v", region, err)
			continue
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

// discoverResourcesInRegion 发现指定区域的所有资源
func (a *FourDimensionAdapter) discoverResourcesInRegion(account config.CloudAccount, region string, result map[string][]string) error {
	products := []string{
		AliyunProductSLB,
		AliyunProductCBWP,
		AliyunProductOSS,
		AliyunProductALB,
		AliyunProductNLB,
	}

	for _, productID := range products {
		ids, err := a.discoverProductResources(account, region, productID)
		if err != nil {
			logger.NewContextLogger("AliyunAdapter", "account_id", account.AccountID, "product_id", productID, "region", region).
				Warnf("产品资源发现失败 error=%v", err)
			continue
		}
		result[productID] = ids
	}

	return nil
}

// discoverProductResources 发现指定产品的所有资源
func (a *FourDimensionAdapter) discoverProductResources(account config.CloudAccount, region, productID string) ([]string, error) {
	switch productID {
	case AliyunProductSLB:
		ids, _ := a.collector.listSLBIDs(account, region)
		return ids, nil
	case AliyunProductCBWP:
		ids := a.collector.listCBWPIDs(account, region)
		return ids, nil
	case AliyunProductOSS:
		ids := a.collector.listOSSIDs(account, region)
		return ids, nil
	case AliyunProductALB:
		ids := a.collector.listALBIDs(account, region)
		return ids, nil
	case AliyunProductNLB:
		ids := a.collector.listNLBIDs(account, region)
		return ids, nil
	default:
		return nil, nil
	}
}
