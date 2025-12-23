package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExpandEnv(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		env      map[string]string
		expected string
	}{
		{
			name:     "no env vars",
			input:    "val: ${VAR}",
			env:      nil,
			expected: "val: ",
		},
		{
			name:     "simple substitution",
			input:    "val: ${VAR}",
			env:      map[string]string{"VAR": "123"},
			expected: "val: 123",
		},
		{
			name:     "default value used",
			input:    "val: ${VAR:-456}",
			env:      nil,
			expected: "val: 456",
		},
		{
			name:     "default value ignored when env set",
			input:    "val: ${VAR:-456}",
			env:      map[string]string{"VAR": "123"},
			expected: "val: 123",
		},
		{
			name:     "empty env var uses default",
			input:    "val: ${VAR:-456}",
			env:      map[string]string{"VAR": ""},
			expected: "val: 456",
		},
		{
			name:     "multiple vars",
			input:    "a: ${A:-1}, b: ${B:-2}",
			env:      map[string]string{"A": "10"},
			expected: "a: 10, b: 2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
            if tt.env == nil {
                if err := os.Unsetenv("VAR"); err != nil {
                    t.Fatal(err)
                }
            }

			got := expandEnv(tt.input)
			if got != tt.expected {
				t.Errorf("expandEnv(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestLoadConfig(t *testing.T) {
	// Create temporary directory for config files
	tmpDir, err := os.MkdirTemp("", "config_test")
	if err != nil {
		t.Fatal(err)
	}
    defer func() { _ = os.RemoveAll(tmpDir) }()

	// 1. Create server.yaml
	serverYaml := `
server:
  region_concurrency: 5
  metric_concurrency: 10
`
	serverPath := filepath.Join(tmpDir, "server.yaml")
	if err = os.WriteFile(serverPath, []byte(serverYaml), 0644); err != nil {
		t.Fatal(err)
	}

	// 2. Create accounts.yaml
	accountsYaml := `
accounts:
  aliyun:
    - account_id: "123456"
      access_key_id: "ak"
      access_key_secret: "sk"
      regions: ["cn-hangzhou"]
`
	accountsPath := filepath.Join(tmpDir, "accounts.yaml")
	if err = os.WriteFile(accountsPath, []byte(accountsYaml), 0644); err != nil {
		t.Fatal(err)
	}

	// 3. Set env vars
	t.Setenv("SERVER_PATH", serverPath)
	t.Setenv("ACCOUNTS_PATH", accountsPath)

	// 4. Load config
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	// 5. Verify
	if cfg.Server == nil {
		t.Fatal("cfg.Server is nil")
	}
	if cfg.Server.RegionConcurrency != 5 {
		t.Errorf("RegionConcurrency = %d, want 5", cfg.Server.RegionConcurrency)
	}
	aliyunAccounts, ok := cfg.AccountsByProvider["aliyun"]
	if !ok || len(aliyunAccounts) != 1 {
		t.Errorf("AccountsByProvider[aliyun] len = %d, want 1", len(aliyunAccounts))
	}
	if len(aliyunAccounts) > 0 && aliyunAccounts[0].AccountID != "123456" {
		t.Errorf("AccountID = %s, want 123456", aliyunAccounts[0].AccountID)
	}
}


func TestLoadConfig_Error(t *testing.T) {
	t.Setenv("ACCOUNTS_PATH", "/non/existent/path.yaml")
	_, err := LoadConfig()
	if err == nil {
		t.Error("LoadConfig() expected error for non-existent ACCOUNTS_PATH, got nil")
	}
}

func TestDefaultResourceDimMapping(t *testing.T) {
	mapping := DefaultResourceDimMapping()
	if len(mapping) == 0 {
		t.Error("DefaultResourceDimMapping() returned empty map")
	}
	if val, ok := mapping["aliyun.acs_ecs_dashboard"]; !ok || len(val) == 0 {
		t.Error("DefaultResourceDimMapping() missing aliyun.acs_ecs_dashboard")
	}
}
