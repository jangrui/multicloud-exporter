// 导出器主入口：负责加载配置、注册指标、定时触发采集并暴露 /metrics
package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strconv"
	"time"

	"multicloud-exporter/internal/collector"
	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/discovery"
	"multicloud-exporter/internal/logger"
	"multicloud-exporter/internal/metrics"
	_ "multicloud-exporter/internal/metrics/aliyun"
	_ "multicloud-exporter/internal/metrics/tencent"
	"path/filepath"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// main 启动 HTTP 服务并周期性采集各云资源指标
func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		logger.Log.Fatalf("Failed to load config: %v", err)
	}
	if cfg.Server != nil && cfg.Server.Log != nil {
		logger.Init(cfg.Server.Log)
	}
	defer logger.Sync()

	// 统计账号数量
	totalAccounts := 0
	accountsByProvider := make(map[string]int)
	for provider, accounts := range cfg.AccountsByProvider {
		count := len(accounts)
		totalAccounts += count
		accountsByProvider[provider] = count
	}

	// 构建账号统计信息
	var accountInfo strings.Builder
	if len(accountsByProvider) > 0 {
		accountInfo.WriteString(" (")
		providers := make([]string, 0, len(accountsByProvider))
		for provider := range accountsByProvider {
			providers = append(providers, provider)
		}
		// 按云平台名称排序
		sort.Strings(providers)
		for i, provider := range providers {
			if i > 0 {
				accountInfo.WriteString(", ")
			}
			accountInfo.WriteString(fmt.Sprintf("%s=%d", provider, accountsByProvider[provider]))
		}
		accountInfo.WriteString(")")
	}

	// 统计产品数量（产品配置已废弃，全面采用自动发现）
	totalProducts := 0
	for _, products := range cfg.ProductsByProvider {
		totalProducts += len(products)
	}

	logger.Log.Infof("配置加载完成，账号配置集合 sizes: accounts=%d%s products=%d", totalAccounts, accountInfo.String(), totalProducts)

	// 加载指标映射配置
	if mappingPath := os.Getenv("MAPPING_PATH"); mappingPath != "" {
		paths := strings.Split(mappingPath, ",")
		loaded := 0
		for _, p := range paths {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			fi, err := os.Stat(p)
			if err != nil {
				logger.Log.Warnf("映射路径未找到: %s (%v)", p, err)
				continue
			}
			if fi.IsDir() {
				files, err := filepath.Glob(filepath.Join(p, "*.yaml"))
				if err != nil || len(files) == 0 {
					logger.Log.Warnf("目录中无映射文件: %s", p)
					continue
				}
				for _, f := range files {
					if err := config.ValidateMappingStructure(f); err != nil {
						logger.Log.Warnf("指标映射验证失败，文件=%s 错误=%v", f, err)
						continue
					}
					if err := config.LoadMetricMappings(f); err != nil {
						logger.Log.Warnf("加载指标映射失败，文件=%s 错误=%v", f, err)
						continue
					}
					loaded++
					logger.Log.Infof("已加载指标映射，文件=%s", f)
				}
			} else {
				if err := config.ValidateMappingStructure(p); err != nil {
					logger.Log.Warnf("指标映射验证失败，文件=%s 错误=%v", p, err)
				}
				if err := config.LoadMetricMappings(p); err != nil {
					logger.Log.Warnf("加载指标映射失败，文件=%s 错误=%v", p, err)
				} else {
					loaded++
					logger.Log.Infof("已加载指标映射，文件=%s", p)
				}
			}
		}
		if loaded == 0 {
			logger.Log.Warnf("未从 MAPPING_PATH=%s 加载任何指标映射", mappingPath)
		} else {
			logger.Log.Infof("已加载指标映射，数量=%d 来源=MAPPING_PATH", loaded)
		}
	} else {
		// 尝试加载默认位置的所有映射文件
		mappingDir := "configs/mappings"
		if err := config.ValidateAllMappings(mappingDir); err != nil {
			logger.Log.Warnf("指标映射验证发现问题:\n%v", err)
		} else {
			logger.Log.Infof("指标映射验证通过，目录=%s", mappingDir)
		}
		files, err := filepath.Glob(filepath.Join(mappingDir, "*.yaml"))
		if err == nil && len(files) > 0 {
			for _, f := range files {
				if err := config.ValidateMappingStructure(f); err != nil {
					logger.Log.Warnf("指标映射验证失败，文件=%s 错误=%v", f, err)
					continue
				}
				if err := config.LoadMetricMappings(f); err != nil {
					logger.Log.Warnf("加载指标映射失败，文件=%s 错误=%v", f, err)
				} else {
					logger.Log.Infof("已加载指标映射，文件=%s", f)
				}
			}
		} else {
			// Fallback to legacy single file check if glob fails or empty (though glob returns nil err on no match usually)
			defaultPath := "configs/mappings/clb.metrics.yaml"
			if _, err := os.Stat(defaultPath); err == nil {
				if err := config.ValidateMappingStructure(defaultPath); err != nil {
					logger.Log.Warnf("指标映射验证失败，文件=%s 错误=%v", defaultPath, err)
				}
				if err := config.LoadMetricMappings(defaultPath); err != nil {
					logger.Log.Warnf("加载指标映射失败，文件=%s 错误=%v", defaultPath, err)
				} else {
					logger.Log.Infof("已加载指标映射，文件=%s", defaultPath)
				}
			}
		}
	}

	port := os.Getenv("EXPORTER_PORT")
	if port == "" {
		if cfg.Server != nil && cfg.Server.Port > 0 {
			port = strconv.Itoa(cfg.Server.Port)
		} else if cfg.ServerConf != nil && cfg.ServerConf.Port > 0 {
			port = strconv.Itoa(cfg.ServerConf.Port)
		} else {
			port = "9101"
		}
	}

	// 确定采集间隔：默认 60s
	interval := 60 * time.Second

	// 1. 优先从配置文件读取
	if cfg.Server != nil && cfg.Server.ScrapeInterval != "" {
		if d, err := time.ParseDuration(cfg.Server.ScrapeInterval); err == nil {
			interval = d
		} else {
			logger.Log.Warnf("警告: 配置中的 scrape_interval 无效: %v", err)
		}
	} else if cfg.ServerConf != nil && cfg.ServerConf.ScrapeInterval != "" {
		if d, err := time.ParseDuration(cfg.ServerConf.ScrapeInterval); err == nil {
			interval = d
		}
	}

	// 2. 环境变量覆盖 (SCRAPE_INTERVAL)
	if envInterval := os.Getenv("SCRAPE_INTERVAL"); envInterval != "" {
		if i, err := strconv.Atoi(envInterval); err == nil {
			interval = time.Duration(i) * time.Second
		} else if d, err := time.ParseDuration(envInterval); err == nil {
			interval = d
		} else {
			logger.Log.Warnf("警告: 环境变量 SCRAPE_INTERVAL 无效: %v", err)
		}
	}

	mgr := discovery.NewManager(cfg)
	ctx := context.Background()
	discoveryStart := time.Now()
	mgr.Start(ctx)
	discoveryDuration := time.Since(discoveryStart)
	lastVer := int64(-1)
	prods := mgr.Get()
	// 统计发现的产品数量
	discoveredTotalProducts := 0
	productsByProvider := make(map[string]int)
	for provider, products := range prods {
		count := len(products)
		discoveredTotalProducts += count
		productsByProvider[provider] = count
	}
	// 构建产品统计信息
	var productInfo strings.Builder
	if len(productsByProvider) > 0 {
		productInfo.WriteString(" (")
		providers := make([]string, 0, len(productsByProvider))
		for provider := range productsByProvider {
			providers = append(providers, provider)
		}
		sort.Strings(providers)
		for i, provider := range providers {
			if i > 0 {
				productInfo.WriteString(", ")
			}
			productInfo.WriteString(fmt.Sprintf("%s=%d", provider, productsByProvider[provider]))
		}
		productInfo.WriteString(")")
	}
	logger.Log.Infof("发现服务启动完成，总耗时: %v，发现产品数量: %d%s，版本=%d", discoveryDuration, discoveredTotalProducts, productInfo.String(), mgr.Version())
	if len(cfg.ProductsByProvider) == 0 && len(prods) > 0 {
		cfg.ProductsByProvider = prods
		lastVer = mgr.Version()
	}
	coll := collector.NewCollector(cfg, mgr)

	prometheus.MustRegister(metrics.ResourceMetric)
	prometheus.MustRegister(metrics.RequestTotal)
	prometheus.MustRegister(metrics.RequestDuration)
	prometheus.MustRegister(metrics.NamespaceMetric)
	prometheus.MustRegister(metrics.RateLimitTotal)
	prometheus.MustRegister(metrics.CollectionDuration)

	// 周期性采集，采集间隔由配置或环境变量控制
	go func() {
		for {
			start := time.Now()
			logger.Log.Infof("开始采集，周期=%v", interval)
			// 仅在发现配置发生变化时才重置指标，避免每周期短暂丢失导致的图表断点
			versionChanged := false
			if v := mgr.Version(); v != lastVer {
				cfg.ProductsByProvider = mgr.Get()
				lastVer = v
				versionChanged = true
			}
			if versionChanged {
				metrics.Reset()
			}
			coll.Collect()
			duration := time.Since(start)
			metrics.CollectionDuration.Observe(duration.Seconds())
			logger.Log.Infof("==========================================")
			logger.Log.Infof("采集周期完成，总耗时: %v", duration)
			logger.Log.Infof("==========================================")
			logger.Log.Infof("采集完成，休眠 %v", interval)
			time.Sleep(interval)
		}
	}()

	http.Handle("/metrics", promhttp.Handler())
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	wrap := func(h http.HandlerFunc) http.HandlerFunc {
		enabled := false
		var pairs []config.BasicAuth
		if ev := os.Getenv("ADMIN_AUTH_ENABLED"); ev != "" {
			if ev == "1" || strings.EqualFold(ev, "true") || strings.EqualFold(ev, "yes") {
				enabled = true
			}
		}
		if enabled {
			if raw := os.Getenv("ADMIN_AUTH"); raw != "" {
				var xs []config.BasicAuth
				if json.Unmarshal([]byte(raw), &xs) == nil && len(xs) > 0 {
					pairs = xs
				} else {
					for _, seg := range strings.Split(raw, ",") {
						kv := strings.SplitN(strings.TrimSpace(seg), ":", 2)
						if len(kv) == 2 && kv[0] != "" {
							pairs = append(pairs, config.BasicAuth{Username: kv[0], Password: kv[1]})
						}
					}
				}
			}
			// 支持从 ADMIN_USERNAME/ADMIN_PASSWORD 构造单个账号
			u := os.Getenv("ADMIN_USERNAME")
			p := os.Getenv("ADMIN_PASSWORD")
			if u != "" && p != "" {
				pairs = append(pairs, config.BasicAuth{Username: u, Password: p})
			}
		}
		if !enabled {
			if cfg.Server != nil {
				if cfg.Server.AdminAuthEnabled {
					enabled = true
					pairs = cfg.Server.AdminAuth
				}
			}
			if !enabled && cfg.ServerConf != nil {
				if cfg.ServerConf.AdminAuthEnabled {
					enabled = true
					pairs = cfg.ServerConf.AdminAuth
				}
			}
		}
		if !enabled || len(pairs) == 0 {
			return h
		}
		return func(w http.ResponseWriter, r *http.Request) {
			u, p, ok := r.BasicAuth()
			if !ok {
				w.Header().Set("WWW-Authenticate", "Basic realm=restricted")
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			authed := false
			for _, pair := range pairs {
				if subtle.ConstantTimeCompare([]byte(u), []byte(pair.Username)) == 1 && subtle.ConstantTimeCompare([]byte(p), []byte(pair.Password)) == 1 {
					authed = true
					break
				}
			}
			if !authed {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			h(w, r)
		}
	}
	http.HandleFunc("/collect", wrap(func(w http.ResponseWriter, r *http.Request) {
		provider := r.URL.Query().Get("provider")
		resource := r.URL.Query().Get("resource")
		go coll.CollectFiltered(provider, resource)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "triggered", "provider": provider, "resource": resource})
	}))
	http.HandleFunc("/status", wrap(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(coll.GetStatus())
	}))
	http.HandleFunc("/api/discovery/config", wrap(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		data := struct {
			Version   int64                       `json:"version"`
			UpdatedAt int64                       `json:"updated_at"`
			Products  map[string][]config.Product `json:"products"`
		}{Version: mgr.Version(), UpdatedAt: mgr.UpdatedAt().Unix(), Products: mgr.Get()}
		bs, err := json.MarshalIndent(data, "", "  ")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(bs)
	}))
	http.HandleFunc("/api/discovery/stream", wrap(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		fl, _ := w.(http.Flusher)
		ch := mgr.Subscribe()
		defer mgr.Unsubscribe(ch)
		initPayload := struct {
			Version int64 `json:"version"`
		}{Version: mgr.Version()}
		bs, _ := json.Marshal(initPayload)
		_, _ = w.Write([]byte("event: init\n"))
		_, _ = w.Write([]byte("data: "))
		_, _ = w.Write(bs)
		_, _ = w.Write([]byte("\n\n"))
		if fl != nil {
			fl.Flush()
		}
		for {
			select {
			case <-r.Context().Done():
				return
			case <-ch:
				payload := struct {
					Version int64 `json:"version"`
				}{Version: mgr.Version()}
				bs, _ := json.Marshal(payload)
				_, _ = w.Write([]byte("event: update\n"))
				_, _ = w.Write([]byte("data: "))
				_, _ = w.Write(bs)
				_, _ = w.Write([]byte("\n\n"))
				if fl != nil {
					fl.Flush()
				}
			}
		}
	}))
	http.HandleFunc("/api/discovery/status", wrap(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		st := mgr.Status()
		resp := struct {
			discovery.DiscoveryStatus
			APIStats []metrics.APIStat `json:"api_stats"`
		}{
			DiscoveryStatus: st,
			APIStats:        metrics.GetAPIStats(),
		}
		bs, err := json.MarshalIndent(resp, "", "  ")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(bs)
	}))
	logger.Log.Infof("服务启动，端口=%s", port)
	logger.Log.Fatal(http.ListenAndServe(":"+port, nil))
}
