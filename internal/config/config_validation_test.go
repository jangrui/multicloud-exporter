// 配置验证测试
package config

import (
	"strings"
	"testing"
)

// TestValidate_RegionDiscoveryIntervalWarning 测试区域重新发现周期小于采集周期时的警告
func TestValidate_RegionDiscoveryIntervalWarning(t *testing.T) {
	cfg := &Config{
		Server: &ServerConf{
			Port:           9101,
			ScrapeInterval: "5m", // 5分钟
			RegionDiscovery: &RegionDiscoveryConf{
				Enabled:           true,
				DiscoveryInterval: "1m", // 1分钟，小于采集周期
				EmptyThreshold:    3,
			},
		},
		AccountsByProvider: map[string][]CloudAccount{
			"aliyun": {
				{
					AccountID:       "test",
					AccessKeyID:     "test",
					AccessKeySecret: "test",
					Regions:         []string{"cn-hangzhou"},
				},
			},
		},
	}

	// 捕获 stderr 输出
	// 注意：由于 Validate 直接输出到 stderr，这里只验证不报错
	err := cfg.Validate()
	if err != nil {
		t.Errorf("Validate() should not return error for warning, got: %v", err)
	}
}

// TestValidate_FirstRunMaxDelayNegative 测试首次采集最大延迟为负数时的处理
func TestValidate_FirstRunMaxDelayNegative(t *testing.T) {
	cfg := &Config{
		Server: &ServerConf{
			Port: 9101,
			FirstRun: &FirstRunConf{
				MaxDelay: -10, // 负数
			},
		},
		AccountsByProvider: map[string][]CloudAccount{
			"aliyun": {
				{
					AccountID:       "test",
					AccessKeyID:     "test",
					AccessKeySecret: "test",
					Regions:         []string{"cn-hangzhou"},
				},
			},
		},
	}

	err := cfg.Validate()
	if err != nil {
		t.Errorf("Validate() should not return error for warning, got: %v", err)
	}

	// 验证值被修正为 180
	if cfg.Server.FirstRun.MaxDelay != 180 {
		t.Errorf("FirstRunMaxDelay should be corrected to 180, got: %d", cfg.Server.FirstRun.MaxDelay)
	}
}

// TestValidate_InvalidFirstRunStrategy 测试无效的首次采集策略
func TestValidate_InvalidFirstRunStrategy(t *testing.T) {
	cfg := &Config{
		Server: &ServerConf{
			Port: 9101,
			FirstRun: &FirstRunConf{
				Strategy: "invalid", // 无效策略
			},
		},
		AccountsByProvider: map[string][]CloudAccount{
			"aliyun": {
				{
					AccountID:       "test",
					AccessKeyID:     "test",
					AccessKeySecret: "test",
					Regions:         []string{"cn-hangzhou"},
				},
			},
		},
	}

	err := cfg.Validate()
	if err != nil {
		t.Errorf("Validate() should not return error for warning, got: %v", err)
	}

	// 验证值被修正为 "auto"
	if cfg.Server.FirstRun.Strategy != "auto" {
		t.Errorf("FirstRunStrategy should be corrected to 'auto', got: %s", cfg.Server.FirstRun.Strategy)
	}
}

// TestValidate_MissingRequiredFields 测试缺少必需字段时的错误
func TestValidate_MissingRequiredFields(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "missing server config",
			cfg: &Config{
				AccountsByProvider: map[string][]CloudAccount{
					"aliyun": {{AccountID: "test", AccessKeyID: "test", AccessKeySecret: "test", Regions: []string{"cn-hangzhou"}}},
				},
			},
			wantErr: true,
			errMsg:  "server config is required",
		},
		{
			name: "missing accounts",
			cfg: &Config{
				Server:             &ServerConf{Port: 9101},
				AccountsByProvider: map[string][]CloudAccount{},
			},
			wantErr: true,
			errMsg:  "no accounts configured",
		},
		{
			name: "invalid port",
			cfg: &Config{
				Server: &ServerConf{Port: 99999},
				AccountsByProvider: map[string][]CloudAccount{
					"aliyun": {{AccountID: "test", AccessKeyID: "test", AccessKeySecret: "test", Regions: []string{"cn-hangzhou"}}},
				},
			},
			wantErr: true,
			errMsg:  "invalid port",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("Validate() error = %v, should contain %s", err, tt.errMsg)
			}
		})
	}
}

// TestValidate_ValidConfig 测试有效配置
func TestValidate_ValidConfig(t *testing.T) {
	cfg := &Config{
		Server: &ServerConf{
			Port:           9101,
			ScrapeInterval: "5m",
			FirstRun: &FirstRunConf{
				Strategy: "auto",
				MaxDelay: 180,
			},
			RegionDiscovery: &RegionDiscoveryConf{
				Enabled:           true,
				DiscoveryInterval: "1h", // 大于采集周期，不会警告
				EmptyThreshold:    3,
			},
			ClusterStabilityCheck: &ClusterStabilityCheckConf{
				Enabled:        true,
				MaxWait:        "30s",
				CheckInterval:  "2s",
				RequiredStable: 3,
			},
		},
		AccountsByProvider: map[string][]CloudAccount{
			"aliyun": {
				{
					AccountID:       "test-account",
					AccessKeyID:     "test-key",
					AccessKeySecret: "test-secret",
					Regions:         []string{"cn-hangzhou", "cn-beijing"},
				},
			},
		},
	}

	err := cfg.Validate()
	if err != nil {
		t.Errorf("Validate() should not return error for valid config, got: %v", err)
	}
}
