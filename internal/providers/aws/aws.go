package aws

import (
	"context"
	"strings"

	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/discovery"
	"multicloud-exporter/internal/logger"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
)

// Collector AWS 采集器：按账号/区域采集 CloudWatch 指标
type Collector struct {
	cfg  *config.Config
	disc *discovery.Manager

	clientFactory ClientFactory
}

func NewCollector(cfg *config.Config, mgr *discovery.Manager) *Collector {
	return &Collector{
		cfg:           cfg,
		disc:          mgr,
		clientFactory: &defaultClientFactory{},
	}
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
			logger.Log.Warnf("AWS 资源类型 %s 尚未实现", resource)
		}
	}
}

// getAllRegions 通过 DescribeRegions 自动发现全部区域
func (c *Collector) getAllRegions(account config.CloudAccount) []string {
	// 使用 us-east-1 作为默认接入点查询所有区域
	client, err := c.clientFactory.NewEC2Client(context.Background(), "us-east-1", account.AccessKeyID, account.AccessKeySecret)
	if err != nil {
		logger.Log.Errorf("AWS 获取区域列表错误，账号ID=%s 错误=%v", account.AccountID, err)
		return []string{"us-east-1"}
	}

	resp, err := client.DescribeRegions(context.Background(), &ec2.DescribeRegionsInput{})
	if err != nil {
		logger.Log.Errorf("AWS DescribeRegions 错误，账号ID=%s 错误=%v", account.AccountID, err)
		return []string{"us-east-1"}
	}

	var regions []string
	for _, r := range resp.Regions {
		if r.RegionName != nil {
			regions = append(regions, *r.RegionName)
		}
	}
	logger.Log.Debugf("AWS DescribeRegions 成功，数量=%d 账号ID=%s", len(regions), account.AccountID)
	return regions
}
