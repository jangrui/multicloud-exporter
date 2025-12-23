package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig_ServerAndAccounts(t *testing.T) {
	dir := t.TempDir()
	serverPath := filepath.Join(dir, "server.yaml")
	accountsPath := filepath.Join(dir, "accounts.yaml")

	// server.yaml with env expansion and partial ResourceDimMapping to test merge
	serverYAML := `
server:
  port: ${PORT:-8080}
  resource_dim_mapping:
    aliyun.acs_oss_dashboard:
      - BucketName
`
	if err := os.WriteFile(serverPath, []byte(serverYAML), 0644); err != nil {
		t.Fatalf("write server.yaml: %v", err)
	}
	accountsYAML := `
accounts:
  aws:
    - provider: aws
      account_id: "acc-1"
      access_key_id: "ak"
      access_key_secret: "sk"
      resources: ["s3"]
`
	if err := os.WriteFile(accountsPath, []byte(accountsYAML), 0644); err != nil {
		t.Fatalf("write accounts.yaml: %v", err)
	}

	_ = os.Setenv("SERVER_PATH", serverPath)
	defer func() { _ = os.Unsetenv("SERVER_PATH") }()
	_ = os.Setenv("ACCOUNTS_PATH", accountsPath)
	defer func() { _ = os.Unsetenv("ACCOUNTS_PATH") }()
	_ = os.Setenv("PORT", "9090")
	defer func() { _ = os.Unsetenv("PORT") }()

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	if cfg.Server == nil {
		t.Fatalf("Server should not be nil")
	}
	if cfg.Server.Port != 9090 {
		t.Fatalf("port env expansion failed, got=%d", cfg.Server.Port)
	}
	// Ensure merge brings defaults for missing keys while keeping provided override
	if cfg.Server.ResourceDimMapping == nil {
		t.Fatalf("ResourceDimMapping should not be nil")
	}
	// user provided key exists
	if _, ok := cfg.Server.ResourceDimMapping["aliyun.acs_oss_dashboard"]; !ok {
		t.Fatalf("missing user mapping aliyun.acs_oss_dashboard")
	}
	// default key merged
	if _, ok := cfg.Server.ResourceDimMapping["aliyun.acs_slb_dashboard"]; !ok {
		t.Fatalf("missing default mapping aliyun.acs_slb_dashboard")
	}
	// Accounts loaded
	if len(cfg.AccountsByProvider["aws"]) == 0 {
		t.Fatalf("accounts not loaded")
	}
}
