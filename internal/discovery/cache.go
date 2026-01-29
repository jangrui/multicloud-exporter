package discovery

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/metrics"
)

const (
	defaultDiscoveryCacheTTL = time.Hour
	cacheTypeDiscoveryMeta   = "discovery_meta"
)

type discoveryMetaCacheEntry struct {
	Provider     string               `json:"provider"`
	Namespace    string               `json:"namespace"`
	AccountID    string               `json:"account_id"`
	MetricGroups []config.MetricGroup `json:"metric_groups"`
	Fingerprint  string               `json:"fingerprint"`
	UpdatedAt    time.Time            `json:"updated_at"`
}

type discoveryMetaCache struct {
	mu        sync.Mutex
	ttl       time.Duration
	entries   map[string]discoveryMetaCacheEntry
	hitCount  int
	missCount int
}

var discoveryMetaCacheOnce sync.Once
var discoveryMetaCacheInstance *discoveryMetaCache

func getDiscoveryMetaCache(cfg *config.Config) *discoveryMetaCache {
	if cfg == nil {
		return nil
	}
	discoveryMetaCacheOnce.Do(func() {
		discoveryMetaCacheInstance = newDiscoveryMetaCache(cfg)
	})
	return discoveryMetaCacheInstance
}

func newDiscoveryMetaCache(cfg *config.Config) *discoveryMetaCache {
	ttl := defaultDiscoveryCacheTTL
	if cfg.GetServer() != nil && cfg.GetServer().DiscoveryTTL != "" {
		if d, err := parseDiscoveryTTL(cfg.GetServer().DiscoveryTTL); err == nil {
			ttl = d
		}
	}

	cache := &discoveryMetaCache{
		ttl:     ttl,
		entries: make(map[string]discoveryMetaCacheEntry),
	}
	return cache
}

func parseDiscoveryTTL(value string) (time.Duration, error) {
	if value == "" {
		return 0, fmt.Errorf("empty duration string")
	}
	if strings.HasSuffix(value, "d") {
		days := strings.TrimSuffix(value, "d")
		var d int
		if _, err := fmt.Sscanf(days, "%d", &d); err != nil {
			return 0, fmt.Errorf("invalid duration format: %s", value)
		}
		return time.Duration(d) * 24 * time.Hour, nil
	}
	return time.ParseDuration(value)
}

func buildDiscoveryCacheKey(provider, namespace, accountID string) string {
	return provider + "|" + namespace + "|" + accountID
}

func buildAccountFingerprint(acc config.CloudAccount) string {
	var builder strings.Builder
	builder.WriteString(acc.Provider)
	builder.WriteString("|")
	builder.WriteString(acc.AccountID)
	builder.WriteString("|")

	regions := append([]string{}, acc.Regions...)
	sort.Strings(regions)
	builder.WriteString(strings.Join(regions, ","))
	builder.WriteString("|")

	resources := append([]string{}, acc.Resources...)
	sort.Strings(resources)
	builder.WriteString(strings.Join(resources, ","))
	builder.WriteString("|")

	if acc.ProductMetric != nil {
		keys := make([]string, 0, len(acc.ProductMetric))
		for k := range acc.ProductMetric {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, key := range keys {
			builder.WriteString(key)
			builder.WriteString("=")
			groups := acc.ProductMetric[key]
			groupStrings := make([]string, 0, len(groups))
			for _, g := range groups {
				metrics := append([]string{}, g.MetricList...)
				sort.Strings(metrics)
				period := ""
				if g.Period != nil {
					period = fmt.Sprintf("%d", *g.Period)
				}
				groupStrings = append(groupStrings, period+":"+strings.Join(metrics, ","))
			}
			sort.Strings(groupStrings)
			builder.WriteString(strings.Join(groupStrings, "|"))
			builder.WriteString(";")
		}
	}

	hash := sha256.Sum256([]byte(builder.String()))
	return hex.EncodeToString(hash[:])
}

func (c *discoveryMetaCache) Get(provider, namespace string, acc config.CloudAccount) ([]config.MetricGroup, bool) {
	if c == nil || acc.AccountID == "" {
		return nil, false
	}
	key := buildDiscoveryCacheKey(provider, namespace, acc.AccountID)
	fingerprint := buildAccountFingerprint(acc)

	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[key]
	if !ok {
		c.recordMiss()
		return nil, false
	}
	if entry.Fingerprint != fingerprint {
		delete(c.entries, key)
		c.recordMiss()
		return nil, false
	}
	if time.Since(entry.UpdatedAt) > c.ttl {
		delete(c.entries, key)
		c.recordMiss()
		return nil, false
	}

	c.recordHit()
	return entry.MetricGroups, true
}

func (c *discoveryMetaCache) Set(provider, namespace string, acc config.CloudAccount, metricGroups []config.MetricGroup) {
	if c == nil || acc.AccountID == "" {
		return
	}
	key := buildDiscoveryCacheKey(provider, namespace, acc.AccountID)
	entry := discoveryMetaCacheEntry{
		Provider:     provider,
		Namespace:    namespace,
		AccountID:    acc.AccountID,
		MetricGroups: metricGroups,
		Fingerprint:  buildAccountFingerprint(acc),
		UpdatedAt:    time.Now(),
	}

	c.mu.Lock()
	c.entries[key] = entry
	c.mu.Unlock()
}

func (c *discoveryMetaCache) recordHit() {
	c.hitCount++
	metrics.CacheHitTotal.WithLabelValues(cacheTypeDiscoveryMeta).Inc()
	c.updateHitRatio()
}

func (c *discoveryMetaCache) recordMiss() {
	c.missCount++
	metrics.CacheMissTotal.WithLabelValues(cacheTypeDiscoveryMeta).Inc()
	c.updateHitRatio()
}

func (c *discoveryMetaCache) updateHitRatio() {
	total := c.hitCount + c.missCount
	if total == 0 {
		return
	}
	ratio := float64(c.hitCount) / float64(total)
	metrics.CacheHitRatio.WithLabelValues(cacheTypeDiscoveryMeta).Set(ratio)
}
