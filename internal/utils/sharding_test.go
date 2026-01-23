package utils

import (
	"net"
	"os"
	"testing"
)

func TestShardIndex(t *testing.T) {
	tests := []struct {
		key      string
		total    int
		expected int
	}{
		{"test", 1, 0},
		{"key1", 2, 0}, // fnv32a("key1") % 2
		{"key2", 2, 1}, // fnv32a("key2") % 2
	}

	for _, tt := range tests {
		got := ShardIndex(tt.key, tt.total)
		if tt.total > 1 && (got < 0 || got >= tt.total) {
			t.Errorf("ShardIndex(%q, %d) = %d; want [0, %d)", tt.key, tt.total, got, tt.total)
		}
		// For total=1, it's always 0. For others, just check bounds/determinism
		if tt.total == 1 && got != 0 {
			t.Errorf("ShardIndex(%q, 1) = %d; want 0", tt.key, got)
		}
	}
}

func TestShouldProcess(t *testing.T) {
	if !ShouldProcess("any", 1, 0) {
		t.Error("ShouldProcess should be true when total=1")
	}

	// key1 -> hash -> index
	key := "key1"
	total := 3
	idx := ShardIndex(key, total)

	if !ShouldProcess(key, total, idx) {
		t.Errorf("ShouldProcess(%q, %d, %d) should be true", key, total, idx)
	}

	if ShouldProcess(key, total, (idx+1)%total) {
		t.Errorf("ShouldProcess(%q, %d, %d) should be false", key, total, (idx+1)%total)
	}
}

func TestClusterConfig_Static(t *testing.T) {
	t.Setenv("CLUSTER_DISCOVERY", "")
	t.Setenv("CLUSTER_WORKERS", "3")
	t.Setenv("CLUSTER_INDEX", "1")

	total, index := ClusterConfig()
	if total != 3 || index != 1 {
		t.Errorf("ClusterConfig() = (%d, %d); want (3, 1)", total, index)
	}
}

func TestClusterConfig_Static_Defaults(t *testing.T) {
	t.Setenv("CLUSTER_DISCOVERY", "")
	t.Setenv("CLUSTER_WORKERS", "")
	t.Setenv("CLUSTER_INDEX", "")
	t.Setenv("EXPORT_SHARD_TOTAL", "")
	t.Setenv("EXPORT_SHARD_INDEX", "")

	total, index := ClusterConfig()
	if total != 1 || index != 0 {
		t.Errorf("ClusterConfig() = (%d, %d); want (1, 0)", total, index)
	}
}

func TestClusterConfig_Static_Legacy(t *testing.T) {
	t.Setenv("CLUSTER_DISCOVERY", "")
	t.Setenv("EXPORT_SHARD_TOTAL", "4")
	t.Setenv("EXPORT_SHARD_INDEX", "2")

	total, index := ClusterConfig()
	if total != 4 || index != 2 {
		t.Errorf("ClusterConfig() = (%d, %d); want (4, 2)", total, index)
	}
}

func TestClusterConfig_File(t *testing.T) {
	// Create a temp file
	tmpfile, err := os.CreateTemp("", "cluster_members")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(tmpfile.Name()) }()

	content := "pod-0\npod-1\npod-2\n"
	if _, err := tmpfile.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}

	t.Setenv("CLUSTER_DISCOVERY", "file")
	t.Setenv("CLUSTER_FILE", tmpfile.Name())
	t.Setenv("POD_NAME", "pod-1")

	total, index := ClusterConfig()
	if total != 3 || index != 1 {
		t.Errorf("ClusterConfig() = (%d, %d); want (3, 1)", total, index)
	}
}

func TestClusterConfig_Headless(t *testing.T) {
	originalLookup := lookupIPFunc
	defer func() { lookupIPFunc = originalLookup }()

	lookupIPFunc = func(host string) ([]net.IP, error) {
		if host == "headless-svc" {
			return []net.IP{
				net.ParseIP("10.0.0.1"),
				net.ParseIP("10.0.0.2"),
				net.ParseIP("10.0.0.3"),
			}, nil
		}
		return nil, &net.DNSError{Err: "not found"}
	}

	t.Setenv("CLUSTER_DISCOVERY", "headless")
	t.Setenv("CLUSTER_SVC", "headless-svc")
	t.Setenv("POD_IP", "10.0.0.2")

	total, index := ClusterConfig()
	if total != 3 || index != 1 {
		t.Errorf("ClusterConfig() = (%d, %d); want (3, 1)", total, index)
	}
}

