// 导出器主入口：负责加载配置、注册指标、定时触发采集并暴露 /metrics
package main

import (
    "context"
    "encoding/json"
    "net/http"
    "os"
    "strconv"
    "time"
    "crypto/subtle"

	"multicloud-exporter/internal/collector"
	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/discovery"
	"multicloud-exporter/internal/logger"
	"multicloud-exporter/internal/metrics"
	_ "multicloud-exporter/internal/metrics/aliyun"
	_ "multicloud-exporter/internal/metrics/tencent"

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

	logger.Log.Infof("配置加载完成，账号配置集合 sizes: accounts=%d products=%d", len(cfg.AccountsList)+len(cfg.AccountsByProvider)+len(cfg.AccountsByProviderLegacy), len(cfg.ProductsList)+len(cfg.ProductsByProvider)+len(cfg.ProductsByProviderLegacy))

	// 加载指标映射配置
	if mappingPath := os.Getenv("MAPPING_PATH"); mappingPath != "" {
		if err := config.LoadMetricMappings(mappingPath); err != nil {
			logger.Log.Warnf("Failed to load metric mappings from %s: %v", mappingPath, err)
		} else {
			logger.Log.Infof("Loaded metric mappings from %s", mappingPath)
		}
	} else {
		// 尝试加载默认位置的映射文件
		defaultPath := "configs/mappings/lb.metrics.yaml"
		if _, err := os.Stat(defaultPath); err == nil {
			if err := config.LoadMetricMappings(defaultPath); err != nil {
				logger.Log.Warnf("Failed to load metric mappings from %s: %v", defaultPath, err)
			} else {
				logger.Log.Infof("Loaded metric mappings from %s", defaultPath)
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
			port = "9100"
		}
	}

	// 确定采集间隔：默认 60s
	interval := 60 * time.Second

	// 1. 优先从配置文件读取
	if cfg.Server != nil && cfg.Server.ScrapeInterval != "" {
		if d, err := time.ParseDuration(cfg.Server.ScrapeInterval); err == nil {
			interval = d
		} else {
			logger.Log.Warnf("Warning: invalid scrape_interval in config: %v", err)
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
			logger.Log.Warnf("Warning: invalid SCRAPE_INTERVAL env: %v", err)
		}
	}

	mgr := discovery.NewManager(cfg)
	ctx := context.Background()
	mgr.Start(ctx)
	lastVer := int64(-1)
	prods := mgr.Get()
	if len(cfg.ProductsList) == 0 && len(cfg.ProductsByProvider) == 0 && len(cfg.ProductsByProviderLegacy) == 0 && len(prods) > 0 {
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
    http.HandleFunc("/api/discovery/config", wrap(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        data := struct {
            Version   int64                       `json:"version"`
            UpdatedAt int64                       `json:"updated_at"`
            Products  map[string][]config.Product `json:"products"`
        }{Version: mgr.Version(), UpdatedAt: mgr.UpdatedAt().Unix(), Products: mgr.Get()}
        _ = json.NewEncoder(w).Encode(data)
    }))
    http.HandleFunc("/api/discovery/stream", wrap(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "text/event-stream")
        w.Header().Set("Cache-Control", "no-cache")
        w.Header().Set("Connection", "keep-alive")
        fl, _ := w.(http.Flusher)
        ch := mgr.Subscribe()
        defer mgr.Unsubscribe(ch)
        _, _ = w.Write([]byte("event: init\n"))
        _, _ = w.Write([]byte("data: {}\n\n"))
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
	logger.Log.Infof("服务启动，端口=%s", port)
	logger.Log.Fatal(http.ListenAndServe(":"+port, nil))
}
