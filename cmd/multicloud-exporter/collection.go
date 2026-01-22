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
//   - auto（默认）：等待集群稳定后，根据 Pod 数量自动选择延迟策略
//   - immediate：跳过稳定性检测，立即采集
//   - staggered：等待集群稳定后，使用线性延迟分布
//
// 环境变量控制：
//   - FIRST_RUN_STRATEGY: auto（自动）| immediate（立即）| staggered（强制错峰）
//   - FIRST_RUN_MAX_DELAY: 最大延迟秒数（默认180秒）
//   - CLUSTER_STABILITY_CHECK_ENABLED: 是否启用集群稳定性检测（默认 true）
//   - CLUSTER_STABILITY_MAX_WAIT: 稳定性检测最长等待时间（默认 30s）
//   - CLUSTER_STABILITY_CHECK_INTERVAL: 稳定性检测间隔（默认 2s）
//   - CLUSTER_STABILITY_REQUIRED_STABLE: 需要连续稳定的次数（默认 3）
func startCollectionLoop(ctx context.Context, cfg *config.Config, coll *collector.FourDimensionCollector, mgr *discovery.Manager, interval time.Duration) {
	go func() {
		lastVer := int64(-1)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		// ========== 智能首次采集策略 ==========
		// 计算首次采集延迟（集成集群稳定性检测）
		firstRunDelay := calculateFirstRunDelay(interval)

		if firstRunDelay > 0 {
			ctxLog := logger.NewContextLogger("Collection", "resource_type", "FirstRun")
			ctxLog.Infof("首次采集延迟: %v，等待后开始采集...", firstRunDelay)

			select {
			case <-time.After(firstRunDelay):
			case <-ctx.Done():
				ctxLog := logger.NewContextLogger("Collection", "resource_type", "FirstRun")
				ctxLog.Info("收到停止信号，取消首次采集")
				return
			}
		}

		// 执行首次采集
		ctxLog := logger.NewContextLogger("Collection", "resource_type", "FirstRun")
		ctxLog.Info("开始首次采集...")

		// 记录分片配置信息和指标
		total, index := utils.ClusterConfig()
		ctxLog.Infof("分片配置: total=%d, index=%d", total, index)

		// 记录集群配置指标
		metrics.ClusterConfigTotal.Set(float64(total))
		metrics.ClusterConfigIndex.Set(float64(index))

		// 记录首次采集延迟指标
		if firstRunDelay > 0 {
			strategy := getEnvOrDefault("FIRST_RUN_STRATEGY", "auto")
			metrics.FirstRunDelaySeconds.WithLabelValues(
				fmt.Sprintf("%d", index),
				strategy,
			).Set(firstRunDelay.Seconds())
		}

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

				// 记录当前分片配置和指标
				total, index := utils.ClusterConfig()
				collectionLog.Infof("当前分片配置: total=%d, index=%d", total, index)

				// 更新集群配置指标
				metrics.ClusterConfigTotal.Set(float64(total))
				metrics.ClusterConfigIndex.Set(float64(index))

				// 检查配置版本是否变化
				versionChanged := false
				if v := mgr.Version(); v != lastVer {
					cfg.ProductsByProvider = mgr.Get()
					lastVer = v
					versionChanged = true
					collectionLog.Infof("配置版本变化: %d -> %d", lastVer, v)
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
		fmt.Fprintf(&info, "%s=%d", provider, productsByProvider[provider])
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

// calculateFirstRunDelay 计算首次采集延迟时间（集成集群稳定性检测）
//
// 策略说明：
//   - immediate：跳过稳定性检测，立即采集
//   - staggered：等待集群稳定后，使用线性延迟分布
//   - auto（默认）：等待集群稳定后，根据 Pod 数量自动选择延迟策略
//   - 单 Pod：立即采集
//   - 2-10 个 Pod：线性延迟 + 随机抖动
//   - >10 个 Pod：指数退避延迟
//
// 参数：
//   - interval: 采集间隔（用于设置集群配置缓存 TTL）
//
// 返回：
//   - 首次采集延迟时间（0 表示立即采集）
func calculateFirstRunDelay(interval time.Duration) time.Duration {
	ctxLog := logger.NewContextLogger("Collection", "resource_type", "FirstRunStrategy")

	// 从环境变量读取策略配置
	strategy := getEnvOrDefault("FIRST_RUN_STRATEGY", "auto")
	maxDelaySeconds := getEnvIntOrDefault("FIRST_RUN_MAX_DELAY", 180) // 默认最大 180 秒
	maxDelay := time.Duration(maxDelaySeconds) * time.Second

	// 设置集群配置缓存 TTL（使用采集间隔）
	utils.SetClusterConfigTTL(interval)

	ctxLog.Infof("首次采集策略: %s, 最大延迟: %v", strategy, maxDelay)

	// 策略 1: immediate - 跳过稳定性检测，立即采集
	if strategy == "immediate" {
		ctxLog.Info("策略选择: immediate - 跳过集群稳定性检测，立即开始采集")
		return 0
	}

	// 策略 2 和 3: 需要等待集群稳定
	// 读取稳定性检测配置
	stabilityCheckEnabled := getEnvBoolOrDefault("CLUSTER_STABILITY_CHECK_ENABLED", true)
	maxWaitSeconds := getEnvIntOrDefault("CLUSTER_STABILITY_MAX_WAIT", 30)
	checkIntervalSeconds := getEnvIntOrDefault("CLUSTER_STABILITY_CHECK_INTERVAL", 2)
	requiredStable := getEnvIntOrDefault("CLUSTER_STABILITY_REQUIRED_STABLE", 3)

	maxWait := time.Duration(maxWaitSeconds) * time.Second
	checkInterval := time.Duration(checkIntervalSeconds) * time.Second

	var totalShards, shardIndex int
	var stable bool

	if stabilityCheckEnabled {
		ctxLog.Infof("开始集群稳定性检测: 最长等待=%v, 检查间隔=%v, 需要连续稳定=%d次",
			maxWait, checkInterval, requiredStable)

		// 记录稳定性检测开始时间
		stabilityCheckStart := time.Now()

		// 等待集群稳定
		totalShards, shardIndex, stable = utils.WaitForStableCluster(maxWait, checkInterval, requiredStable)

		// 计算稳定性检测耗时
		stabilityCheckDuration := time.Since(stabilityCheckStart)

		if stable {
			ctxLog.Infof("集群稳定性检测完成: 集群已稳定, total=%d, index=%d, 耗时=%v",
				totalShards, shardIndex, stabilityCheckDuration)
		} else {
			ctxLog.Warnf("集群稳定性检测超时: 使用当前配置, total=%d, index=%d, 耗时=%v",
				totalShards, shardIndex, stabilityCheckDuration)
		}
	} else {
		ctxLog.Info("集群稳定性检测已禁用，直接获取集群配置")
		totalShards, shardIndex = utils.ClusterConfig()
		ctxLog.Infof("集群配置: total=%d, index=%d", totalShards, shardIndex)
	}

	// 策略 2: staggered - 强制线性错峰
	if strategy == "staggered" {
		delay := calculateStaggeredDelay(totalShards, shardIndex, maxDelay)
		ctxLog.Infof("策略选择: staggered - 线性错峰延迟, total=%d, index=%d, delay=%v",
			totalShards, shardIndex, delay)
		return delay
	}

	// 策略 3: auto - 自动判断（默认）
	delay := calculateAutoDelay(totalShards, shardIndex, maxDelay)
	ctxLog.Infof("策略选择: auto - 自动判断延迟, total=%d, index=%d, delay=%v",
		totalShards, shardIndex, delay)
	return delay
}

// calculateAutoDelay 自动判断延迟策略（根据 Pod 数量）
func calculateAutoDelay(totalShards, shardIndex int, maxDelay time.Duration) time.Duration {
	ctxLog := logger.NewContextLogger("Collection", "resource_type", "AutoDelay")

	// 场景 1：单 Pod
	if totalShards == 1 {
		ctxLog.Info("单 Pod 场景，立即采集")
		return 0
	}

	// 场景 2：中等规模（2-10 个 Pod）
	if totalShards <= 10 {
		// 基础延迟 5s + 索引*3s + 随机 0-2s
		baseDelay := 5 * time.Second
		indexDelay := time.Duration(shardIndex) * 3 * time.Second
		randomDelay := time.Duration(rand.Intn(3)) * time.Second

		totalDelay := baseDelay + indexDelay + randomDelay

		// 不超过最大延迟
		if totalDelay > maxDelay {
			totalDelay = maxDelay
		}

		ctxLog.Infof("中等规模场景(%d个Pod): 线性延迟策略, base=%v, index_delay=%v, random=%v, total=%v",
			totalShards, baseDelay, indexDelay, randomDelay, totalDelay)

		return totalDelay
	}

	// 场景 3：大规模（>10 个 Pod）- 指数退避策略
	// 使用指数级增长，避免线性延迟导致最后一个 Pod 等待过久
	// 公式：base * (1.5 ^ index) + random
	base := 5 * time.Second
	multiplier := 1.5
	indexDelay := time.Duration(float64(base) * pow(multiplier, shardIndex))
	randomDelay := time.Duration(rand.Intn(5)) * time.Second

	totalDelay := indexDelay + randomDelay

	// 不超过最大延迟
	if totalDelay > maxDelay {
		totalDelay = maxDelay
	}

	ctxLog.Warnf("大规模场景(%d个Pod): 指数退避策略, index_delay=%v, random=%v, total=%v, 建议监控云API限流情况",
		totalShards, indexDelay, randomDelay, totalDelay)

	return totalDelay
}

// calculateStaggeredDelay 计算强制错峰延迟（线性分布）
func calculateStaggeredDelay(totalShards, shardIndex int, maxDelay time.Duration) time.Duration {
	if totalShards <= 1 {
		return 0
	}

	// 线性分布：将 maxDelay 均匀分配给所有 Pod
	delayPerShard := maxDelay / time.Duration(totalShards)
	return time.Duration(shardIndex) * delayPerShard
}

// pow 计算指数（用于大规模场景的指数退避）
func pow(base float64, exp int) float64 {
	result := 1.0
	for range exp {
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

// getEnvBoolOrDefault 获取环境变量布尔值或返回默认值
func getEnvBoolOrDefault(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolVal, err := strconv.ParseBool(value); err == nil {
			return boolVal
		}
	}
	return defaultValue
}
