package aws

import (
	"context"
	"sync"
	"time"

	"multicloud-exporter/internal/cluster"
	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/discovery"
	"multicloud-exporter/internal/logger"
	providerscommon "multicloud-exporter/internal/providers/common"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
)

const (
	AWSProductS3   = "s3"
	AWSProductLB   = "lb"
	AWSProductALB  = "alb"
	AWSProductCLB  = "clb"
	AWSProductNLB  = "nlb"
	AWSProductGWLB = "gwlb"
)

var (
	productToNamespaceMapAWS = map[string]string{
		AWSProductS3:   "AWS/S3",
		AWSProductLB:   "AWS/ELB",
		AWSProductALB:  "AWS/ApplicationELB",
		AWSProductCLB:  "AWS/ELB",
		AWSProductNLB:  "AWS/NetworkELB",
		AWSProductGWLB: "AWS/GatewayELB",
	}
)

// Collector AWS 采集器：按账号/区域采集 CloudWatch 指标
type Collector struct {
	cfg                   *config.Config
	disc                  *discovery.Manager
	clientFactory         ClientFactory
	productRegionManagers map[string]providerscommon.RegionManager
	rmMu                  sync.RWMutex
	degradeMgr            *providerscommon.Manager
}

func NewCollector(cfg *config.Config, mgr *discovery.Manager, clusterMgr *cluster.SyncManager) *Collector {
	c := &Collector{
		cfg:                   cfg,
		disc:                  mgr,
		clientFactory:         &defaultClientFactory{},
		productRegionManagers: make(map[string]providerscommon.RegionManager),
	}

	// 为每个产品创建独立的 RegionManager
	if cfg != nil && cfg.GetServer() != nil && cfg.GetServer().RegionDiscovery != nil {
		for product := range productToNamespaceMapAWS {
			rm := providerscommon.NewRegionManager(providerscommon.RegionDiscoveryConfig{
				Enabled:           cfg.GetServer().RegionDiscovery.Enabled,
				DiscoveryInterval: parseDuration(cfg.GetServer().RegionDiscovery.DiscoveryInterval),
				EmptyThreshold:    cfg.GetServer().RegionDiscovery.EmptyThreshold,
			})

			if clusterMgr != nil {
				rm.SetBroadcaster(clusterMgr, "aws", product)
				clusterMgr.RegisterProductRegionManager("aws", product, rm)
			}

			rm.StartRediscoveryScheduler()
			c.productRegionManagers[product] = rm
		}
	}

	return c
}

// getProductRegionManager 获取指定产品的 RegionManager
func (c *Collector) getProductRegionManager(product string) providerscommon.RegionManager {
	c.rmMu.RLock()
	defer c.rmMu.RUnlock()
	return c.productRegionManagers[product]
}

// parseDuration 解析时长字符串为 time.Duration
func parseDuration(s string) time.Duration {
	if s == "" {
		return 0
	}
	if d, err := time.ParseDuration(s); err == nil {
		return d
	}
	return 0
}

// getAllRegions 通过 DescribeRegions 自动发现全部区域
func (c *Collector) getAllRegions(account config.CloudAccount) []string {
	// 使用 us-east-1 作为默认接入点查询所有区域
	client, err := c.clientFactory.NewEC2Client(context.Background(), "us-east-1", account.AccessKeyID, account.AccessKeySecret)
	if err != nil {
		ctxLog := logger.NewContextLogger("AWS", "account_id", account.AccountID, "region", "us-east-1", "resource_type", "EC2")
		ctxLog.Errorf("获取区域列表错误: %v", err)
		return []string{"us-east-1"}
	}

	resp, err := client.DescribeRegions(context.Background(), &ec2.DescribeRegionsInput{})
	if err != nil {
		ctxLog := logger.NewContextLogger("AWS", "account_id", account.AccountID, "region", "us-east-1", "resource_type", "EC2")
		ctxLog.Errorf("DescribeRegions API调用错误: %v", err)
		return []string{"us-east-1"}
	}

	var regions []string
	for _, r := range resp.Regions {
		if r.RegionName != nil {
			regions = append(regions, *r.RegionName)
		}
	}
	ctxLog := logger.NewContextLogger("AWS", "account_id", account.AccountID, "region", "us-east-1", "resource_type", "EC2")
	ctxLog.Debugf("DescribeRegions API调用成功，数量=%d", len(regions))

	// 如果未启用区域管理器，返回所有区域
	return regions
}
