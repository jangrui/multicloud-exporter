package discovery

import (
	"context"
	"os"
	"sync"
	"time"

	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/logger"

	"gopkg.in/yaml.v3"
)

type Manager struct {
	cfg           *config.Config
	mu            sync.RWMutex
	products      map[string][]config.Product
	version       int64
	updatedAt     time.Time
	subsMu        sync.Mutex
	subs          map[chan struct{}]struct{}
	lastAccSig    string
	lastAccPath   string
	watchInterval time.Duration
	// 发现统计信息
	lastRefreshDuration time.Duration
	providerDurations   map[string]time.Duration
	refreshCount        int64
}

func NewManager(cfg *config.Config) *Manager {
	return &Manager{
		cfg:               cfg,
		products:          make(map[string][]config.Product),
		subs:              make(map[chan struct{}]struct{}),
		watchInterval:     3 * time.Second,
		providerDurations: make(map[string]time.Duration),
	}
}

func (m *Manager) Get() map[string][]config.Product {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[string][]config.Product)
	for k, v := range m.products {
		cp := make([]config.Product, len(v))
		copy(cp, v)
		out[k] = cp
	}
	return out
}

func (m *Manager) Version() int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.version
}

func (m *Manager) UpdatedAt() time.Time {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.updatedAt
}

type ProductDetail struct {
	Namespace    string `json:"namespace"`
	MetricsCount int    `json:"metrics_count"`
	AutoDiscover bool   `json:"auto_discover"`
}

type ProviderStats struct {
	ProductsCount     int             `json:"products_count"`
	MetricsCount      int             `json:"metrics_count"`
	AccountsCount     int             `json:"accounts_count"`
	DiscoveryDuration string          `json:"discovery_duration"`
	Products          []ProductDetail `json:"products"`
}

type DiscoveryStatus struct {
	Version             int64                    `json:"version"`
	UpdatedAt           int64                    `json:"updated_at"`
	AccountsPath        string                   `json:"accounts_path"`
	AccountsSignature   string                   `json:"accounts_signature"`
	Subscribers         int                      `json:"subscribers"`
	Providers           []string                 `json:"providers"`
	ProductsCount       map[string]int           `json:"products_count"`
	LastRefreshDuration string                   `json:"last_refresh_duration"`
	RefreshCount        int64                    `json:"refresh_count"`
	ProviderStats       map[string]ProviderStats `json:"provider_stats"`
}

func (m *Manager) Status() DiscoveryStatus {
	// 先获取账号统计（使用 cfg.Mu）
	m.cfg.Mu.RLock()
	accountsByProvider := make(map[string]int)
	if m.cfg.AccountsByProvider != nil {
		for provider, accounts := range m.cfg.AccountsByProvider {
			accountsByProvider[provider] = len(accounts)
		}
	}
	m.cfg.Mu.RUnlock()

	// 获取 Manager 的统计信息（使用 mu）
	m.mu.RLock()
	providers := make([]string, 0, len(m.products))
	counts := make(map[string]int, len(m.products))
	providerStats := make(map[string]ProviderStats)

	// 计算每个云平台的统计信息
	for provider, products := range m.products {
		providers = append(providers, provider)
		counts[provider] = len(products)

		totalMetrics := 0
		productDetails := make([]ProductDetail, 0, len(products))

		for _, product := range products {
			metricsCount := 0
			for _, group := range product.MetricInfo {
				metricsCount += len(group.MetricList)
			}
			totalMetrics += metricsCount
			productDetails = append(productDetails, ProductDetail{
				Namespace:    product.Namespace,
				MetricsCount: metricsCount,
				AutoDiscover: product.AutoDiscover,
			})
		}

		discoveryDuration := ""
		if dur, ok := m.providerDurations[provider]; ok {
			discoveryDuration = dur.String()
		}

		providerStats[provider] = ProviderStats{
			ProductsCount:     len(products),
			MetricsCount:      totalMetrics,
			AccountsCount:     accountsByProvider[provider],
			DiscoveryDuration: discoveryDuration,
			Products:          productDetails,
		}
	}

	ver := m.version
	up := m.updatedAt.Unix()
	accPath := m.lastAccPath
	accSig := m.lastAccSig
	lastRefreshDuration := m.lastRefreshDuration.String()
	refreshCount := m.refreshCount
	m.mu.RUnlock()

	// 获取订阅者数量（使用 subsMu）
	m.subsMu.Lock()
	subs := len(m.subs)
	m.subsMu.Unlock()

	return DiscoveryStatus{
		Version:             ver,
		UpdatedAt:           up,
		AccountsPath:        accPath,
		AccountsSignature:   accSig,
		Subscribers:         subs,
		Providers:           providers,
		ProductsCount:       counts,
		LastRefreshDuration: lastRefreshDuration,
		RefreshCount:        refreshCount,
		ProviderStats:       providerStats,
	}
}

