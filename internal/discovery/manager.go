package discovery

import (
	"context"
	"encoding/json"
	"os"
	"sync"
	"time"

	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/logger"

	"gopkg.in/yaml.v3"
)

type Manager struct {
	cfg         *config.Config
	mu          sync.RWMutex
	products    map[string][]config.Product
	version     int64
	updatedAt   time.Time
	subsMu      sync.Mutex
	subs        map[chan struct{}]struct{}
	lastAccSig  string
	lastAccPath string
}

func NewManager(cfg *config.Config) *Manager {
	return &Manager{cfg: cfg, products: make(map[string][]config.Product), subs: make(map[chan struct{}]struct{})}
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

func (m *Manager) Refresh(ctx context.Context) error {
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
	if changed {
		m.version++
		m.updatedAt = time.Now()
		logger.Log.Infof("discovery refreshed version=%d", m.version)
	}
	m.mu.Unlock()
	if changed {
		m.broadcast()
	}
	if !m.noSavepoint() {
		if err := m.Save("configs/products.auto.yaml"); err != nil {
			logger.Log.Errorf("discovery save error: %v", err)
		}
	}
	return nil
}

func equalProducts(a, b map[string][]config.Product) bool {
	aj, _ := json.Marshal(a)
	bj, _ := json.Marshal(b)
	return string(aj) == string(bj)
}

func (m *Manager) Start(ctx context.Context) {
	p := os.Getenv("ACCOUNTS_PATH")
	m.lastAccPath = p
	m.lastAccSig = m.accountsSignature()
	_ = m.Refresh(ctx)
	go m.watchAccounts(ctx, p)
}

func (m *Manager) Save(path string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	data := struct {
		Products map[string][]config.Product `yaml:"products"`
	}{Products: m.products}
	bs, err := yaml.Marshal(data)
	if err != nil {
		return err
	}
	_ = os.MkdirAll("configs", 0755)
	return os.WriteFile(path, bs, 0644)
}

func (m *Manager) noSavepoint() bool {
	if m.cfg != nil && m.cfg.Server != nil {
		if m.cfg.Server.NoSavepoint {
			return true
		}
	}
	if m.cfg != nil && m.cfg.ServerConf != nil {
		if m.cfg.ServerConf.NoSavepoint {
			return true
		}
	}
	return false
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
		case <-time.After(3 * time.Second):
			fi, err := os.Stat(path)
			if err != nil {
				continue
			}
			mt := fi.ModTime().Unix()
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
		AccountsByProvider       map[string][]config.CloudAccount `yaml:"accounts"`
		AccountsByProviderLegacy map[string][]config.CloudAccount `yaml:"accounts_by_provider"`
		AccountsList             []config.CloudAccount            `yaml:"accounts_list"`
	}
	if err := yaml.Unmarshal([]byte(expanded), &accCfg); err != nil {
		return ""
	}
	if m.cfg != nil {
		m.cfg.Mu.Lock()
		m.cfg.AccountsByProvider = accCfg.AccountsByProvider
		m.cfg.AccountsByProviderLegacy = accCfg.AccountsByProviderLegacy
		m.cfg.AccountsList = accCfg.AccountsList
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
	accounts = append(accounts, m.cfg.AccountsList...)
	if m.cfg.AccountsByProvider != nil {
		for p, xs := range m.cfg.AccountsByProvider {
			for _, a := range xs {
				a.Provider = p
				accounts = append(accounts, a)
			}
		}
	}
	if m.cfg.AccountsByProviderLegacy != nil {
		for p, xs := range m.cfg.AccountsByProviderLegacy {
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
	sig := ""
	i := 0
	for k := range mset {
		if i > 0 {
			sig += "|"
		}
		sig += k.provider + "#" + k.resources
		i++
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
