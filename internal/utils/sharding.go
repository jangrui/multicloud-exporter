package utils

import (
	"bufio"
	"hash/fnv"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
)

// lookupIPFunc is used for mocking net.LookupIP in tests
var lookupIPFunc = net.LookupIP

// ClusterConfig returns the total number of workers and the current worker's index.
// It supports discovery via Headless Service (DNS), File, or Static env vars.
func ClusterConfig() (int, int) {
	// Priority 1: Headless Service (Dynamic)
	if os.Getenv("CLUSTER_DISCOVERY") == "headless" {
		svc := os.Getenv("CLUSTER_SVC")
		selfIP := os.Getenv("POD_IP")
		if svc != "" && selfIP != "" {
			if ips, err := lookupIPFunc(svc); err == nil && len(ips) > 0 {
				var list []string
				for _, ip := range ips {
					list = append(list, ip.String())
				}
				sort.Strings(list)
				for i, ip := range list {
					if ip == selfIP {
						return len(list), i
					}
				}
			}
		}
	}

	// Priority 2: File Member Discovery
	if os.Getenv("CLUSTER_DISCOVERY") == "file" {
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
			}
		}
	}

	// Priority 3: Static Configuration
	total := 1
	index := 0

	// Support both CLUSTER_WORKERS and EXPORT_SHARD_TOTAL
	totalEnv := os.Getenv("CLUSTER_WORKERS")
	if totalEnv == "" {
		totalEnv = os.Getenv("EXPORT_SHARD_TOTAL")
	}
	if totalEnv != "" {
		if n, err := strconv.Atoi(totalEnv); err == nil && n > 0 {
			total = n
		}
	}

	// Support both CLUSTER_INDEX and EXPORT_SHARD_INDEX
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

// ShardIndex calculates the shard index for a given string key.
func ShardIndex(s string, n int) int {
	if n <= 1 {
		return 0
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	return int(h.Sum32() % uint32(n))
}

// ShouldProcess checks if the current worker (index) should process the given key.
func ShouldProcess(key string, total, index int) bool {
	if total <= 1 {
		return true
	}
	return ShardIndex(key, total) == index
}
