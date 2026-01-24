package utils

import (
	"bufio"
	"hash/fnv"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"multicloud-exporter/internal/logger"
)

var lookupIPFunc = net.LookupIP

var (
	clusterCfgMu        sync.RWMutex
	clusterCfgTotal     int
	clusterCfgIndex     int
	clusterCfgUpdatedAt time.Time
	clusterCfgTTL       time.Duration // 零值表示未启用缓存
)

// SetClusterConfigTTL 设置集群配置缓存 TTL
// 通常在启动时使用采集间隔作为参数调用
// ttl 为零或负数时禁用缓存（每次都重新获取）
func SetClusterConfigTTL(ttl time.Duration) {
	clusterCfgMu.Lock()
	defer clusterCfgMu.Unlock()
	clusterCfgTTL = ttl
}

func getCachedClusterConfig() (int, int, bool) {
	clusterCfgMu.RLock()
	defer clusterCfgMu.RUnlock()

	if clusterCfgTotal <= 0 {
		return 0, 0, false
	}
	if clusterCfgTTL <= 0 {
		return 0, 0, false
	}
	if time.Since(clusterCfgUpdatedAt) > clusterCfgTTL {
		return 0, 0, false
	}
	return clusterCfgTotal, clusterCfgIndex, true
}

func getLastClusterConfig() (int, int, bool) {
	clusterCfgMu.RLock()
	defer clusterCfgMu.RUnlock()

	if clusterCfgTotal <= 0 {
		return 0, 0, false
	}
	return clusterCfgTotal, clusterCfgIndex, true
}

func setClusterConfig(total, index int) {
	clusterCfgMu.Lock()
	defer clusterCfgMu.Unlock()

	if total <= 0 {
		return
	}
	if index < 0 {
		index = 0
	}
	if index >= total {
		index = index % total
	}
	clusterCfgTotal = total
	clusterCfgIndex = index
	clusterCfgUpdatedAt = time.Now()
}

func TryStableHeadlessClusterConfig(stabilityAttempts int, stabilityInterval time.Duration) (int, int, bool) {
	svc := os.Getenv("CLUSTER_SVC")
	selfIP := os.Getenv("POD_IP")
	if svc == "" || selfIP == "" {
		return 1, 0, true
	}

	var consistentIPs []string
	for i := 0; i < stabilityAttempts; i++ {
		ips, err := lookupIPFunc(svc)
		if err != nil || len(ips) == 0 {
			return 0, 0, false
		}

		currentList := make([]string, 0, len(ips))
		for _, ip := range ips {
			currentList = append(currentList, ip.String())
		}
		sort.Strings(currentList)

		if i == 0 {
			consistentIPs = currentList
		} else {
			if len(currentList) != len(consistentIPs) {
				return 0, 0, false
			}
			for j := range currentList {
				if currentList[j] != consistentIPs[j] {
					return 0, 0, false
				}
			}
		}

		if i < stabilityAttempts-1 {
			time.Sleep(stabilityInterval)
		}
	}

	index := -1
	for i, ip := range consistentIPs {
		if ip == selfIP {
			index = i
			break
		}
	}
	if index == -1 {
		return 0, 0, false
	}

	setClusterConfig(len(consistentIPs), index)
	return len(consistentIPs), index, true
}

