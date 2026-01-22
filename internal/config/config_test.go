package config

import (
	"os"
	"path/filepath"
	"strings"
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

// TestParseDuration 测试时间间隔解析函数
func TestParseDuration(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		want    string // 期望的标准格式
	}{
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "days format",
			input:   "1d",
			wantErr: false,
			want:    "24h0m0s",
		},
		{
			name:    "multiple days",
			input:   "7d",
			wantErr: false,
			want:    "168h0m0s",
		},
		{
			name:    "hours format",
			input:   "24h",
			wantErr: false,
			want:    "24h0m0s",
		},
		{
			name:    "minutes format",
			input:   "30m",
			wantErr: false,
			want:    "30m0s",
		},
		{
			name:    "seconds format",
			input:   "60s",
			wantErr: false,
			want:    "1m0s",
		},
		{
			name:    "invalid format",
			input:   "invalid",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseDuration(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseDuration(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got.String() != tt.want {
				t.Errorf("parseDuration(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// TestValidateRegionDiscoveryConfig 测试区域发现配置验证
func TestValidateRegionDiscoveryConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config",
			config: &Config{
				Server: &ServerConf{
					Port: 9101,
					RegionDiscovery: &RegionDiscoveryConf{
						Enabled:           true,
						DiscoveryInterval: "1h",
						EmptyThreshold:    3,
					},
				},
				AccountsByProvider: map[string][]CloudAccount{
					"aliyun": {
						{
							AccountID:       "test",
							AccessKeyID:     "ak",
							AccessKeySecret: "sk",
							Regions:         []string{"cn-hangzhou"},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid empty_threshold - negative",
			config: &Config{
				Server: &ServerConf{
					Port: 9101,
					RegionDiscovery: &RegionDiscoveryConf{
						Enabled:           true,
						DiscoveryInterval: "1h",
						EmptyThreshold:    -1,
					},
				},
				AccountsByProvider: map[string][]CloudAccount{
					"aliyun": {{AccountID: "test", AccessKeyID: "ak", AccessKeySecret: "sk", Regions: []string{"cn-hangzhou"}}},
				},
			},
			wantErr: true,
			errMsg:  "empty_threshold",
		},
		{
			name: "invalid empty_threshold - too large",
			config: &Config{
				Server: &ServerConf{
					Port: 9101,
					RegionDiscovery: &RegionDiscoveryConf{
						Enabled:           true,
						DiscoveryInterval: "1h",
						EmptyThreshold:    101,
					},
				},
				AccountsByProvider: map[string][]CloudAccount{
					"aliyun": {{AccountID: "test", AccessKeyID: "ak", AccessKeySecret: "sk", Regions: []string{"cn-hangzhou"}}},
				},
			},
			wantErr: true,
			errMsg:  "empty_threshold",
		},
		{
			name: "invalid discovery_interval format",
			config: &Config{
				Server: &ServerConf{
					Port: 9101,
					RegionDiscovery: &RegionDiscoveryConf{
						Enabled:           true,
						DiscoveryInterval: "invalid",
						EmptyThreshold:    3,
					},
				},
				AccountsByProvider: map[string][]CloudAccount{
					"aliyun": {{AccountID: "test", AccessKeyID: "ak", AccessKeySecret: "sk", Regions: []string{"cn-hangzhou"}}},
				},
			},
			wantErr: true,
			errMsg:  "discovery_interval",
		},
		{
			name: "disabled region_discovery - no validation",
			config: &Config{
				Server: &ServerConf{
					Port: 9101,
					RegionDiscovery: &RegionDiscoveryConf{
						Enabled:           false,
						DiscoveryInterval: "",
						EmptyThreshold:    0,
					},
				},
				AccountsByProvider: map[string][]CloudAccount{
					"aliyun": {{AccountID: "test", AccessKeyID: "ak", AccessKeySecret: "sk", Regions: []string{"cn-hangzhou"}}},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && err != nil && tt.errMsg != "" {
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("Validate() error = %v, want error containing %q", err, tt.errMsg)
				}
			}
		})
	}
}

// TestValidateHuaweiCacheConfig 测试华为云缓存配置验证
func TestValidateHuaweiCacheConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid huawei cache config",
			config: &Config{
				Server: &ServerConf{
					Port: 9101,
					HuaweiCache: &HuaweiCacheConf{
						Enabled:     true,
						ResourceTTL: "10m",
						TagTTL:      "30m",
					},
				},
				AccountsByProvider: map[string][]CloudAccount{
					"huawei": {
						{
							AccountID:       "test",
							AccessKeyID:     "ak",
							AccessKeySecret: "sk",
							Regions:         []string{"cn-north-1"},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid resource_ttl format",
			config: &Config{
				Server: &ServerConf{
					Port: 9101,
					HuaweiCache: &HuaweiCacheConf{
						Enabled:     true,
						ResourceTTL: "invalid",
						TagTTL:      "30m",
					},
				},
				AccountsByProvider: map[string][]CloudAccount{
					"huawei": {{AccountID: "test", AccessKeyID: "ak", AccessKeySecret: "sk", Regions: []string{"cn-north-1"}}},
				},
			},
			wantErr: true,
			errMsg:  "resource_ttl",
		},
		{
			name: "invalid tag_ttl format",
			config: &Config{
				Server: &ServerConf{
					Port: 9101,
					HuaweiCache: &HuaweiCacheConf{
						Enabled:     true,
						ResourceTTL: "10m",
						TagTTL:      "invalid",
					},
				},
				AccountsByProvider: map[string][]CloudAccount{
					"huawei": {{AccountID: "test", AccessKeyID: "ak", AccessKeySecret: "sk", Regions: []string{"cn-north-1"}}},
				},
			},
			wantErr: true,
			errMsg:  "tag_ttl",
		},
		{
			name: "disabled huawei cache - no validation",
			config: &Config{
				Server: &ServerConf{
					Port: 9101,
					HuaweiCache: &HuaweiCacheConf{
						Enabled:     false,
						ResourceTTL: "",
						TagTTL:      "",
					},
				},
				AccountsByProvider: map[string][]CloudAccount{
					"huawei": {{AccountID: "test", AccessKeyID: "ak", AccessKeySecret: "sk", Regions: []string{"cn-north-1"}}},
				},
			},
			wantErr: false,
		},
		{
			name: "empty ttl values - should pass",
			config: &Config{
				Server: &ServerConf{
					Port: 9101,
					HuaweiCache: &HuaweiCacheConf{
						Enabled:     true,
						ResourceTTL: "",
						TagTTL:      "",
					},
				},
				AccountsByProvider: map[string][]CloudAccount{
					"huawei": {{AccountID: "test", AccessKeyID: "ak", AccessKeySecret: "sk", Regions: []string{"cn-north-1"}}},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && err != nil && tt.errMsg != "" {
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("Validate() error = %v, want error containing %q", err, tt.errMsg)
				}
			}
		})
	}
}

// TestValidateFirstRunStrategy 测试首次采集策略配置验证
func TestValidateFirstRunStrategy(t *testing.T) {
	tests := []struct {
		name   string
		config *Config
	}{
		{
			name: "valid strategy - auto",
			config: &Config{
				Server: &ServerConf{
					Port: 9101,
					FirstRun: &FirstRunConf{
						Strategy: "auto",
						MaxDelay: 180,
					},
				},
				AccountsByProvider: map[string][]CloudAccount{
					"aliyun": {{AccountID: "test", AccessKeyID: "ak", AccessKeySecret: "sk", Regions: []string{"cn-hangzhou"}}},
				},
			},
		},
		{
			name: "valid strategy - immediate",
			config: &Config{
				Server: &ServerConf{
					Port: 9101,
					FirstRun: &FirstRunConf{
						Strategy: "immediate",
						MaxDelay: 180,
					},
				},
				AccountsByProvider: map[string][]CloudAccount{
					"aliyun": {{AccountID: "test", AccessKeyID: "ak", AccessKeySecret: "sk", Regions: []string{"cn-hangzhou"}}},
				},
			},
		},
		{
			name: "valid strategy - staggered",
			config: &Config{
				Server: &ServerConf{
					Port: 9101,
					FirstRun: &FirstRunConf{
						Strategy: "staggered",
						MaxDelay: 180,
					},
				},
				AccountsByProvider: map[string][]CloudAccount{
					"aliyun": {{AccountID: "test", AccessKeyID: "ak", AccessKeySecret: "sk", Regions: []string{"cn-hangzhou"}}},
				},
			},
		},
		{
			name: "invalid strategy - should use default",
			config: &Config{
				Server: &ServerConf{
					Port: 9101,
					FirstRun: &FirstRunConf{
						Strategy: "invalid",
						MaxDelay: 180,
					},
				},
				AccountsByProvider: map[string][]CloudAccount{
					"aliyun": {{AccountID: "test", AccessKeyID: "ak", AccessKeySecret: "sk", Regions: []string{"cn-hangzhou"}}},
				},
			},
		},
		{
			name: "negative max_delay - should use default",
			config: &Config{
				Server: &ServerConf{
					Port: 9101,
					FirstRun: &FirstRunConf{
						Strategy: "auto",
						MaxDelay: -100,
					},
				},
				AccountsByProvider: map[string][]CloudAccount{
					"aliyun": {{AccountID: "test", AccessKeyID: "ak", AccessKeySecret: "sk", Regions: []string{"cn-hangzhou"}}},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if err != nil {
				t.Errorf("Validate() unexpected error = %v", err)
			}
			// 注意：警告信息会输出到 stderr，这里只验证不报错
			// 实际的警告验证需要捕获 stderr 输出，这里简化处理
		})
	}
}

// TestValidateClusterStabilityCheck 测试集群稳定性检测配置验证
func TestValidateClusterStabilityCheck(t *testing.T) {
	tests := []struct {
		name     string
		config   *Config
		wantWarn bool
	}{
		{
			name: "valid cluster stability check config",
			config: &Config{
				Server: &ServerConf{
					Port: 9101,
					ClusterStabilityCheck: &ClusterStabilityCheckConf{
						Enabled:        true,
						MaxWait:        "30s",
						CheckInterval:  "2s",
						RequiredStable: 3,
					},
				},
				AccountsByProvider: map[string][]CloudAccount{
					"aliyun": {{AccountID: "test", AccessKeyID: "ak", AccessKeySecret: "sk", Regions: []string{"cn-hangzhou"}}},
				},
			},
			wantWarn: false,
		},
		{
			name: "invalid max_wait format",
			config: &Config{
				Server: &ServerConf{
					Port: 9101,
					ClusterStabilityCheck: &ClusterStabilityCheckConf{
						Enabled:        true,
						MaxWait:        "invalid",
						CheckInterval:  "2s",
						RequiredStable: 3,
					},
				},
				AccountsByProvider: map[string][]CloudAccount{
					"aliyun": {{AccountID: "test", AccessKeyID: "ak", AccessKeySecret: "sk", Regions: []string{"cn-hangzhou"}}},
				},
			},
			wantWarn: true,
		},
		{
			name: "invalid check_interval format",
			config: &Config{
				Server: &ServerConf{
					Port: 9101,
					ClusterStabilityCheck: &ClusterStabilityCheckConf{
						Enabled:        true,
						MaxWait:        "30s",
						CheckInterval:  "invalid",
						RequiredStable: 3,
					},
				},
				AccountsByProvider: map[string][]CloudAccount{
					"aliyun": {{AccountID: "test", AccessKeyID: "ak", AccessKeySecret: "sk", Regions: []string{"cn-hangzhou"}}},
				},
			},
			wantWarn: true,
		},
		{
			name: "required_stable out of range - too small",
			config: &Config{
				Server: &ServerConf{
					Port: 9101,
					ClusterStabilityCheck: &ClusterStabilityCheckConf{
						Enabled:        true,
						MaxWait:        "30s",
						CheckInterval:  "2s",
						RequiredStable: 0,
					},
				},
				AccountsByProvider: map[string][]CloudAccount{
					"aliyun": {{AccountID: "test", AccessKeyID: "ak", AccessKeySecret: "sk", Regions: []string{"cn-hangzhou"}}},
				},
			},
			wantWarn: true,
		},
		{
			name: "required_stable out of range - too large",
			config: &Config{
				Server: &ServerConf{
					Port: 9101,
					ClusterStabilityCheck: &ClusterStabilityCheckConf{
						Enabled:        true,
						MaxWait:        "30s",
						CheckInterval:  "2s",
						RequiredStable: 11,
					},
				},
				AccountsByProvider: map[string][]CloudAccount{
					"aliyun": {{AccountID: "test", AccessKeyID: "ak", AccessKeySecret: "sk", Regions: []string{"cn-hangzhou"}}},
				},
			},
			wantWarn: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if err != nil {
				t.Errorf("Validate() unexpected error = %v", err)
			}
			// 注意：警告信息会输出到 stderr，这里只验证不报错
		})
	}
}
