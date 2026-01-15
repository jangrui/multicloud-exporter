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

		svc := os.Getenv("CLUSTER_SVC")
		selfIP := os.Getenv("POD_IP")
		if svc != "" && selfIP != "" {
			var ips []net.IP
			var err error
			for i := 0; i < 3; i++ {
				ips, err = lookupIPFunc(svc)
				if err == nil && len(ips) > 0 {
					break
				}
				if i < 2 {
					time.Sleep(100 * time.Millisecond)
				}
			}

			if err == nil && len(ips) > 0 {
				var list []string
				for _, ip := range ips {
					list = append(list, ip.String())
				}
				sort.Strings(list)

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
					setClusterConfig(len(list), idx)
					return len(list), idx
				} else {
					shardLog.With("service", svc, "self_ip", selfIP, "dns_list", list).Warn("未在 DNS 列表中找到 selfIP")
				}
			} else {
				if err != nil {
					shardLog.With("service", svc).Warnf("DNS 查询失败: %v", err)
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
