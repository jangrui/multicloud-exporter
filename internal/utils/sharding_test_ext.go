package utils

import (
	"testing"
	"time"
)

func TestShardIndex_Deterministic(t *testing.T) {
	key := "test-key"
	index1 := ShardIndex(key, 10)
	index2 := ShardIndex(key, 10)
	if index1 != index2 {
		t.Errorf("ShardIndex should be deterministic, got %d and %d", index1, index2)
	}
}

func TestShouldProcess_SingleShard(t *testing.T) {
	keys := []string{"a", "b", "c"}
	for _, key := range keys {
		if !ShouldProcess(key, 1, 0) {
			t.Errorf("Single shard mode should process all keys, key=%s", key)
		}
	}
}

func TestShouldProcess_MultipleShards(t *testing.T) {
	const totalShards = 3
	keyToShard := make(map[string]int)

	for i := 0; i < 30; i++ {
		key := "test-key-" + string(rune('a'+(i%26)))
		for shard := 0; shard < totalShards; shard++ {
			if ShouldProcess(key, totalShards, shard) {
				existingShard, exists := keyToShard[key]
				if exists {
					t.Errorf("Key %s already assigned to shard %d, now assigned to shard %d",
						key, existingShard, shard)
				}
				keyToShard[key] = shard
				break
			}
		}
	}

	if len(keyToShard) != 30 {
		t.Errorf("Expected all 30 keys to be assigned, got %d", len(keyToShard))
	}
}

func TestParseDuration_ValidFormats(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Duration
	}{
		{"60s", 60 * time.Second},
		{"1m", 1 * time.Minute},
		{"1h", 1 * time.Hour},
		{"1d", 24 * time.Hour},
		{"1d30m", 24*time.Hour + 30*time.Minute},
		{"1h30m", 90 * time.Minute},
	}

	for _, tt := range tests {
		result, err := ParseDuration(tt.input)
		if err != nil {
			t.Errorf("ParseDuration(%q) failed: %v", tt.input, err)
		}
		if result != tt.expected {
			t.Errorf("ParseDuration(%q) = %v, want %v", tt.input, result, tt.expected)
		}
	}
}

func TestParseDuration_InvalidFormat(t *testing.T) {
	_, err := ParseDuration("invalid")
	if err == nil {
		t.Error("ParseDuration should return error for invalid format")
	}
}

func TestClusterConfig_DefaultValues(t *testing.T) {
	total, index := ClusterConfig()
	if total < 1 {
		t.Errorf("Expected total >= 1, got %d", total)
	}
	if index < 0 || index >= total {
		t.Errorf("Expected index in range [0, %d), got %d", total-1, index)
	}
}

func TestSetClusterConfigTTL(t *testing.T) {
	SetClusterConfigTTL(1 * time.Second)

	total, index := ClusterConfig()
	if total < 1 {
		t.Errorf("Expected total >= 1, got %d", total)
	}
	if index < 0 || index >= total {
		t.Errorf("Expected index in range [0, %d], got %d", total-1, index)
	}
}
