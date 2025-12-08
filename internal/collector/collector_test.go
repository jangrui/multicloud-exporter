package collector

import "testing"

func TestShardOfDeterminism(t *testing.T) {
    if shardOf("x", 1) != 0 { t.Fatalf("n1") }
    a := shardOf("key", 8)
    b := shardOf("key", 8)
    if a != b { t.Fatalf("stable") }
}

func TestAssignAccount(t *testing.T) {
    total := 5
    count := 0
    for i := 0; i < total; i++ {
        if assignAccount("p", "id", total, i) { count++ }
    }
    if count != 1 { t.Fatalf("unique") }
}
