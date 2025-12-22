// 采集调度器：按账号并发调度各云采集器，统一处理资源类型集合
package collector

import (
	"sync"

	"time"

	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/discovery"
	"multicloud-exporter/internal/logger"
	"multicloud-exporter/internal/metrics"
	"multicloud-exporter/internal/providers"
	_ "multicloud-exporter/internal/providers/aliyun"
	_ "multicloud-exporter/internal/providers/aws"
	_ "multicloud-exporter/internal/providers/huawei"
	_ "multicloud-exporter/internal/providers/tencent"
)

// Status 定义采集器状态
type Status struct {
	LastStart    time.Time              `json:"last_start"`
	LastEnd      time.Time              `json:"last_end"`
	Duration     string                 `json:"duration"`
	LastResults  map[string]AccountStat `json:"last_results"` // key: provider|account_id
	SampleCounts map[string]int         `json:"sample_counts"`
}

type AccountStat struct {
	Timestamp time.Time `json:"timestamp"`
	Status    string    `json:"status"` // "running", "completed"
}

// Collector 持有配置与各云采集器实例
type Collector struct {
	cfg        *config.Config
	disc       *discovery.Manager
	providers  map[string]providers.Provider
	status     Status
	statusLock sync.RWMutex
}

// NewCollector 创建调度器并初始化各云采集器
func NewCollector(cfg *config.Config, mgr *discovery.Manager) *Collector {
	c := &Collector{
		cfg:       cfg,
		disc:      mgr,
		providers: make(map[string]providers.Provider),
		status: Status{
			LastResults: make(map[string]AccountStat),
		},
	}

	for _, name := range providers.GetAllProviders() {
		if factory, ok := providers.GetFactory(name); ok {
			c.providers[name] = factory(cfg, mgr)
		}
	}
	return c
}

// GetStatus 返回当前采集状态
func (c *Collector) GetStatus() Status {
	c.statusLock.RLock()
	defer c.statusLock.RUnlock()
	// Copy map to avoid race condition
	res := make(map[string]AccountStat)
	for k, v := range c.status.LastResults {
		res[k] = v
	}
	return Status{
		LastStart:   c.status.LastStart,
		LastEnd:     c.status.LastEnd,
		Duration:    c.status.Duration,
		LastResults: res,
	}
}

// CollectFiltered 执行带过滤条件的采集
func (c *Collector) CollectFiltered(filterProvider, filterResource string) {
	c.collectInternal(filterProvider, filterResource)
}

// Collect 为每个账号并发执行采集任务
func (c *Collector) Collect() {
	c.collectInternal("", "")
}

func (c *Collector) collectInternal(filterProvider, filterResource string) {
	c.cfg.Mu.RLock()
	total := len(c.cfg.AccountsByProvider)
	for _, list := range c.cfg.AccountsByProvider {
		total += len(list)
	}
	logger.Log.Infof("开始采集，加载账号数量=%d", total)
	var accounts []config.CloudAccount
	if c.cfg.AccountsByProvider != nil {
		for provider, list := range c.cfg.AccountsByProvider {
			for _, acc := range list {
				acc.Provider = provider
				accounts = append(accounts, acc)
			}
		}
	}
	c.cfg.Mu.RUnlock()

	// Filter accounts if provider is specified
	if filterProvider != "" {
		var filtered []config.CloudAccount
		for _, acc := range accounts {
			if acc.Provider == filterProvider {
				filtered = append(filtered, acc)
			}
		}
		accounts = filtered
	}

	// 重置命名空间样本计数
	metrics.ResetSampleCounts()
	c.statusLock.Lock()
	c.status.LastStart = time.Now()
	c.statusLock.Unlock()
	start := time.Now()

	var wg sync.WaitGroup
	for _, account := range accounts {
		// Note: We removed account-level sharding here because providers (Aliyun, Tencent)
		// implement their own fine-grained sharding (e.g. by Region) inside Collect().
		// If we shard by account here, we might skip an account entirely that contains
		// regions belonging to this shard, leading to missing data (Double Sharding bug).
		//
		// Future optimization: If a provider does NOT support internal sharding,
		// we should handle it here or enforce them to implement it.

		c.statusLock.Lock()
		c.status.LastResults[account.Provider+"|"+account.AccountID] = AccountStat{
			Timestamp: time.Now(),
			Status:    "running",
		}
		c.statusLock.Unlock()

		wg.Add(1)
		go func(acc config.CloudAccount) {
			defer wg.Done()
			logger.Log.Debugf("开始账号采集 provider=%s account_id=%s", acc.Provider, acc.AccountID)
			c.collectAccount(acc, filterResource)
			logger.Log.Debugf("完成账号采集 provider=%s account_id=%s", acc.Provider, acc.AccountID)

			c.statusLock.Lock()
			c.status.LastResults[acc.Provider+"|"+acc.AccountID] = AccountStat{
				Timestamp: time.Now(),
				Status:    "completed",
			}
			c.statusLock.Unlock()
		}(account)
	}
	wg.Wait()

	c.statusLock.Lock()
	c.status.LastEnd = time.Now()
	c.status.Duration = time.Since(start).String()
	c.status.SampleCounts = metrics.GetSampleCounts()
	c.statusLock.Unlock()
}

// collectAccount 规范化资源类型并路由到对应云采集器
func (c *Collector) collectAccount(account config.CloudAccount, filterResource string) {
	p, ok := c.providers[account.Provider]
	if !ok {
		logger.Log.Warnf("未知的云平台: %s", account.Provider)
		return
	}

	if filterResource != "" {
		// Only collect specified resource
		account.Resources = []string{filterResource}
	} else if len(account.Resources) == 0 || (len(account.Resources) == 1 && account.Resources[0] == "*") {
		account.Resources = p.GetDefaultResources()
	}

	p.Collect(account)
}
