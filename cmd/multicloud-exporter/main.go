// 导出器主入口：负责加载配置、注册指标、定时触发采集并暴露 /metrics
package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"multicloud-exporter/internal/collector"
	"multicloud-exporter/internal/logger"
	_ "multicloud-exporter/internal/metrics/aliyun"
	_ "multicloud-exporter/internal/metrics/tencent"
)

// global context for graceful shutdown
var (
	shutdownCtx    context.Context
	shutdownCancel context.CancelFunc
)

// main 启动 HTTP 服务并周期性采集各云资源指标
func main() {
	// 设置信号处理，实现优雅关闭
	setupSignalHandler()

	// 1. 加载配置
	cfg, err := setupConfig()
	if err != nil {
		logger.Log.Fatalf("Failed to setup config: %v", err)
	}

	// 验证配置
	if err := cfg.Validate(); err != nil {
		logger.Log.Fatalf("Config validation failed: %v", err)
	}

	defer logger.Sync()

	// 2. 记录账号统计
	logAccountStats(cfg)

	// 3. 加载指标映射
	setupMetricMappings(cfg)

	// 4. 获取服务端口和采集间隔
	port := getServerPort(cfg)
	interval := getScrapeInterval(cfg)

	// 5. 初始化发现管理器（必须成功）
	mgr, err := initializeDiscovery(cfg)
	if err != nil {
		logger.Log.Fatalf("Failed to initialize discovery: %v", err)
	}

	// 6. 创建采集器
	coll := collector.NewCollector(cfg, mgr)

	// 7. 注册 Prometheus 指标
	registerPrometheusMetrics()

	// 8. 启动周期性采集（支持优雅停止）
	startCollectionLoop(shutdownCtx, cfg, coll, mgr, interval)

	// 9. 设置 HTTP 路由
	setupHTTPHandlers(cfg, coll, mgr)

	// 10. 启动 HTTP 服务器
	logger.Log.Infof("HTTP 服务启动，监听端口=%s", port)

	// 在 goroutine 中启动 HTTP 服务器
	serverErr := make(chan error, 1)
	go func() {
		addr := ":" + port
		logger.Log.Infof("开始监听 %s", addr)
		if err := http.ListenAndServe(addr, nil); err != nil {
			serverErr <- err
		}
	}()

	// 等待关闭信号或服务器错误
	select {
	case <-shutdownCtx.Done():
		logger.Log.Info("收到关闭信号，正在停止服务...")
	case err := <-serverErr:
		logger.Log.Fatalf("HTTP 服务器错误: %v", err)
	}

	// 给 HTTP 服务器一点时间处理最后的请求
	shutdownCancel()
}

// setupSignalHandler 设置信号处理器
func setupSignalHandler() {
	shutdownCtx, shutdownCancel = context.WithCancel(context.Background())

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		logger.Log.Infof("收到信号 %v，开始优雅关闭...", sig)
		shutdownCancel()
	}()
}
