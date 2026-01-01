package main

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"multicloud-exporter/internal/collector"
	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/discovery"
	"multicloud-exporter/internal/logger"
	"multicloud-exporter/internal/metrics"
	"multicloud-exporter/internal/utils"
)

// startCollectionLoop 启动周期性采集循环（支持优雅停止和智能首次采集）
//
// 首次采集策略（自适应错峰）：
//   - 单/双Pod场景：立即采集，无需等待
//   - 3-10个Pod（中等规模）：线性延迟 + 随机抖动
//   - >10个Pod（大规模）：指数退避延迟，避免API压力
//
// 环境变量控制：
//   - FIRST_RUN_STRATEGY: auto（自动）| immediate（立即）| staggered（强制错峰）
//   - FIRST_RUN_MAX_DELAY: 最大延迟秒数（默认180秒）
func startCollectionLoop(ctx context.Context, cfg *config.Config, coll *collector.Collector, mgr *discovery.Manager, interval time.Duration) {
	go func() {
		lastVer := int64(-1)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		// ========== 智能首次采集策略 ==========
		// 获取集群分片配置
		totalShards, shardIndex := utils.ClusterConfig()

		// 计算首次采集延迟
		firstRunDelay := calculateFirstRunDelay(totalShards, shardIndex, interval)

		if firstRunDelay > 0 {
			ctxLog := logger.NewContextLogger("Collection", "resource_type", "FirstRun")
			ctxLog.Infof("首次采集延迟策略: 分片总数=%d, 当前索引=%d, 延迟=%v",
				totalShards, shardIndex, firstRunDelay)

			select {
			case <-time.After(firstRunDelay):
				// 延迟结束，继续执行首次采集
			case <-ctx.Done():
				ctxLog := logger.NewContextLogger("Collection", "resource_type", "FirstRun")
				ctxLog.Info("收到停止信号，取消首次采集")
				return
			}
		} else {
			ctxLog := logger.NewContextLogger("Collection", "resource_type", "FirstRun")
			ctxLog.Infof("首次采集策略: 立即执行 (分片总数=%d, 当前索引=%d)",
				totalShards, shardIndex)
		}

		// 执行首次采集
		ctxLog := logger.NewContextLogger("Collection", "resource_type", "FirstRun")
		ctxLog.Info("开始首次采集...")
		coll.Collect()
		ctxLog.Info("首次采集完成，进入定时采集循环")
		// ========== 智能首次采集结束 ==========

		for {
			select {
			case <-ctx.Done():
				ctxLog := logger.NewContextLogger("Collection", "resource_type", "CollectionLoop")
				ctxLog.Info("采集循环收到停止信号，正在退出...")
				return

			case <-ticker.C:
				start := time.Now()
				collectionLog := logger.NewContextLogger("Collection", "resource_type", "CollectionLoop")
				collectionLog.Infof("开始采集，周期=%v", interval)

				// 检查配置版本是否变化
				versionChanged := false
				if v := mgr.Version(); v != lastVer {
					cfg.ProductsByProvider = mgr.Get()
					lastVer = v
					versionChanged = true
				}

				// 版本变化时重置指标
				if versionChanged {
					metrics.Reset()
				}

				// 执行采集
				coll.Collect()
				duration := time.Since(start)
				metrics.CollectionDuration.Observe(duration.Seconds())

				collectionLog.Infof("==========================================")
				collectionLog.Infof("采集周期完成，总耗时: %v", duration)
				collectionLog.Infof("==========================================")
			}
		}
	}()
}

// initializeDiscovery 初始化发现服务并返回管理器
func initializeDiscovery(cfg *config.Config) (*discovery.Manager, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}

	mgr := discovery.NewManager(cfg)
	ctx := context.Background()
	discoveryStart := time.Now()

	mgr.Start(ctx)

	discoveryDuration := time.Since(discoveryStart)

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
	productInfo := buildProductStats(productsByProvider)

	ctxLog := logger.NewContextLogger("Discovery", "resource_type", "Manager")
	ctxLog.Infof("发现服务启动完成，总耗时: %v，发现产品数量: %d%s，版本=%d",
		discoveryDuration, discoveredTotalProducts, productInfo, mgr.Version())

	// 如果配置中没有产品，使用发现的产品
	if len(cfg.ProductsByProvider) == 0 && len(prods) > 0 {
		cfg.ProductsByProvider = prods
	}

	return mgr, nil
}

// buildProductStats 构建产品统计信息字符串
func buildProductStats(productsByProvider map[string]int) string {
	if len(productsByProvider) == 0 {
		return ""
	}

	var info strings.Builder
	info.WriteString(" (")
	providers := make([]string, 0, len(productsByProvider))
	for provider := range productsByProvider {
		providers = append(providers, provider)
	}
	sort.Strings(providers)
	for i, provider := range providers {
		if i > 0 {
			info.WriteString(", ")
		}
		info.WriteString(fmt.Sprintf("%s=%d", provider, productsByProvider[provider]))
	}
	info.WriteString(")")
	return info.String()
}

// getScrapeInterval 获取采集间隔（优先级：环境变量 > 配置文件 > 默认值）
func getScrapeInterval(cfg *config.Config) time.Duration {
	interval := 60 * time.Second

	// 1. 优先从配置文件读取
	if server := cfg.GetServer(); server != nil && server.ScrapeInterval != "" {
		if d, err := time.ParseDuration(server.ScrapeInterval); err == nil {
			interval = d
		} else {
			ctxLog := logger.NewContextLogger("Collection", "resource_type", "Config")
			ctxLog.Warnf("警告: 配置中的 scrape_interval 无效: %v", err)
		}
	}

	// 2. 环境变量覆盖
	if envInterval := getEnv("SCRAPE_INTERVAL"); envInterval != "" {
		if i, err := parseIntervalSeconds(envInterval); err == nil {
			interval = i
		} else if d, err := time.ParseDuration(envInterval); err == nil {
			interval = d
		} else {
			ctxLog := logger.NewContextLogger("Collection", "resource_type", "Config")
			ctxLog.Warnf("警告: 环境变量 SCRAPE_INTERVAL 无效: %v", err)
		}
	}

	return interval
}

