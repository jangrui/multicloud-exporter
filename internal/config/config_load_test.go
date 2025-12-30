package config

import (
	"fmt"
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

// Benchmark tests for performance measurement

// BenchmarkLoadConfig measures the performance of configuration loading
func BenchmarkLoadConfig(b *testing.B) {
	dir := b.TempDir()
	serverPath := filepath.Join(dir, "server.yaml")
	accountsPath := filepath.Join(dir, "accounts.yaml")

	// Create realistic server configuration
	serverYAML := `
server:
  port: 9101
  region_concurrency: 4
  product_concurrency: 2
  metric_concurrency: 5
  cache_ttl: 1d
  scrape_interval: 60s
  log:
    level: info
    format: json
    output: stdout
    file:
      path: logs/exporter.log
      max_size: 100
      max_backups: 3
      max_age: 28
      compress: true
`
	if err := os.WriteFile(serverPath, []byte(serverYAML), 0644); err != nil {
		b.Fatalf("write server.yaml: %v", err)
	}

	// Create realistic accounts configuration with multiple providers and accounts
	accountsYAML := `
accounts:
  aliyun:
    - provider: aliyun
      account_id: "aliyun-account-1"
      access_key_id: "${AK1:-default_ak}"
      access_key_secret: "${SK1:-default_sk}"
      regions:
        - cn-hangzhou
        - cn-beijing
        - cn-shanghai
      resources: ["slb", "nlb", "oss"]
    - provider: aliyun
      account_id: "aliyun-account-2"
      access_key_id: "ak2"
      access_key_secret: "sk2"
      regions: ["cn-shenzhen"]
      resources: ["bwp"]
  tencent:
    - provider: tencent
      account_id: "tencent-account-1"
      secret_id: "${TENCENT_SID:-sid}"
      secret_key: "${TENCENT_SKEY:-skey}"
      regions: ["ap-guangzhou", "ap-shanghai"]
      resources: ["clb"]
  aws:
    - provider: aws
      account_id: "aws-account-1"
      access_key_id: "aws_ak"
      access_key_secret: "aws_sk"
      regions: ["us-east-1", "us-west-2"]
      resources: ["s3"]
`
	if err := os.WriteFile(accountsPath, []byte(accountsYAML), 0644); err != nil {
		b.Fatalf("write accounts.yaml: %v", err)
	}

	_ = os.Setenv("SERVER_PATH", serverPath)
	defer func() { _ = os.Unsetenv("SERVER_PATH") }()
	_ = os.Setenv("ACCOUNTS_PATH", accountsPath)
	defer func() { _ = os.Unsetenv("ACCOUNTS_PATH") }()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := LoadConfig()
		if err != nil {
			b.Fatalf("LoadConfig error: %v", err)
		}
	}
}

// BenchmarkLoadConfig_WithEnvExpansion measures performance with environment variable expansion
func BenchmarkLoadConfig_WithEnvExpansion(b *testing.B) {
	dir := b.TempDir()
	serverPath := filepath.Join(dir, "server.yaml")
	accountsPath := filepath.Join(dir, "accounts.yaml")

	serverYAML := `
server:
  port: ${PORT:-9101}
  scrape_interval: ${INTERVAL:-60s}
`
	if err := os.WriteFile(serverPath, []byte(serverYAML), 0644); err != nil {
		b.Fatalf("write server.yaml: %v", err)
	}

	accountsYAML := `
accounts:
  aliyun:
    - provider: aliyun
      account_id: "${ACCOUNT_ID:-default}"
      access_key_id: "${AK:-ak}"
      access_key_secret: "${SK:-sk}"
      regions: ["cn-hangzhou"]
      resources: ["slb"]
`
	if err := os.WriteFile(accountsPath, []byte(accountsYAML), 0644); err != nil {
		b.Fatalf("write accounts.yaml: %v", err)
	}

	_ = os.Setenv("SERVER_PATH", serverPath)
	_ = os.Setenv("ACCOUNTS_PATH", accountsPath)
	_ = os.Setenv("PORT", "9101")
	_ = os.Setenv("INTERVAL", "120s")
	_ = os.Setenv("ACCOUNT_ID", "test-account")
	_ = os.Setenv("AK", "test_ak")
	_ = os.Setenv("SK", "test_sk")
	defer func() {
		_ = os.Unsetenv("SERVER_PATH")
		_ = os.Unsetenv("ACCOUNTS_PATH")
		_ = os.Unsetenv("PORT")
		_ = os.Unsetenv("INTERVAL")
		_ = os.Unsetenv("ACCOUNT_ID")
		_ = os.Unsetenv("AK")
		_ = os.Unsetenv("SK")
	}()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := LoadConfig()
		if err != nil {
			b.Fatalf("LoadConfig error: %v", err)
		}
	}
}

// BenchmarkLoadConfig_LargeConfig measures performance with a large configuration
func BenchmarkLoadConfig_LargeConfig(b *testing.B) {
	dir := b.TempDir()
	serverPath := filepath.Join(dir, "server.yaml")
	accountsPath := filepath.Join(dir, "accounts.yaml")

	serverYAML := `
server:
  port: 9101
  region_concurrency: 4
  product_concurrency: 2
  metric_concurrency: 5
`
	if err := os.WriteFile(serverPath, []byte(serverYAML), 0644); err != nil {
		b.Fatalf("write server.yaml: %v", err)
	}

	// Generate a large accounts configuration (20 accounts across 4 providers)
	accountsBuilder := `accounts:
`
	providers := []string{"aliyun", "tencent", "aws", "huawei"}
	regions := map[string][]string{
		"aliyun":  {"cn-hangzhou", "cn-beijing", "cn-shanghai", "cn-shenzhen"},
		"tencent": {"ap-guangzhou", "ap-shanghai", "ap-beijing"},
		"aws":     {"us-east-1", "us-west-2", "eu-west-1"},
		"huawei":  {"cn-north-1", "cn-south-1"},
	}
	resources := map[string][]string{
		"aliyun":  {"slb", "nlb", "oss", "bwp"},
		"tencent": {"clb"},
		"aws":     {"s3"},
		"huawei":  {"elb"},
	}

	for _, provider := range providers {
		accountsBuilder += fmt.Sprintf("  %s:\n", provider)
		for i := 0; i < 5; i++ {
			accountsBuilder += fmt.Sprintf(`    - provider: %s
      account_id: "%s-account-%d"
      access_key_id: "ak%d"
      access_key_secret: "sk%d"
      regions:
`, provider, provider, i, i, i)
			for _, region := range regions[provider] {
				accountsBuilder += fmt.Sprintf("        - %s\n", region)
			}
			accountsBuilder += fmt.Sprintf("      resources: %v\n", resources[provider])
		}
	}

	if err := os.WriteFile(accountsPath, []byte(accountsBuilder), 0644); err != nil {
		b.Fatalf("write accounts.yaml: %v", err)
	}

	_ = os.Setenv("SERVER_PATH", serverPath)
	_ = os.Setenv("ACCOUNTS_PATH", accountsPath)
	defer func() {
		_ = os.Unsetenv("SERVER_PATH")
		_ = os.Unsetenv("ACCOUNTS_PATH")
	}()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := LoadConfig()
		if err != nil {
			b.Fatalf("LoadConfig error: %v", err)
		}
	}
}
