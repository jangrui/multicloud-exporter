// 导出器主入口：负责加载配置、注册指标、定时触发采集并暴露 /metrics
package main

import (
    "log"
    "net/http"
    "os"
    "strconv"
    "time"

    "multicloud-exporter/internal/collector"
    "multicloud-exporter/internal/config"
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
	coll := collector.NewCollector(cfg)

	prometheus.MustRegister(metrics.ResourceMetric)
	prometheus.MustRegister(metrics.RequestTotal)
	prometheus.MustRegister(metrics.RequestDuration)
	prometheus.MustRegister(metrics.NamespaceMetric)

	// 周期性采集，采集间隔由环境变量 SCRAPE_INTERVAL 控制
	go func() {
		for {
			log.Printf("开始采集，周期=%ds", interval)
			coll.Collect()
			log.Printf("采集完成，休眠 %d 秒", interval)
			time.Sleep(time.Duration(interval) * time.Second)
		}
	}()

	// 暴露 Prometheus 兼容的 /metrics 端点
	http.Handle("/metrics", promhttp.Handler())
	log.Printf("服务启动，端口=%s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