func (m *Manager) Refresh(ctx context.Context) error {
	start := time.Now()
	prods := make(map[string][]config.Product)
	providerDurations := make(map[string]time.Duration)
	m.cfg.Mu.RLock()
	for _, name := range GetAllDiscoverers() {
		if d, ok := GetDiscoverer(name); ok {
			providerStart := time.Now()
			if ps := d.Discover(ctx, m.cfg); len(ps) > 0 {
				prods[name] = ps
			}
			providerDurations[name] = time.Since(providerStart)
		}
	}
	m.cfg.Mu.RUnlock()
	m.mu.Lock()
	changed := !equalProducts(m.products, prods)
	m.products = prods
	duration := time.Since(start)
	m.lastRefreshDuration = duration
	m.providerDurations = providerDurations
	m.refreshCount++
	if changed {
		m.version++
		m.updatedAt = time.Now()
		ctxLog := logger.NewContextLogger("Discovery", "resource_type", "Manager")
		ctxLog.Infof("发现服务已刷新，版本=%d，总耗时: %v", m.version, duration)
	} else {
		ctxLog := logger.NewContextLogger("Discovery", "resource_type", "Manager")
		ctxLog.Infof("发现服务检查完成，无变化，总耗时: %v", duration)
	}
	m.mu.Unlock()
	if changed {
		m.broadcast()
	}
	return nil
}

func equalProducts(a, b map[string][]config.Product) bool {
	// 快速路径：长度不同
	if len(a) != len(b) {
		return false
	}

	// 比较每个 provider 的产品列表
	for provider, productsA := range a {
		productsB, ok := b[provider]
		if !ok {
			return false
		}

		// 快速路径：产品数量不同
		if len(productsA) != len(productsB) {
			return false
		}

		// 比较每个产品
		for i := range productsA {
			pa := &productsA[i]
			pb := &productsB[i]

			// 比较 Namespace 和 AutoDiscover
			if pa.Namespace != pb.Namespace || pa.AutoDiscover != pb.AutoDiscover {
				return false
			}

			// 比较 Period
			if (pa.Period == nil) != (pb.Period == nil) {
				return false
			}
			if pa.Period != nil && pb.Period != nil && *pa.Period != *pb.Period {
				return false
			}

			// 比较 MetricInfo
			if len(pa.MetricInfo) != len(pb.MetricInfo) {
				return false
			}

			for j := range pa.MetricInfo {
				ma := &pa.MetricInfo[j]
				mb := &pb.MetricInfo[j]

				// 比较 MetricList
				if len(ma.MetricList) != len(mb.MetricList) {
					return false
				}
				for k := range ma.MetricList {
					if ma.MetricList[k] != mb.MetricList[k] {
						return false
					}
				}

				// 比较 Statistics
				if len(ma.Statistics) != len(mb.Statistics) {
					return false
				}
				for k := range ma.Statistics {
					if ma.Statistics[k] != mb.Statistics[k] {
						return false
					}
				}

				// 比较 Period
				if (ma.Period == nil) != (mb.Period == nil) {
					return false
				}
				if ma.Period != nil && mb.Period != nil && *ma.Period != *mb.Period {
					return false
				}
			}
		}
	}

	return true
}

func (m *Manager) Start(ctx context.Context) {
	p := os.Getenv("ACCOUNTS_PATH")
	m.lastAccPath = p
	m.lastAccSig = m.accountsSignature()
	_ = m.Refresh(ctx)
	go m.watchAccounts(ctx, p)
}

