// 采集调度器：按账号并发调度各云采集器，统一处理资源类型集合
package collector

import (
	"log"
	"sync"

	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/providers/aliyun"
	"multicloud-exporter/internal/providers/huawei"
	"multicloud-exporter/internal/providers/tencent"
)

// Collector 持有配置与各云采集器实例
type Collector struct {
	cfg     *config.Config
	aliyun  *aliyun.Collector
	huawei  *huawei.Collector
	tencent *tencent.Collector
}

// NewCollector 创建调度器并初始化各云采集器
func NewCollector(cfg *config.Config) *Collector {
	return &Collector{
		cfg:     cfg,
		aliyun:  aliyun.NewCollector(cfg),
		huawei:  huawei.NewCollector(),
		tencent: tencent.NewCollector(),
	}
}

// Collect 为每个账号并发执行采集任务
func (c *Collector) Collect() {
    log.Printf("开始采集，加载账号数量=%d", len(c.cfg.AccountsList)+len(c.cfg.AccountsByProvider)+len(c.cfg.AccountsByProviderLegacy))
	var accounts []config.CloudAccount
	accounts = append(accounts, c.cfg.AccountsList...)
	if c.cfg.AccountsByProvider != nil {
		for provider, list := range c.cfg.AccountsByProvider {
			for _, acc := range list {
				if acc.Provider == "" {
					acc.Provider = provider
				}
				accounts = append(accounts, acc)
			}
		}
	}
	if c.cfg.AccountsByProviderLegacy != nil {
		for provider, list := range c.cfg.AccountsByProviderLegacy {
			for _, acc := range list {
				if acc.Provider == "" {
					acc.Provider = provider
				}
				accounts = append(accounts, acc)
			}
		}
	}

	var wg sync.WaitGroup
    for _, account := range accounts {
        wg.Add(1)
        go func(acc config.CloudAccount) {
            defer wg.Done()
            log.Printf("开始账号采集 provider=%s account_id=%s", acc.Provider, acc.AccountID)
            c.collectAccount(acc)
            log.Printf("完成账号采集 provider=%s account_id=%s", acc.Provider, acc.AccountID)
        }(account)
    }
    wg.Wait()
}

// collectAccount 规范化资源类型并路由到对应云采集器
func (c *Collector) collectAccount(account config.CloudAccount) {
	if len(account.Resources) == 0 || (len(account.Resources) == 1 && account.Resources[0] == "*") {
		account.Resources = GetAllResources(account.Provider)
	}

	switch account.Provider {
	case "aliyun":
		c.aliyun.Collect(account)
	case "huawei":
		c.huawei.Collect(account)
	case "tencent":
		c.tencent.Collect(account)
	default:
		log.Printf("Unknown provider: %s", account.Provider)
	}
}
