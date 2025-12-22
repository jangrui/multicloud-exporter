package discovery

import (
	"context"
	"os"
	"reflect"
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
}

func NewManager(cfg *config.Config) *Manager {
	return &Manager{
		cfg:           cfg,
		products:      make(map[string][]config.Product),
		subs:          make(map[chan struct{}]struct{}),
		watchInterval: 3 * time.Second,
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

type DiscoveryStatus struct {
	Version           int64          `json:"version"`
	UpdatedAt         int64          `json:"updated_at"`
	AccountsPath      string         `json:"accounts_path"`
	AccountsSignature string         `json:"accounts_signature"`
	Subscribers       int            `json:"subscribers"`
	Providers         []string       `json:"providers"`
	ProductsCount     map[string]int `json:"products_count"`
}

func (m *Manager) Status() DiscoveryStatus {
	m.mu.RLock()
	providers := make([]string, 0, len(m.products))
	counts := make(map[string]int, len(m.products))
	for k, v := range m.products {
		providers = append(providers, k)
		counts[k] = len(v)
	}
	ver := m.version
	up := m.updatedAt.Unix()
	accPath := m.lastAccPath
	accSig := m.lastAccSig
	m.mu.RUnlock()
	m.subsMu.Lock()
	subs := len(m.subs)
	m.subsMu.Unlock()
	return DiscoveryStatus{
		Version:           ver,
		UpdatedAt:         up,
		AccountsPath:      accPath,
		AccountsSignature: accSig,
		Subscribers:       subs,
		Providers:         providers,
		ProductsCount:     counts,
	}
}

func (m *Manager) Refresh(ctx context.Context) error {
	start := time.Now()
	prods := make(map[string][]config.Product)
	m.cfg.Mu.RLock()
	for _, name := range GetAllDiscoverers() {
		if d, ok := GetDiscoverer(name); ok {
			if ps := d.Discover(ctx, m.cfg); len(ps) > 0 {
				prods[name] = ps
			}
		}
	}
	m.cfg.Mu.RUnlock()
	m.mu.Lock()
	changed := !equalProducts(m.products, prods)
	m.products = prods
	duration := time.Since(start)
	if changed {
		m.version++
		m.updatedAt = time.Now()
		logger.Log.Infof("发现服务已刷新，版本=%d，总耗时: %v", m.version, duration)
	} else {
		logger.Log.Infof("发现服务检查完成，无变化，总耗时: %v", duration)
	}
	m.mu.Unlock()
	if changed {
		m.broadcast()
	}
	return nil
}

func equalProducts(a, b map[string][]config.Product) bool {
	return reflect.DeepEqual(a, b)
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
		m.cfg.Mu.Lock()
		m.cfg.AccountsByProvider = accCfg.AccountsByProvider
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