// getServerPort 获取服务端口（优先级：环境变量 > 配置文件 > 默认值）
func getServerPort(cfg *config.Config) string {
	port := getEnv("EXPORTER_PORT")
	if port != "" {
		return port
	}

	if server := cfg.GetServer(); server != nil && server.Port > 0 {
		return fmt.Sprintf("%d", server.Port)
	}

	return "9101"
}

// parseIntervalSeconds 解析秒数格式的间隔
func parseIntervalSeconds(s string) (time.Duration, error) {
	var seconds int
	if _, err := fmt.Sscanf(s, "%d", &seconds); err != nil {
		return 0, err
	}
	return time.Duration(seconds) * time.Second, nil
}

// ========== 智能首次采集策略相关函数 ==========

// calculateFirstRunDelay 计算首次采集延迟时间
//
// 策略说明：
//   - 策略1（immediate）：立即采集，单/多Pod都无延迟
//   - 策略2（staggered）：强制错峰，线性分配延迟
//   - 策略3（auto，默认）：自适应判断
//   - 单/双Pod：立即采集
//   - 3-10个Pod：线性延迟 + 随机抖动
//   - >10个Pod：指数退避延迟，避免大规模并发
//
// 参数：
//   - totalShards: 集群总分片数（Pod总数）
//   - shardIndex: 当前Pod的索引
//   - interval: 采集间隔
//
// 返回：
//   - 首次采集延迟时间（0表示立即采集）
func calculateFirstRunDelay(totalShards, shardIndex int, interval time.Duration) time.Duration {
	// 从环境变量读取策略
	strategy := getEnvOrDefault("FIRST_RUN_STRATEGY", "auto")
	maxDelaySeconds := getEnvIntOrDefault("FIRST_RUN_MAX_DELAY", 180) // 默认最大180秒
	maxDelay := time.Duration(maxDelaySeconds) * time.Second

	switch strategy {
	case "immediate":
		// 策略1：立即采集（单/多Pod都立即）
		return 0

	case "staggered":
		// 策略2：强制错峰（单/多Pod都延迟）
		return calculateStaggeredDelay(totalShards, shardIndex, maxDelay)

	case "auto":
		fallthrough
	default:
		// 策略3：自动判断（推荐）
		return calculateAutoDelay(totalShards, shardIndex, interval, maxDelay)
	}
}

// calculateAutoDelay 自动判断延迟策略
func calculateAutoDelay(totalShards, shardIndex int, interval, maxDelay time.Duration) time.Duration {
	// 场景1：单Pod或双Pod
	if totalShards <= 2 {
		// Pod数量少，立即采集即可
		return 0
	}

	// 场景2：中等规模（3-10个Pod）
	if totalShards <= 10 {
		// 基础延迟5s + 索引*3s + 随机0-2s
		baseDelay := 5 * time.Second
		indexDelay := time.Duration(shardIndex) * 3 * time.Second
		randomDelay := time.Duration(rand.Intn(3)) * time.Second

		totalDelay := baseDelay + indexDelay + randomDelay

		// 不超过最大延迟
		if totalDelay > maxDelay {
			totalDelay = maxDelay
		}

		return totalDelay
	}

	// 场景3：大规模（>10个Pod）- 指数退避策略
	// 使用指数级增长，避免线性延迟导致最后一个Pod等待过久
	// 公式：base * (1.5 ^ index)
	//
	// 示例（20个Pod，base=5s）：
	//   Pod-0:  5s
	//   Pod-1:  7.5s
	//   Pod-5:  38s
	//   Pod-10: 86s
	//   Pod-15: 120s (封顶)
	//   Pod-19: 120s (封顶)

	base := 5 * time.Second
	multiplier := 1.5
	indexDelay := time.Duration(float64(base) * pow(multiplier, shardIndex))
	randomDelay := time.Duration(rand.Intn(5)) * time.Second

	totalDelay := indexDelay + randomDelay

	// 不超过最大延迟
	if totalDelay > maxDelay {
		totalDelay = maxDelay
	}

	// 大规模场景警告
	if totalShards > 10 {
		ctxLog := logger.NewContextLogger("Collection", "resource_type", "Scale")
		ctxLog.Warnf("大规模部署检测: %d个Pod，建议监控云API限流情况", totalShards)
	}

	return totalDelay
}

// calculateStaggeredDelay 计算强制错峰延迟（线性分布）
func calculateStaggeredDelay(totalShards, shardIndex int, maxDelay time.Duration) time.Duration {
	if totalShards <= 1 {
		return 0
	}

	// 线性分布：将maxDelay均匀分配给所有Pod
	delayPerShard := maxDelay / time.Duration(totalShards)
	return time.Duration(shardIndex) * delayPerShard
}

// pow 计算指数（用于大规模场景的指数退避）
func pow(base float64, exp int) float64 {
	result := 1.0
	for i := 0; i < exp; i++ {
		result *= base
	}
	return result
}

// getEnvOrDefault 获取环境变量或返回默认值
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvIntOrDefault 获取环境变量整数值或返回默认值
func getEnvIntOrDefault(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}