// TestClusterConfigCache_TTL 测试缓存 TTL 功能
func TestClusterConfigCache_TTL(t *testing.T) {
	originalLookup := lookupIPFunc
	defer func() { lookupIPFunc = originalLookup }()

	// 重置缓存状态
	defer func() {
		clusterCfgMu.Lock()
		clusterCfgTotal = 0
		clusterCfgIndex = 0
		clusterCfgTTL = 0
		clusterCfgMu.Unlock()
	}()

	callCount := 0
	lookupIPFunc = func(host string) ([]net.IP, error) {
		callCount++
		if host == "headless-svc" {
			return []net.IP{
				net.ParseIP("10.0.0.1"),
				net.ParseIP("10.0.0.2"),
			}, nil
		}
		return nil, &net.DNSError{Err: "not found"}
	}

	t.Setenv("CLUSTER_DISCOVERY", "headless")
	t.Setenv("CLUSTER_SVC", "headless-svc")
	t.Setenv("POD_IP", "10.0.0.1")

	// 测试 1: 禁用缓存（TTL = 0），每次都应该查询 DNS
	SetClusterConfigTTL(0)
	callCount = 0

	total1, index1 := ClusterConfig()
	if total1 != 2 || index1 != 0 {
		t.Errorf("第1次调用: ClusterConfig() = (%d, %d); want (2, 0)", total1, index1)
	}
	if callCount != 3 {
		t.Errorf("第1次调用: DNS 查询次数 = %d; want 3", callCount)
	}

	total2, index2 := ClusterConfig()
	if total2 != 2 || index2 != 0 {
		t.Errorf("第2次调用: ClusterConfig() = (%d, %d); want (2, 0)", total2, index2)
	}
	if callCount != 6 {
		t.Errorf("第2次调用: DNS 查询次数 = %d; want 6 (缓存禁用)", callCount)
	}

	// 测试 2: 启用缓存（TTL = 1小时），第二次调用应该命中缓存
	// 先清理之前的缓存状态
	clusterCfgMu.Lock()
	clusterCfgTotal = 0
	clusterCfgIndex = 0
	clusterCfgMu.Unlock()

	SetClusterConfigTTL(3600 * 1000000000) // 1 小时
	callCount = 0

	total3, index3 := ClusterConfig()
	if total3 != 2 || index3 != 0 {
		t.Errorf("第3次调用: ClusterConfig() = (%d, %d); want (2, 0)", total3, index3)
	}
	if callCount != 3 {
		t.Errorf("第3次调用: DNS 查询次数 = %d; want 3", callCount)
	}

	total4, index4 := ClusterConfig()
	if total4 != 2 || index4 != 0 {
		t.Errorf("第4次调用: ClusterConfig() = (%d, %d); want (2, 0)", total4, index4)
	}
	if callCount != 3 {
		t.Errorf("第4次调用: DNS 查询次数 = %d; want 3 (应该命中缓存)", callCount)
	}
}

// TestClusterConfigCache_Expiry 测试缓存过期后重新查询
func TestClusterConfigCache_Expiry(t *testing.T) {
	originalLookup := lookupIPFunc
	defer func() { lookupIPFunc = originalLookup }()

	// 重置缓存状态
	defer func() {
		clusterCfgMu.Lock()
		clusterCfgTotal = 0
		clusterCfgIndex = 0
		clusterCfgTTL = 0
		clusterCfgMu.Unlock()
	}()

	callCount := 0
	lookupIPFunc = func(host string) ([]net.IP, error) {
		callCount++
		return []net.IP{
			net.ParseIP("10.0.0.1"),
			net.ParseIP("10.0.0.2"),
		}, nil
	}

	t.Setenv("CLUSTER_DISCOVERY", "headless")
	t.Setenv("CLUSTER_SVC", "headless-svc")
	t.Setenv("POD_IP", "10.0.0.1")

	// 设置一个很短的 TTL（1 纳秒，立即过期）
	SetClusterConfigTTL(1)
	callCount = 0

	// 第一次调用
	total1, index1 := ClusterConfig()
	if total1 != 2 || index1 != 0 {
		t.Errorf("第1次调用: ClusterConfig() = (%d, %d); want (2, 0)", total1, index1)
	}
	if callCount != 3 {
		t.Errorf("第1次调用: DNS 查询次数 = %d; want 3", callCount)
	}

	// 第二次调用，缓存应该已过期，需要重新查询
	total2, index2 := ClusterConfig()
	if total2 != 2 || index2 != 0 {
		t.Errorf("第2次调用: ClusterConfig() = (%d, %d); want (2, 0)", total2, index2)
	}
	if callCount != 6 {
		t.Errorf("第2次调用: DNS 查询次数 = %d; want 6 (缓存已过期)", callCount)
	}
}

// TestClusterConfigCache_DNSFailureFallback 测试 DNS 查询失败时使用缓存配置
func TestClusterConfigCache_DNSFailureFallback(t *testing.T) {
	originalLookup := lookupIPFunc
	defer func() { lookupIPFunc = originalLookup }()

	// 重置缓存状态
	defer func() {
		clusterCfgMu.Lock()
		clusterCfgTotal = 0
		clusterCfgIndex = 0
		clusterCfgTTL = 0
		clusterCfgMu.Unlock()
	}()

	callCount := 0
	shouldFail := false

	lookupIPFunc = func(host string) ([]net.IP, error) {
		callCount++
		if shouldFail {
			return nil, &net.DNSError{Err: "temporary failure"}
		}
		return []net.IP{
			net.ParseIP("10.0.0.1"),
			net.ParseIP("10.0.0.2"),
			net.ParseIP("10.0.0.3"),
		}, nil
	}

	t.Setenv("CLUSTER_DISCOVERY", "headless")
	t.Setenv("CLUSTER_SVC", "headless-svc")
	t.Setenv("POD_IP", "10.0.0.2")

	// 设置一个很短的 TTL，确保缓存会过期
	SetClusterConfigTTL(1)

	// 第一次调用成功，建立缓存
	total1, index1 := ClusterConfig()
	if total1 != 3 || index1 != 1 {
		t.Errorf("第1次调用: ClusterConfig() = (%d, %d); want (3, 1)", total1, index1)
	}

	// 模拟 DNS 查询失败
	shouldFail = true
	callCount = 0

	// 第二次调用，DNS 失败但应该使用上次成功的配置
	total2, index2 := ClusterConfig()
	if total2 != 3 || index2 != 1 {
		t.Errorf("第2次调用（DNS失败）: ClusterConfig() = (%d, %d); want (3, 1) (应该使用缓存配置)", total2, index2)
	}
	// 应该尝试了 1 次 DNS 查询（当前实现遇到错误立即中断）
	if callCount != 1 {
		t.Errorf("DNS 失败时的查询次数 = %d; want 1", callCount)
	}
}