// periodic refresh removed: discovery uses accounts resources changes as trigger

func (m *Manager) watchAccounts(ctx context.Context, path string) {
	if path == "" {
		return
	}
	var lastMod int64
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(m.watchInterval):
			fi, err := os.Stat(path)
			if err != nil {
				continue
			}
			mt := fi.ModTime().UnixNano()
			if mt != lastMod {
				lastMod = mt
				sig := m.reloadAccounts(path)
				if sig != "" && sig != m.lastAccSig {
					m.lastAccSig = sig
					_ = m.Refresh(ctx)
				}
			}
		}
	}
}

func (m *Manager) reloadAccounts(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	// 同样需要进行环境变量替换，否则热加载后的配置包含占位符
	expanded := os.ExpandEnv(string(data))

	var accCfg struct {
		AccountsByProvider map[string][]config.CloudAccount `yaml:"accounts"`
	}
	if err := yaml.Unmarshal([]byte(expanded), &accCfg); err != nil {
		return ""
	}
	if m.cfg != nil {
		// 使用 Copy-On-Write 模式：先创建深拷贝，再原子替换
		newAccounts := make(map[string][]config.CloudAccount, len(accCfg.AccountsByProvider))
		for provider, accounts := range accCfg.AccountsByProvider {
			// 深拷贝账号列表
			copiedAccounts := make([]config.CloudAccount, len(accounts))
			copy(copiedAccounts, accounts)
			newAccounts[provider] = copiedAccounts
		}

		// 原子替换配置
		m.cfg.Mu.Lock()
		m.cfg.AccountsByProvider = newAccounts
		sig := m.accountsSignatureLocked()
		m.cfg.Mu.Unlock()
		return sig
	}
	return ""
}

func (m *Manager) accountsSignature() string {
	if m.cfg == nil {
		return ""
	}
	m.cfg.Mu.RLock()
	defer m.cfg.Mu.RUnlock()
	return m.accountsSignatureLocked()
}

func (m *Manager) accountsSignatureLocked() string {
	var accounts []config.CloudAccount
	if m.cfg == nil {
		return ""
	}
	if m.cfg.AccountsByProvider != nil {
		for p, xs := range m.cfg.AccountsByProvider {
			for _, a := range xs {
				a.Provider = p
				accounts = append(accounts, a)
			}
		}
	}
	type key struct {
		provider  string
		resources string
	}
	mset := make(map[key]struct{})
	for _, a := range accounts {
		if len(a.Resources) == 0 {
			continue
		}
		rs := append([]string{}, a.Resources...)
		for i := 0; i < len(rs); i++ {
			for j := i + 1; j < len(rs); j++ {
				if rs[j] < rs[i] {
					rs[i], rs[j] = rs[j], rs[i]
				}
			}
		}
		res := ""
		for i, r := range rs {
			if i > 0 {
				res += ","
			}
			res += r
		}
		mset[key{provider: a.Provider, resources: res}] = struct{}{}
	}
	keys := make([]key, 0, len(mset))
	for k := range mset {
		keys = append(keys, k)
	}
	// sort keys to ensure deterministic signature
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[i].provider > keys[j].provider || (keys[i].provider == keys[j].provider && keys[i].resources > keys[j].resources) {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	sig := ""
	for i, k := range keys {
		if i > 0 {
			sig += "|"
		}
		sig += k.provider + "#" + k.resources
	}
	return sig
}

func (m *Manager) Subscribe() chan struct{} {
	ch := make(chan struct{}, 1)
	m.subsMu.Lock()
	m.subs[ch] = struct{}{}
	m.subsMu.Unlock()
	return ch
}

func (m *Manager) Unsubscribe(ch chan struct{}) {
	m.subsMu.Lock()
	delete(m.subs, ch)
	m.subsMu.Unlock()
}

// GetSubscribersCount 返回当前订阅者数量
func (m *Manager) GetSubscribersCount() int {
	m.subsMu.Lock()
	defer m.subsMu.Unlock()
	return len(m.subs)
}

func (m *Manager) broadcast() {
	m.subsMu.Lock()
	for ch := range m.subs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
	m.subsMu.Unlock()
}
