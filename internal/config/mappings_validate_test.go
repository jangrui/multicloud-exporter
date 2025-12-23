package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateAllMappings_OK(t *testing.T) {
	dir := filepath.Join("..", "..", "configs", "mappings")
	if err := ValidateAllMappings(dir); err != nil {
		t.Fatalf("ValidateAllMappings error: %v", err)
	}
}

func TestValidateMappingStructure_BadTopLevel(t *testing.T) {
	tmp := t.TempDir()
	p := filepath.Join(tmp, "bad.yaml")
	content := "unexpected_key: 1\n"
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatalf("write temp: %v", err)
	}
	if err := ValidateMappingStructure(p); err == nil {
		t.Fatalf("expected error for bad top-level key")
	}
}
