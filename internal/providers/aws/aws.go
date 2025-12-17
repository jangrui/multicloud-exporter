package aws

import (
	"strings"

	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/discovery"
	"multicloud-exporter/internal/logger"
	"multicloud-exporter/internal/utils"
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
	// AWS 的 S3 采集不依赖 region 列表（bucket 分布在各 region），这里仅做分片入口：
	wTotal, wIndex := utils.ClusterConfig()
	key := account.AccountID + "|aws"
	if !utils.ShouldProcess(key, wTotal, wIndex) {
		return
	}
	for _, resource := range account.Resources {
		r := strings.ToLower(strings.TrimSpace(resource))
		switch r {
		case "*":
			c.collectS3(account)
		case "s3":
			c.collectS3(account)
		default:
			logger.Log.Warnf("AWS 资源类型 %s 尚未实现", resource)
		}
	}
}
