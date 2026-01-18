// 导出器主入口：负责加载配置、注册指标、定时触发采集并暴露 /metrics
package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"multicloud-exporter/internal/cluster"
	"multicloud-exporter/internal/collector"
	"multicloud-exporter/internal/logger"
	"multicloud-exporter/internal/providers/common"
	"multicloud-exporter/internal/utils"
)

// global context for graceful shutdown
var (
	shutdownCtx    context.Context
	shutdownCancel context.CancelFunc
	reloadMu       sync.RWMutex
)

// main 启动 HTTP 服务并周期性采集各云资源指标
func main() {
	// 设置信号处理，实现优雅关闭
	setupSignalHandler()

	// 1. 加载配置
	cfg, err := setupConfig()
	if err != nil {
		ctxLog := logger.NewContextLogger("Main", "resource_type", "Config")
		ctxLog.Errorf("Failed to setup config: %v", err)
		os.Exit(1)
	}

	// 验证配置
	if err := cfg.Validate(); err != nil {
		ctxLog := logger.NewContextLogger("Main", "resource_type", "Config")
		ctxLog.Errorf("Config validation failed: %v", err)
		os.Exit(1)
	}

	defer logger.Sync()

	// 2. 记录账号统计
	logAccountStats(cfg)

	// 3. 加载指标映射
	setupMetricMappings(cfg)

	// 4. 获取服务端口和采集间隔
	port := getServerPort(cfg)
	interval := getScrapeInterval(cfg)

	// 设置分片配置缓存 TTL 为采集间隔的 1/2
	// 这样可以在滚动更新时更快感知到拓扑变化，减少分片不一致的时间窗口
	utils.SetClusterConfigTTL(interval / 2)

	// 记录集群配置信息
	discoveryType := os.Getenv("CLUSTER_DISCOVERY")
	total, index := utils.ClusterConfig()
	ctxLog := logger.NewContextLogger("Main", "resource_type", "ClusterConfig")
	if discoveryType != "" {
		ctxLog.Infof("集群配置初始化: 发现方式=%s, 总Pod数=%d, 当前索引=%d, 缓存TTL=%v",
			discoveryType, total, index, interval)
	} else {
		ctxLog.Infof("集群配置初始化: 单实例模式, total=%d, index=%d", total, index)
	}

	// 5. 初始化发现管理器（必须成功）
	mgr, err := initializeDiscovery(cfg)
	if err != nil {
		ctxLog = logger.NewContextLogger("Main", "resource_type", "Discovery")
		ctxLog.Errorf("Failed to initialize discovery: %v", err)
		os.Exit(1)
	}

	// 6. 初始化集群同步管理器（如果启用）
	var clusterMgr *cluster.SyncManager
	if cfg.GetServer() != nil && cfg.GetServer().Cluster != nil && cfg.GetServer().Cluster.Enabled {
		ctxLog := logger.NewContextLogger("Main", "resource_type", "Cluster")
		ctxLog.Info("初始化集群同步管理器...")
		clusterMgr = cluster.NewSyncManager(
			cfg.GetServer().Cluster.ServiceName,
			cfg.GetServer().Cluster.Port,
			cfg.GetServer().Cluster.Secret,
		)
		// 启动自动发现（后台协程）
		go clusterMgr.Start(shutdownCtx)
	}

	// 7. 创建采集器
	coll := collector.NewCollector(cfg, mgr, clusterMgr)

	// 8. 注册 Prometheus 指标
	registerPrometheusMetrics()

	// 9. 启动自动恢复协程（如果有降级管理器）
	degradeMgr := coll.GetDegradationManager()
	if degradeMgr != nil {
		ctxLog := logger.NewContextLogger("Main", "resource_type", "AutoRecovery")
		recoverFunc := func(key string, rtype common.ResourceType) bool {
			ctxLog.Infof("尝试恢复资源: key=%s type=%s", key, rtype)
			return true
		}
		go degradeMgr.StartAutoRecovery(recoverFunc, shutdownCtx)
		ctxLog.Info("自动恢复协程已启动")
	}

	// 10. 启动周期性采集（支持优雅停止）
	startCollectionLoop(shutdownCtx, cfg, coll, mgr, interval)

	// 10. 设置 HTTP 路由
	setupHTTPHandlers(cfg, coll, mgr, clusterMgr)

	// 11. 启动 HTTP 服务器
	ctxLog = logger.NewContextLogger("Main", "resource_type", "HTTPServer")
	ctxLog.Infof("HTTP 服务启动，监听端口=%s", port)

	// 在 goroutine 中启动 HTTP 服务器
	serverErr := make(chan error, 1)
	go func() {
		addr := ":" + port
		ctxLog := logger.NewContextLogger("Main", "resource_type", "HTTPServer")
		ctxLog.Infof("开始监听 %s", addr)
		if err := http.ListenAndServe(addr, nil); err != nil {
			serverErr <- err
		}
	}()

	// 等待关闭信号或服务器错误
	select {
	case <-shutdownCtx.Done():
		ctxLog := logger.NewContextLogger("Main", "resource_type", "HTTPServer")
		ctxLog.Info("收到关闭信号，正在停止服务...")
	case err := <-serverErr:
		ctxLog := logger.NewContextLogger("Main", "resource_type", "HTTPServer")
		ctxLog.Errorf("HTTP 服务器错误: %v", err)
		os.Exit(1)
	}

	// 给 HTTP 服务器一点时间处理最后的请求
	shutdownCancel()
}

// setupSignalHandler 设置信号处理器
func setupSignalHandler() {
	shutdownCtx, shutdownCancel = context.WithCancel(context.Background())

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	go func() {
		for sig := range sigCh {
			ctxLog := logger.NewContextLogger("Main", "resource_type", "SignalHandler")
			if sig == syscall.SIGHUP {
				ctxLog.Info("收到 SIGHUP 信号，触发热加载...")
				handleReload(ctxLog)
			} else {
				ctxLog.Infof("收到信号 %v，开始优雅关闭...", sig)
				shutdownCancel()
			}
		}
	}()
}

// handleReload 处理热加载请求
func handleReload(ctxLog *logger.ContextLogger) {
	reloadMu.Lock()
	defer reloadMu.Unlock()

	ctxLog.Info("开始热加载配置...")

	newCfg, err := setupConfig()
	if err != nil {
		ctxLog.Errorf("热加载失败: 加载配置失败: %v", err)
		return
	}

	if err := newCfg.Validate(); err != nil {
		ctxLog.Errorf("热加载失败: 配置验证失败: %v", err)
		return
	}

	ctxLog.Info("配置热加载成功")

	ctxLog.Info("配置热加载成功")
}
