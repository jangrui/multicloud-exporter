package tencent

import (
	"context"

	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/logger"
	"multicloud-exporter/internal/providers"

	"go.uber.org/zap"
)

func init() {
	providers.RegisterFourDimensionAdapter("tencent", func(collector providers.Provider, logger *zap.Logger) providers.FourDimensionAdapter {
		c, ok := collector.(*Collector)
		if !ok {
			return nil
		}
		return NewFourDimensionAdapter(c, logger)
	})
}

// FourDimensionAdapter 腾讯云四维架构适配器
type FourDimensionAdapter struct {
	collector *Collector
	logger    *zap.Logger
}

// NewFourDimensionAdapter 创建腾讯云四维架构适配器
func NewFourDimensionAdapter(collector *Collector, logger *zap.Logger) *FourDimensionAdapter {
	return &FourDimensionAdapter{
		collector: collector,
		logger:    logger,
	}
}

// CollectAccountMetrics 采集账号级指标
func (t *FourDimensionAdapter) CollectAccountMetrics(ctx context.Context, account config.CloudAccount) error {
	ctxLog := logger.NewContextLogger("TencentAdapter", "account_id", account.AccountID)
	ctxLog.Info("开始采集账号级指标")

	regions := account.Regions
	if len(regions) == 0 || (len(regions) == 1 && regions[0] == "*") {
		regions = t.collector.getAllRegions(account)
		if len(regions) == 0 {
			regions = []string{"ap-guangzhou"}
		}
	}

	if len(regions) == 0 {
		ctxLog.Warn("无可用区域")
		return nil
	}

	for _, region := range regions {
		regionLog := ctxLog.With("region", region)
		regionLog.Debug("采集区域指标")

		t.collector.collectRegion(account, region)
	}

	ctxLog.Infof("完成账号级指标采集 regions=%d", len(regions))
	return nil
}

// CollectProductMetrics 采集产品级指标
func (t *FourDimensionAdapter) CollectProductMetrics(ctx context.Context, account config.CloudAccount, productID string) error {
	ctxLog := logger.NewContextLogger("TencentAdapter", "account_id", account.AccountID, "product_id", productID)
	ctxLog.Info("开始采集产品级指标")

	regions := account.Regions
	if len(regions) == 0 || (len(regions) == 1 && regions[0] == "*") {
		regions = t.collector.getAllRegions(account)
		if len(regions) == 0 {
			regions = []string{"ap-guangzhou"}
		}
	}

	if len(regions) == 0 {
		ctxLog.Warn("无可用区域")
		return nil
	}

	for _, region := range regions {
		regionLog := ctxLog.With("region", region)
		regionLog.Debug("采集产品指标")

		switch productID {
		case TencentProductCLB:
			t.collector.collectCLB(account, region)
		case TencentProductBWP:
			t.collector.collectBWP(account, region)
		case TencentProductCOS:
			t.collector.collectCOS(account, region)
		case TencentProductGWLB:
			t.collector.collectGWLB(account, region)
		}
	}

	ctxLog.Infof("完成产品级指标采集 regions=%d", len(regions))
	return nil
}

// CollectRegionMetrics 采集区域级指标
func (t *FourDimensionAdapter) CollectRegionMetrics(ctx context.Context, account config.CloudAccount, productID, region string) error {
	ctxLog := logger.NewContextLogger("TencentAdapter", "account_id", account.AccountID, "product_id", productID, "region", region)
	ctxLog.Info("开始采集区域级指标")

	switch productID {
	case TencentProductCLB:
		t.collector.collectCLB(account, region)
	case TencentProductBWP:
		t.collector.collectBWP(account, region)
	case TencentProductCOS:
		t.collector.collectCOS(account, region)
	case TencentProductGWLB:
		t.collector.collectGWLB(account, region)
	}

	ctxLog.Info("完成区域级指标采集")
	return nil
}

// CollectResourceMetrics 采集资源级指标
func (t *FourDimensionAdapter) CollectResourceMetrics(ctx context.Context, account config.CloudAccount, productID, region, resourceID string) error {
	ctxLog := logger.NewContextLogger("TencentAdapter", "account_id", account.AccountID, "product_id", productID, "region", region, "resource_id", resourceID)
	ctxLog.Info("开始采集资源级指标")

	switch productID {
	case TencentProductCLB:
		t.collector.collectCLB(account, region)
	case TencentProductBWP:
		t.collector.collectBWP(account, region)
	case TencentProductCOS:
		t.collector.collectCOS(account, region)
	case TencentProductGWLB:
		t.collector.collectGWLB(account, region)
	}

	ctxLog.Info("完成资源级指标采集")
	return nil
}

// DiscoverResources 发现该账号下所有资源
func (t *FourDimensionAdapter) DiscoverResources(ctx context.Context, account config.CloudAccount) (map[string]map[string][]string, error) {
	ctxLog := logger.NewContextLogger("TencentAdapter", "account_id", account.AccountID)
	ctxLog.Info("开始发现资源")

	result := make(map[string]map[string][]string)

	regions := account.Regions
	if len(regions) == 0 || (len(regions) == 1 && regions[0] == "*") {
		regions = t.collector.getAllRegions(account)
		if len(regions) == 0 {
			regions = []string{"ap-guangzhou"}
		}
	}

	if len(regions) == 0 {
		return result, nil
	}

	for _, region := range regions {
		regionResult := make(map[string][]string)

		// CLB
		ids := t.collector.listCLBVips(account, region)
		regionResult["clb"] = ids

		// BWP
		ids = t.collector.listBWPIDs(account, region)
		regionResult["bwp"] = ids

		// COS
		ids = t.collector.listCOSBuckets(account, region)
		regionResult["cos"] = ids

		// GWLB
		ids = t.collector.listGWLBIDs(account, region)
		regionResult["gwlb"] = ids

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
func (t *FourDimensionAdapter) GetRegions(ctx context.Context, account config.CloudAccount) ([]string, error) {
	regions := account.Regions
	if len(regions) == 0 || (len(regions) == 1 && regions[0] == "*") {
		regions = t.collector.getAllRegions(account)
		if len(regions) == 0 {
			regions = []string{"ap-guangzhou"}
		}
	}
	if len(regions) == 0 {
		return []string{}, nil
	}
	return regions, nil
}
