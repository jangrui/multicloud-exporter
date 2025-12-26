package main

import (
	"path/filepath"
	"testing"
)

func TestSplitCSV(t *testing.T) {
	input := "aws, tencent ,aliyun,,"
	got := splitCSV(input)
	want := []string{"aws", "tencent", "aliyun"}
	if len(got) != len(want) {
		t.Fatalf("len mismatch: got=%d want=%d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("item %d mismatch: got=%q want=%q", i, got[i], want[i])
		}
	}
}

func TestLoadProductMap(t *testing.T) {
	pm, err := loadProductMap(filepath.Join("..", "..", "configs", "product-map.yaml"))
	if err != nil {
		t.Fatalf("loadProductMap error: %v", err)
	}
	if pm == nil || pm.Products == nil {
		t.Fatalf("product map empty")
	}
	if pm.Products["s3"]["aws"] != "s3" {
		t.Fatalf("unexpected mapping for s3->aws: %q", pm.Products["s3"]["aws"])
	}
}

func TestCheckMappingAgainstProducts_AWS_S3(t *testing.T) {
	mapping := filepath.Join("..", "..", "configs", "mappings", "s3.metrics.yaml")
	productsRoot := filepath.Join("..", "..", "local", "configs", "products")
	pm, err := loadProductMap(filepath.Join("..", "..", "configs", "product-map.yaml"))
	if err != nil {
		t.Fatalf("loadProductMap error: %v", err)
	}
	if e := checkMappingAgainstProducts(mapping, productsRoot, "aws", pm, false); e != nil {
		t.Fatalf("checkMappingAgainstProducts failed: %v", e)
	}
}
