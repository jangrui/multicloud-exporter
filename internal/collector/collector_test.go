package collector

import (
	"multicloud-exporter/internal/utils"
	"testing"
)

func TestShardOfDeterminism(t *testing.T) {
	if utils.ShardIndex("x", 1) != 0 {
		t.Fatalf("n1")
	}
	a := utils.ShardIndex("key", 8)
	b := utils.ShardIndex("key", 8)
	if a != b {
		t.Fatalf("stable")
	}
}

func TestAssignAccount(t *testing.T) {
	total := 5
	count := 0
	for i := 0; i < total; i++ {
		key := "p" + "|" + "id"
		if utils.ShouldProcess(key, total, i) {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("unique")
	}
}
