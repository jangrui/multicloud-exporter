// 采集调度器：按账号并发调度各云采集器，统一处理资源类型集合
package collector

import (
	"bufio"
	"hash/fnv"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"

	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/discovery"
	"multicloud-exporter/internal/logger"
	"multicloud-exporter/internal/providers"
	_ "multicloud-exporter/internal/providers/aliyun"
	_ "multicloud-exporter/internal/providers/huawei"
	_ "multicloud-exporter/internal/providers/tencent"
)

// Collector 持有配置与各云采集器实例
type Collector struct {
	cfg       *config.Config
	disc      *discovery.Manager
	providers map[string]providers.Provider
}

// NewCollector 创建调度器并初始化各云采集器
func NewCollector(cfg *config.Config, mgr *discovery.Manager) *Collector {
	c := &Collector{
		cfg:       cfg,
		disc:      mgr,
		providers: make(map[string]providers.Provider),
	}

	for _, name := range providers.GetAllProviders() {
		if factory, ok := providers.GetFactory(name); ok {
			c.providers[name] = factory(cfg, mgr)
		}
	}
	return c
}

// Collect 为每个账号并发执行采集任务
func (c *Collector) Collect() {
	c.cfg.Mu.RLock()
	total := len(c.cfg.AccountsList) + len(c.cfg.AccountsByProvider) + len(c.cfg.AccountsByProviderLegacy)
	logger.Log.Infof("开始采集，加载账号数量=%d", total)
	var accounts []config.CloudAccount
	accounts = append(accounts, c.cfg.AccountsList...)
	if c.cfg.AccountsByProvider != nil {
		for provider, list := range c.cfg.AccountsByProvider {
			for _, acc := range list {
				acc.Provider = provider
				accounts = append(accounts, acc)
			}
		}
	}
	if c.cfg.AccountsByProviderLegacy != nil {
		for provider, list := range c.cfg.AccountsByProviderLegacy {
			for _, acc := range list {
				acc.Provider = provider
				accounts = append(accounts, acc)
			}
		}
	}
	c.cfg.Mu.RUnlock()

	wTotal, wIndex := clusterConf()
	var wg sync.WaitGroup
	for _, account := range accounts {
		if !assignAccount(account.Provider, account.AccountID, wTotal, wIndex) {
			continue
		}
		wg.Add(1)
		go func(acc config.CloudAccount) {
			defer wg.Done()
			logger.Log.Debugf("开始账号采集 provider=%s account_id=%s", acc.Provider, acc.AccountID)
			c.collectAccount(acc)
			logger.Log.Debugf("完成账号采集 provider=%s account_id=%s", acc.Provider, acc.AccountID)
		}(account)
	}
	wg.Wait()
}

// collectAccount 规范化资源类型并路由到对应云采集器
func (c *Collector) collectAccount(account config.CloudAccount) {
	p, ok := c.providers[account.Provider]
	if !ok {
		logger.Log.Warnf("Unknown provider: %s", account.Provider)
		return
	}

	if len(account.Resources) == 0 || (len(account.Resources) == 1 && account.Resources[0] == "*") {
		account.Resources = p.GetDefaultResources()
	}

	p.Collect(account)
}

func clusterConf() (int, int) {
	// 优先：Headless Service 动态成员发现（Deployment 多副本）
	if os.Getenv("CLUSTER_DISCOVERY") == "headless" {
		svc := os.Getenv("CLUSTER_SVC")
		selfIP := os.Getenv("POD_IP")
		if svc != "" && selfIP != "" {
			if ips, err := net.LookupIP(svc); err == nil && len(ips) > 0 {
				var list []string
				for _, ip := range ips {
					list = append(list, ip.String())
				}
				sort.Strings(list)
				for i, ip := range list {
					if ip == selfIP {
						return len(list), i
					}
				}
			}
		}
	}

	// 次选：文件成员发现（宿主机共享文件，无中间件）
	if os.Getenv("CLUSTER_DISCOVERY") == "file" {
		path := os.Getenv("CLUSTER_FILE")
		self := os.Getenv("POD_NAME")
		if self == "" {
			self = os.Getenv("HOSTNAME")
		}
		if path != "" && self != "" {
			if f, err := os.Open(path); err == nil {
				defer func() { _ = f.Close() }()
				var members []string
				sc := bufio.NewScanner(f)
				for sc.Scan() {
					line := strings.TrimSpace(sc.Text())
					if line != "" {
						members = append(members, line)
					}
				}
				if len(members) > 0 {
					sort.Strings(members)
					for i, m := range members {
						if m == self {
							return len(members), i
						}
					}
				}
			}
		}
	}

	// 回退：静态分片参数
	total := 1
	index := 0
	if v := os.Getenv("CLUSTER_WORKERS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			total = n
		}
	}
	if v := os.Getenv("CLUSTER_INDEX"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			index = n
		}
	}
	if index >= total {
		index = index % total
	}
	return total, index
}

func shardOf(s string, n int) int {
	if n <= 1 {
		return 0
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	return int(h.Sum32() % uint32(n))
}

func assignAccount(provider, accountID string, total, index int) bool {
	if total <= 1 {
		return true
	}
	key := provider + "|" + accountID
	return shardOf(key, total) == index
}
