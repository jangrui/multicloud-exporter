package common

import (
	"context"

	"multicloud-exporter/internal/config"
)

// FourDimensionAdapter 四维架构适配器接口
// 每个云厂商（Aliyun、Tencent、Huawei、AWS）需要实现此接口
// 将原有的采集逻辑适配到四维架构（Account → Product → Region → Resource）
type FourDimensionAdapter interface {
	// CollectAccountMetrics 采集账号级指标
	// 返回该账号下所有产品、区域、资源的指标总和
	CollectAccountMetrics(ctx context.Context, account config.CloudAccount) error

	// CollectProductMetrics 采集产品级指标
	// 返回该账号下指定产品的所有区域、资源的指标总和
	CollectProductMetrics(ctx context.Context, account config.CloudAccount, productID string) error

	// CollectRegionMetrics 采集区域级指标
	// 返回该账号下指定产品、区域的所有资源的指标总和
	CollectRegionMetrics(ctx context.Context, account config.CloudAccount, productID, region string) error

	// CollectResourceMetrics 采集资源级指标
	// 返回指定资源的详细指标
	CollectResourceMetrics(ctx context.Context, account config.CloudAccount, productID, region, resourceID string) error

	// DiscoverResources 发现该账号下所有资源
	// 返回 productID → region → resourceIDs 的映射
	DiscoverResources(ctx context.Context, account config.CloudAccount) (map[string]map[string][]string, error)
}
