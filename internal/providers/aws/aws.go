package aws

import (
	"context"
	"strings"
	"time"

	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/discovery"
	"multicloud-exporter/internal/logger"
	providerscommon "multicloud-exporter/internal/providers/common"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
)

// Collector AWS 采集器：按账号/区域采集 CloudWatch 指标
type Collector struct {
	cfg           *config.Config
	disc          *discovery.Manager
	clientFactory ClientFactory
	regionManager providerscommon.RegionManager
}

func NewCollector(cfg *config.Config, mgr *discovery.Manager) *Collector {
	c := &Collector{
		cfg:           cfg,
		disc:          mgr,
		clientFactory: &defaultClientFactory{},
	}

	// 初始化区域管理器
	if cfg != nil && cfg.GetServer() != nil && cfg.GetServer().RegionDiscovery != nil {
		c.regionManager = providerscommon.NewRegionManager(providerscommon.RegionDiscoveryConfig{
			Enabled:           cfg.GetServer().RegionDiscovery.Enabled,
			DiscoveryInterval: parseDuration(cfg.GetServer().RegionDiscovery.DiscoveryInterval),
			EmptyThreshold:    cfg.GetServer().RegionDiscovery.EmptyThreshold,
			DataDir:           cfg.GetServer().RegionDiscovery.DataDir,
			PersistFile:       cfg.GetServer().RegionDiscovery.PersistFile,
		})

		// 加载持久化的区域状态
		if err := c.regionManager.Load(); err != nil {
			ctxLog := logger.NewContextLogger("AWS", "resource_type", "RegionManager")
			ctxLog.Warnf("加载区域状态失败: %v", err)
		}

		// 启动定期重新发现调度器
		c.regionManager.StartRediscoveryScheduler()
	}

	return c
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

func (c *Collector) Collect(account config.CloudAccount) {
	// 注意：分片逻辑已下沉到产品级（collectS3/collectALB 等），此处不做账号级分片
	// 这样可以避免双重分片导致的任务丢失问题
	for _, resource := range account.Resources {
		r := strings.ToLower(strings.TrimSpace(resource))
		switch r {
		case "*":
			c.collectS3(account)
			c.collectALB(account)
			c.collectCLB(account)
			c.collectNLB(account)
			c.collectGWLB(account)
		case "s3":
			c.collectS3(account)
		case "alb":
			c.collectALB(account)
		case "clb":
			c.collectCLB(account)
		case "nlb":
			c.collectNLB(account)
		case "gwlb":
			c.collectGWLB(account)
		default:
			ctxLog := logger.NewContextLogger("AWS", "account_id", account.AccountID, "resource_type", resource)
			ctxLog.Warnf("资源类型尚未实现")
		}
	}
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

	// 使用区域管理器进行智能过滤
	if c.regionManager != nil {
		activeRegions := c.regionManager.GetActiveRegions(account.AccountID, regions)
		ctxLog := logger.NewContextLogger("AWS", "account_id", account.AccountID, "resource_type", "RegionManager")
		ctxLog.Infof("智能区域选择: 总=%d 活跃=%d",
			len(regions), len(activeRegions))
		return activeRegions
	}

	// 如果未启用区域管理器，返回所有区域
	return regions
}