// WaitForStableCluster 等待集群拓扑稳定
// 通过连续多次 DNS 查询，确保返回的 Pod 数量一致，避免滚动启动时的重复采集问题
// 参数:
//   - maxWait: 最长等待时间（如 30s）
//   - checkInterval: 检查间隔（如 2s）
//   - requiredStable: 需要连续稳定的次数（如 3）
//
// 返回:
//   - total: 集群总 Pod 数
//   - index: 当前 Pod 索引
//   - stable: 是否在超时前达到稳定状态
func WaitForStableCluster(maxWait, checkInterval time.Duration, requiredStable int) (int, int, bool) {
	svc := os.Getenv("CLUSTER_SVC")
	selfIP := os.Getenv("POD_IP")

	// 非集群模式，直接返回单实例配置
	if svc == "" || selfIP == "" {
		return 1, 0, true
	}

	ctxLog := logger.NewContextLogger("Cluster", "resource_type", "StabilityCheck")
	ctxLog.Infof("开始集群稳定性检测，最长等待 %v，检查间隔 %v，需要连续稳定 %d 次", maxWait, checkInterval, requiredStable)

	deadline := time.Now().Add(maxWait)
	var lastTotal int
	stableCount := 0
	checkCount := 0

	for time.Now().Before(deadline) {
		checkCount++

		// 执行 DNS 查询（带重试）
		var ips []net.IP
		var err error
		for i := range 3 {
			ips, err = lookupIPFunc(svc)
			if err == nil && len(ips) > 0 {
				break
			}
			if i < 2 {
				time.Sleep(100 * time.Millisecond)
			}
		}

		// DNS 查询失败，等待后继续
		if err != nil || len(ips) == 0 {
			if err != nil {
				ctxLog.Warnf("第 %d 次检查：DNS 查询失败: %v", checkCount, err)
			} else {
				ctxLog.Warnf("第 %d 次检查：DNS 查询返回空列表", checkCount)
			}
			time.Sleep(checkInterval)
			continue
		}

		currentTotal := len(ips)

		// 检查是否与上次结果一致
		if currentTotal == lastTotal && lastTotal > 0 {
			stableCount++
			ctxLog.Debugf("第 %d 次检查：Pod 数量稳定 (%d)，连续稳定次数 %d/%d", checkCount, currentTotal, stableCount, requiredStable)

			// 达到稳定要求
			if stableCount >= requiredStable {
				// 计算当前 Pod 的索引
				var list []string
				for _, ip := range ips {
					list = append(list, ip.String())
				}
				sort.Strings(list)

				index := -1
				for i, ip := range list {
					if ip == selfIP {
						index = i
						break
					}
				}

				if index == -1 {
					ctxLog.Warnf("集群已稳定但未在 DNS 列表中找到 selfIP: %s，列表: %v", selfIP, list)
					stableCount = 0
					time.Sleep(checkInterval)
					continue
				}

				ctxLog.Infof("集群已稳定：total=%d, index=%d, 检查次数=%d, 耗时=%v", currentTotal, index, checkCount, time.Since(deadline.Add(-maxWait)))
				setClusterConfig(currentTotal, index)
				return currentTotal, index, true
			}
		} else {
			// Pod 数量变化，重置稳定计数
			if lastTotal > 0 {
				ctxLog.Infof("第 %d 次检查：Pod 数量变化 %d -> %d，重置稳定计数", checkCount, lastTotal, currentTotal)
			} else {
				ctxLog.Debugf("第 %d 次检查：检测到 %d 个 Pod", checkCount, currentTotal)
			}
			stableCount = 1
			lastTotal = currentTotal
		}

		// 等待下一次检查
		time.Sleep(checkInterval)
	}

	// 超时，使用最后一次查询结果
	ctxLog.Warnf("集群稳定性检测超时（%v），使用当前配置: total=%d, 检查次数=%d", maxWait, lastTotal, checkCount)

	// 尝试获取最后一次的 Pod 列表和索引
	ips, err := lookupIPFunc(svc)
	if err != nil || len(ips) == 0 {
		// DNS 查询失败，尝试使用缓存配置
		if total, index, ok := getLastClusterConfig(); ok {
			ctxLog.Warnf("DNS 查询失败，使用缓存配置: total=%d, index=%d", total, index)
			return total, index, false
		}
		ctxLog.Warnf("DNS 查询失败且无缓存，无法确定分片配置")
		return 0, 0, false
	}

	var list []string
	for _, ip := range ips {
		list = append(list, ip.String())
	}
	sort.Strings(list)

	index := -1
	for i, ip := range list {
		if ip == selfIP {
			index = i
			break
		}
	}

	if index == -1 {
		if total, cachedIndex, ok := getLastClusterConfig(); ok {
			ctxLog.Warnf("未在 DNS 列表中找到 selfIP: %s，使用缓存配置: total=%d, index=%d", selfIP, total, cachedIndex)
			return total, cachedIndex, false
		}
		ctxLog.Warnf("未在 DNS 列表中找到 selfIP: %s，无法确定分片配置", selfIP)
		return 0, 0, false
	}

	setClusterConfig(len(list), index)
	return len(list), index, false
}

