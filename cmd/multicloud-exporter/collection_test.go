package main

import (
	"context"
	"time"

	"testing"

	"multicloud-exporter/internal/config"

	"github.com/stretchr/testify/assert"
)

func TestGetScrapeInterval(t *testing.T) {
	tests := []struct {
		name             string
		cfg              *config.Config
		envValue         string
		expectedDuration int
	}{
		{"default config", &config.Config{}, "", 60},
		{"config value", &config.Config{Server: &config.ServerConf{ScrapeInterval: "30s"}}, "", 30},
		{"env override", &config.Config{Server: &config.ServerConf{ScrapeInterval: "30s"}}, "120", 120},
		{"env duration parses to seconds", &config.Config{}, "2m", 2},
		{"invalid config", &config.Config{Server: &config.ServerConf{ScrapeInterval: "invalid"}}, "", 60},
		{"env pure seconds", &config.Config{}, "300", 300},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				t.Setenv("SCRAPE_INTERVAL", tt.envValue)
			} else {
				t.Setenv("SCRAPE_INTERVAL", "")
			}
			result := getScrapeInterval(tt.cfg)
			assert.Equal(t, time.Duration(tt.expectedDuration)*time.Second, result)
		})
	}
}

func TestGetServerPort(t *testing.T) {
	tests := []struct {
		name     string
		cfg      *config.Config
		envValue string
		expected string
	}{
		{"default", &config.Config{}, "", "9101"},
		{"config value", &config.Config{Server: &config.ServerConf{Port: 8080}}, "", "8080"},
		{"env override", &config.Config{Server: &config.ServerConf{Port: 8080}}, "9090", "9090"},
		{"env only", &config.Config{}, "9090", "9090"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				t.Setenv("EXPORTER_PORT", tt.envValue)
			} else {
				t.Setenv("EXPORTER_PORT", "")
			}
			result := getServerPort(tt.cfg)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCalculateFirstRunDelay(t *testing.T) {
	tests := []struct {
		name        string
		interval    time.Duration
		strategy    string
		expectedMin time.Duration
		expectedMax time.Duration
	}{
		{"immediate strategy", 60 * time.Second, "immediate", 0, 0},
		{"short interval", 60 * time.Second, "auto", 0, 30 * time.Second},
		{"long interval", 600 * time.Second, "auto", 0, 60 * time.Second},
		{"minimum", 30 * time.Second, "auto", 0, 5 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("FIRST_RUN_STRATEGY", tt.strategy)
			t.Setenv("CLUSTER_STABILITY_CHECK_ENABLED", "false")
			t.Setenv("CLUSTER_STABILITY_MAX_WAIT", "10")
			result := calculateFirstRunDelay(tt.interval)
			assert.GreaterOrEqual(t, result, tt.expectedMin)
			assert.LessOrEqual(t, result, tt.expectedMax)
		})
	}
}

func TestCalculateAutoDelay(t *testing.T) {
	tests := []struct {
		name        string
		totalShards int
		shardIndex  int
		maxDelay    time.Duration
	}{
		{"single shard", 1, 0, time.Minute},
		{"two shards index 0", 2, 0, time.Minute},
		{"two shards index 1", 2, 1, time.Minute},
		{"many shards", 10, 5, 5 * time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculateAutoDelay(tt.totalShards, tt.shardIndex, tt.maxDelay)
			assert.GreaterOrEqual(t, result, 0*time.Second)
			assert.LessOrEqual(t, result, tt.maxDelay)
		})
	}
}

func TestCalculateStaggeredDelay(t *testing.T) {
	tests := []struct {
		name        string
		totalShards int
		shardIndex  int
		maxDelay    time.Duration
		expectedMin time.Duration
	}{
		{"single shard", 1, 0, time.Minute, 0},
		{"two shards index 0", 2, 0, time.Minute, 0},
		{"two shards index 1", 2, 1, time.Minute, time.Minute / 2},
		{"many shards index 5", 10, 5, 5 * time.Minute, 5 * time.Minute / 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculateStaggeredDelay(tt.totalShards, tt.shardIndex, tt.maxDelay)
			assert.GreaterOrEqual(t, result, tt.expectedMin)
			assert.LessOrEqual(t, result, tt.maxDelay)
		})
	}
}

func TestBuildProductStats(t *testing.T) {
	tests := []struct {
		name               string
		productsByProvider map[string]int
		want               string
	}{
		{"空映射返回空字符串", map[string]int{}, ""},
		{"单个厂商", map[string]int{"aliyun": 5}, " (aliyun=5)"},
		{"多个厂商", map[string]int{"aliyun": 5, "tencent": 3}, " (aliyun=5, tencent=3)"},
		{"按字母排序", map[string]int{"tencent": 3, "aliyun": 5, "aws": 2}, " (aliyun=5, aws=2, tencent=3)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildProductStats(tt.productsByProvider)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestPow(t *testing.T) {
	tests := []struct {
		name string
		base float64
		exp  int
		want float64
	}{
		{"零次幂", 2.0, 0, 1.0},
		{"一次幂", 2.0, 1, 2.0},
		{"二次幂", 2.0, 2, 4.0},
		{"五次幂", 1.5, 5, 7.59375},
		{"小数底数", 3.0, 3, 27.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pow(tt.base, tt.exp)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestInitializeDiscovery(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *config.Config
		wantErr bool
	}{
		{"nil配置返回错误", nil, true},
		{"空配置创建Manager", &config.Config{}, false},
		{"带厂商配置", &config.Config{
			ProductsByProvider: map[string][]config.Product{
				"aliyun": {{Namespace: "aliyun.acs_slb_dashboard"}},
			},
		}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := initializeDiscovery(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("initializeDiscovery() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got == nil {
				t.Error("initializeDiscovery() should return manager but got nil")
			}
			if got != nil {
				ctx, cancel := context.WithCancel(context.Background())
				got.Start(ctx)
				cancel()
			}
		})
	}
}
