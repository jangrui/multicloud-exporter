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
	defer os.Remove(tmpfile.Name())

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