// ClusterConfig 获取当前实例在集群中的分片配置（总数和索引）
// 支持多种发现方式：
// 1. headless: 通过 Kubernetes Headless Service DNS 自动发现
// 2. file: 从文件读取成员列表
// 3. env: 使用环境变量 CLUSTER_WORKERS 和 CLUSTER_INDEX（或兼容的 EXPORT_SHARD_*）
func ClusterConfig() (int, int) {
	discoveryType := os.Getenv("CLUSTER_DISCOVERY")
	shardLog := logger.NewContextLogger("Sharding", "discovery", discoveryType)

	if discoveryType == "headless" {
		if total, index, ok := getCachedClusterConfig(); ok {
			return total, index
		}

		// 记录配置刷新开始
		refreshStart := time.Now()

		svc := os.Getenv("CLUSTER_SVC")
		selfIP := os.Getenv("POD_IP")
		if svc != "" && selfIP != "" {
			// 增强版：执行稳定性检测
			// 连续多次查询 DNS，只有结果一致时才认为集群稳定
			const stabilityAttempts = 3
			const stabilityInterval = 1 * time.Second

			var consistentIPs []string
			isStable := true
			var ips []net.IP
			var err error

			for i := 0; i < stabilityAttempts; i++ {
				ips, err = lookupIPFunc(svc)
				if err != nil {
					isStable = false
					break
				}
				if len(ips) == 0 {
					isStable = false
					break
				}

				var currentList []string
				for _, ip := range ips {
					currentList = append(currentList, ip.String())
				}
				sort.Strings(currentList)

				if i == 0 {
					consistentIPs = currentList
				} else {
					if len(currentList) != len(consistentIPs) {
						isStable = false
						shardLog.With("attempt", i, "prev_count", len(consistentIPs), "curr_count", len(currentList)).Warn("集群拓扑不稳定：Pod 数量变化")
						break
					}
					for j, ip := range currentList {
						if ip != consistentIPs[j] {
							isStable = false
							shardLog.With("attempt", i, "prev_ip", consistentIPs[j], "curr_ip", ip).Warn("集群拓扑不稳定：Pod IP 变化")
							break
						}
					}
				}

				if i < stabilityAttempts-1 {
					time.Sleep(stabilityInterval)
				}
			}

			if isStable && len(consistentIPs) > 0 {
				list := consistentIPs

				found := false
				var idx int
				for i, ip := range list {
					if ip == selfIP {
						idx = i
						found = true
						break
					}
				}
				if found {
					// 检查配置是否变化
					oldTotal, oldIndex, hadCache := getLastClusterConfig()
					newTotal := len(list)
					newIndex := idx

					// 记录配置刷新耗时
					refreshDuration := time.Since(refreshStart).Seconds()

					// 如果配置发生变化，记录详细日志
					if hadCache && (oldTotal != newTotal || oldIndex != newIndex) {
						shardLog.Infof("集群配置已刷新: 旧配置=(total=%d, index=%d), 新配置=(total=%d, index=%d), 耗时=%.3fs",
							oldTotal, oldIndex, newTotal, newIndex, refreshDuration)
					} else if !hadCache {
						shardLog.Infof("集群配置初始化: total=%d, index=%d, 发现方式=%s, 耗时=%.3fs",
							newTotal, newIndex, discoveryType, refreshDuration)
					}

					setClusterConfig(newTotal, newIndex)
					return newTotal, newIndex
				} else {
					shardLog.With("service", svc, "self_ip", selfIP, "dns_list", list).Warn("未在 DNS 列表中找到 selfIP")
				}
			} else {
				if err != nil {
					shardLog.With("service", svc).Warnf("DNS 查询失败: %v", err)
				} else if !isStable {
					shardLog.With("service", svc).Warn("集群拓扑不稳定（DNS 抖动），保持当前配置")
				} else {
					shardLog.With("service", svc).Warn("DNS 查询返回空列表")
				}
			}
		}

		if total, index, ok := getLastClusterConfig(); ok {
			return total, index
		}
	}

	if discoveryType == "file" {
		path := os.Getenv("CLUSTER_FILE")
		self := os.Getenv("POD_NAME")
		if self == "" {
			self = os.Getenv("HOSTNAME")
		}
		if path != "" && self != "" {
			if f, err := os.Open(path); err == nil {
				defer func() { _ = f.Close() }()
				var members []string
				sc := bufio.NewScanner(f)
				for sc.Scan() {
					line := strings.TrimSpace(sc.Text())
					if line != "" {
						members = append(members, line)
					}
				}
				if len(members) > 0 {
					sort.Strings(members)
					for i, m := range members {
						if m == self {
							return len(members), i
						}
					}
				}
			} else {
				shardLog.With("file", path, "self", self).Warnf("打开文件失败: %v", err)
			}
		}
	}

	total := 1
	index := 0

	totalEnv := os.Getenv("CLUSTER_WORKERS")
	if totalEnv == "" {
		totalEnv = os.Getenv("EXPORT_SHARD_TOTAL")
	}
	if totalEnv != "" {
		if n, err := strconv.Atoi(totalEnv); err == nil && n > 0 {
			total = n
		}
	}

	indexEnv := os.Getenv("CLUSTER_INDEX")
	if indexEnv == "" {
		indexEnv = os.Getenv("EXPORT_SHARD_INDEX")
	}
	if indexEnv != "" {
		if n, err := strconv.Atoi(indexEnv); err == nil && n >= 0 {
			index = n
		}
	}

	if index >= total {
		index = index % total
	}
	return total, index
}

// ShardIndex 根据字符串键计算分片索引
// 使用 FNV-1a 哈希算法确保相同的键始终映射到同一分片
func ShardIndex(s string, n int) int {
	if n <= 1 {
		return 0
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	return int(h.Sum32() % uint32(n))
}

// ShouldProcess 判断当前分片是否应该处理给定的键
// 用于实现分片采集逻辑，确保每个键只被一个分片处理
func ShouldProcess(key string, total, index int) bool {
	if total <= 1 {
		return true
	}
	return ShardIndex(key, total) == index
}
