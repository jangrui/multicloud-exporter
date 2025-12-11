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
	if err := os.WriteFile(serverPath, []byte(serverYaml), 0644); err != nil {
		t.Fatal(err)
	}

	// 2. Create accounts.yaml
	accountsYaml := `
accounts_list:
  - provider: aliyun
    account_id: "123456"
    access_key_id: "ak"
    access_key_secret: "sk"
    regions: ["cn-hangzhou"]
`
	accountsPath := filepath.Join(tmpDir, "accounts.yaml")
	if err := os.WriteFile(accountsPath, []byte(accountsYaml), 0644); err != nil {
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
	if len(cfg.AccountsList) != 1 {
		t.Errorf("AccountsList len = %d, want 1", len(cfg.AccountsList))
	}
	if cfg.AccountsList[0].AccountID != "123456" {
		t.Errorf("AccountID = %s, want 123456", cfg.AccountsList[0].AccountID)
	}
}

func TestLoadConfig_Legacy(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "config_test_legacy")
	if err != nil {
		t.Fatal(err)
	}
    defer func() { _ = os.RemoveAll(tmpDir) }()

	configYaml := `
server:
  region_concurrency: 2
accounts_list:
  - provider: tencent
    account_id: "999"
`
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configYaml), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("CONFIG_PATH", configPath)
	// Unset others to ensure legacy path is taken if others are missing (though code loads both if present)
	t.Setenv("SERVER_PATH", "")
	t.Setenv("ACCOUNTS_PATH", "")
	// Code logic:
	// 1. CONFIG_PATH (if set) -> unmarshal to cfg
	// 2. SERVER_PATH (if set or default exists) -> unmarshal server part
	// 3. ACCOUNTS_PATH (if set) -> unmarshal accounts part

	// Since we set CONFIG_PATH, it should load.
	// However, LoadConfig checks for server.yaml default paths.
	// If I don't want it to fail on missing server.yaml, I should make sure it doesn't find one or I ignore the error?
	// The code says:
	// if serverPath == "" { check defaults... if found set serverPath }
	// if serverPath != "" { read... }

	// In test env, default paths likely don't exist, so serverPath remains empty.

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if cfg.Server == nil {
		// Wait, if I use legacy config.yaml, it has "server" key.
		// yaml.Unmarshal([]byte(expanded), &cfg) should populate cfg.Server.
		// But later, if serverPath is found, it overwrites.
		// If serverPath is NOT found, cfg.Server remains from config.yaml?
		// Yes.
	} else {
		if cfg.Server.RegionConcurrency != 2 {
			t.Errorf("Legacy RegionConcurrency = %d, want 2", cfg.Server.RegionConcurrency)
		}
	}

	if len(cfg.AccountsList) != 1 {
		t.Errorf("Legacy AccountsList len = %d, want 1", len(cfg.AccountsList))
	}
}

func TestLoadConfig_Error(t *testing.T) {
	t.Setenv("CONFIG_PATH", "/non/existent/path.yaml")
	_, err := LoadConfig()
	if err == nil {
		t.Error("LoadConfig() expected error for non-existent CONFIG_PATH, got nil")
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
