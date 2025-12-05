// 导出器主入口：负责加载配置、注册指标、定时触发采集并暴露 /metrics
package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"multicloud-exporter/internal/collector"
	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/discovery"
	"multicloud-exporter/internal/metrics"
	_ "multicloud-exporter/internal/metrics/aliyun"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// main 启动 HTTP 服务并周期性采集各云资源指标
func main() {
	port := os.Getenv("EXPORTER_PORT")
	intervalStr := os.Getenv("SCRAPE_INTERVAL")
	if intervalStr == "" {
		intervalStr = "60"
	}

	interval, err := strconv.Atoi(intervalStr)
	if err != nil {
		log.Fatal("Invalid SCRAPE_INTERVAL")
	}

	cfg := config.LoadConfig()
	log.Printf("配置加载完成，账号配置集合 sizes: accounts=%d products=%d", len(cfg.AccountsList)+len(cfg.AccountsByProvider)+len(cfg.AccountsByProviderLegacy), len(cfg.ProductsList)+len(cfg.ProductsByProvider)+len(cfg.ProductsByProviderLegacy))
	if port == "" {
		if cfg.Server != nil && cfg.Server.Port > 0 {
			port = strconv.Itoa(cfg.Server.Port)
		} else if cfg.ServerConf != nil && cfg.ServerConf.Port > 0 {
			port = strconv.Itoa(cfg.ServerConf.Port)
		} else {
			port = "9100"
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

	// 周期性采集，采集间隔由环境变量 SCRAPE_INTERVAL 控制
	go func() {
		for {
			log.Printf("开始采集，周期=%ds", interval)
			if v := mgr.Version(); v != lastVer {
				cfg.ProductsByProvider = mgr.Get()
				lastVer = v
			}
			coll.Collect()
			log.Printf("采集完成，休眠 %d 秒", interval)
			time.Sleep(time.Duration(interval) * time.Second)
		}
	}()

	http.Handle("/metrics", promhttp.Handler())
	http.HandleFunc("/api/discovery/config", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		data := struct {
			Version   int64                       `json:"version"`
			UpdatedAt int64                       `json:"updated_at"`
			Products  map[string][]config.Product `json:"products"`
		}{Version: mgr.Version(), UpdatedAt: mgr.UpdatedAt().Unix(), Products: mgr.Get()}
		_ = json.NewEncoder(w).Encode(data)
	})
	http.HandleFunc("/api/discovery/stream", func(w http.ResponseWriter, r *http.Request) {
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
	})
	log.Printf("服务启动，端口=%s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
