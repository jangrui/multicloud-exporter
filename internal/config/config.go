// 配置包：定义账号与全局配置结构，提供 YAML 加载能力
package config

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// CloudAccount 描述单个云账号的采集范围与凭证
type CloudAccount struct {
	Provider        string   `yaml:"provider"`
	AccountID       string   `yaml:"account_id"`
	AccessKeyID     string   `yaml:"access_key_id"`
	AccessKeySecret string   `yaml:"access_key_secret"`
	Regions         []string `yaml:"regions"`
	Resources       []string `yaml:"resources"`
}

// expandEnv replaces ${var} or $var in the string according to the values
// of the current environment variables. It supports default values using
// the ${var:-default} syntax.
func expandEnv(s string) string {
	return os.Expand(s, func(key string) string {
		// Handle ${VAR:-default}
		if k, def, cut := strings.Cut(key, ":-"); cut {
			if v, ok := os.LookupEnv(k); ok && v != "" {
				return v
			}
			return def
		}
		return os.Getenv(key)
	})
}

// Config 汇总所有云账号配置
type Config struct {
	Mu sync.RWMutex `yaml:"-"`

	Server *ServerConf `yaml:"server"`
	// ServerConf 已废弃，保留用于向后兼容，将在加载时合并到 Server
	ServerConf *ServerConf     `yaml:"serverconf"` // Deprecated: use Server instead
	RemoteProm *RemoteProm     `yaml:"remote_prom"`
	Credential *Credential     `yaml:"credential"`
	DataTag    []DataTag       `yaml:"datatag"`
	Estimation *EstimationConf `yaml:"estimation"`

	AccountsByProvider map[string][]CloudAccount `yaml:"accounts"`

	ProductsByProvider map[string][]Product `yaml:"products"`
}

// GetServer 获取 Server 配置，优先返回 Server，如果为空则返回 ServerConf（向后兼容）
func (c *Config) GetServer() *ServerConf {
	if c.Server != nil {
		return c.Server
	}
	return c.ServerConf
}

// DefaultResourceDimMapping 返回默认的资源维度映射配置
func DefaultResourceDimMapping() map[string][]string {
	return map[string][]string{
		// Aliyun
		"aliyun.acs_ecs_dashboard":     {"InstanceId", "instanceId", "instance_id"},
		"aliyun.acs_slb_dashboard":     {"InstanceId", "instanceId", "instance_id", "groupId", "group_id", "userId", "vip", "port", "protocol"},
		"aliyun.acs_bandwidth_package": {"BandwidthPackageId", "bandwidthPackageId", "sharebandwidthpackages", "userId", "instanceId"},
		"aliyun.acs_oss_dashboard":     {"BucketName", "bucketName", "bucket_name", "userId", "instanceId"},
		"aliyun.acs_alb":               {"loadBalancerId", "LoadBalancerId", "serverGroupId", "listenerId", "vip", "userId", "listenerProtocol", "listenerPort", "ruleId"},
		"aliyun.acs_nlb":               {"InstanceId", "instanceId", "instance_id", "listenerId", "vip", "userId", "listenerPort", "listenerProtocol"},
		"aliyun.acs_gwlb":              {"instanceId", "InstanceId", "instance_id", "userId", "regionId", "availableZone", "addressIpVersion", "serverGroupId"},
		// Tencent
		"tencent.QCE/CVM":  {"InstanceId"},
		"tencent.QCE/LB":   {"LoadBalancerId", "vip"},
		"tencent.qce/gwlb": {"gwLoadBalancerId", "GwLoadBalancerId"},
		// AWS (Example)
		"aws.AWS/EC2": {"InstanceId"},
		"aws.AWS/ELB": {"LoadBalancerName"},
	}
}

// Validate 验证配置的完整性和合法性
func (c *Config) Validate() error {
	var errs []string

	// 验证 Server 配置
	if c.Server == nil && c.ServerConf == nil {
		errs = append(errs, "server config is required")
	} else {
		server := c.GetServer()
		// 验证端口
		if server.Port <= 0 || server.Port > 65535 {
			errs = append(errs, fmt.Sprintf("invalid port: %d (must be 1-65535)", server.Port))
		}

		// 验证日志配置
		if server.Log != nil {
			level := strings.ToLower(server.Log.Level)
			validLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true, "fatal": true}
			if !validLevels[level] {
				errs = append(errs, fmt.Sprintf("invalid log level: %s", server.Log.Level))
			}

			output := strings.ToLower(server.Log.Output)
			validOutputs := map[string]bool{"stdout": true, "console": true, "file": true, "both": true}
			if output != "" && !validOutputs[output] {
				errs = append(errs, fmt.Sprintf("invalid log output: %s", server.Log.Output))
			}
		}

		// 验证并发配置
		if server.RegionConcurrency < 0 || server.RegionConcurrency > 20 {
			errs = append(errs, fmt.Sprintf("invalid region_concurrency: %d (must be 0-20)", server.RegionConcurrency))
		}
		if server.MetricConcurrency < 0 || server.MetricConcurrency > 20 {
			errs = append(errs, fmt.Sprintf("invalid metric_concurrency: %d (must be 0-20)", server.MetricConcurrency))
		}
		if server.ProductConcurrency < 0 || server.ProductConcurrency > 10 {
			errs = append(errs, fmt.Sprintf("invalid product_concurrency: %d (must be 0-10)", server.ProductConcurrency))
		}
	}

	// 验证账号配置
	if len(c.AccountsByProvider) == 0 {
		errs = append(errs, "no accounts configured")
	}

	for provider, accounts := range c.AccountsByProvider {
		for i, acc := range accounts {
			if acc.AccountID == "" {
				errs = append(errs, fmt.Sprintf("%s: account[%d].account_id is required", provider, i))
			}
			if acc.AccessKeyID == "" {
				errs = append(errs, fmt.Sprintf("%s: account[%d].access_key_id is required", provider, i))
			}
			if acc.AccessKeySecret == "" {
				errs = append(errs, fmt.Sprintf("%s: account[%d].access_key_secret is required", provider, i))
			}
			if len(acc.Regions) == 0 {
				errs = append(errs, fmt.Sprintf("%s: account[%d].regions is empty", provider, i))
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("config validation failed:\n  - %s", strings.Join(errs, "\n  - "))
	}

	return nil
}

// LoadConfig 从环境变量加载拆分配置文件
func LoadConfig() (*Config, error) {
	var cfg Config

	// 加载 server.yaml
	serverPath := os.Getenv("SERVER_PATH")
	data, actualPath, err := LoadConfigFile(serverPath, []string{"/app/configs/server.yaml", "./configs/server.yaml"})
	if err == nil && data != nil {
		expanded := expandEnv(string(data))
		var s struct {
			Server *ServerConf `yaml:"server"`
		}
		if err := yaml.Unmarshal([]byte(expanded), &s); err != nil {
			return nil, fmt.Errorf("failed to parse server config from %s: %v", actualPath, err)
		}
		if s.Server != nil {
			cfg.Server = s.Server
			cfg.ServerConf = s.Server
			// 初始化默认维度映射
			if cfg.Server.ResourceDimMapping == nil {
				cfg.Server.ResourceDimMapping = DefaultResourceDimMapping()
			} else {
				// 合并默认配置（优先使用用户配置，缺失的补充默认）
				def := DefaultResourceDimMapping()
				for k, v := range def {
					if _, ok := cfg.Server.ResourceDimMapping[k]; !ok {
						cfg.Server.ResourceDimMapping[k] = v
					}
				}
			}
		}
		// 解析估算配置
		var est struct {
			Estimation *EstimationConf `yaml:"estimation"`
		}
		if err := yaml.Unmarshal([]byte(expanded), &est); err == nil && est.Estimation != nil {
			cfg.Estimation = est.Estimation
		}
	}

	// 手工产品配置已废弃：Exporter 全面采用自动发现生成产品与指标配置

	// 新版拆分：accounts.yaml
	accountsPath := os.Getenv("ACCOUNTS_PATH")
	if accountsPath != "" {
		// 如果明确指定了 ACCOUNTS_PATH，文件必须存在
		accData, _, err := LoadConfigFile(accountsPath, []string{})
		if err != nil {
			return nil, fmt.Errorf("failed to load accounts config from %s: %v", accountsPath, err)
		}
		accExpanded := expandEnv(string(accData))
		var accCfg struct {
			AccountsByProvider map[string][]CloudAccount `yaml:"accounts"`
		}
		if err := yaml.Unmarshal([]byte(accExpanded), &accCfg); err != nil {
			return nil, fmt.Errorf("failed to parse accounts config: %v", err)
		}
		if accCfg.AccountsByProvider != nil {
			cfg.AccountsByProvider = accCfg.AccountsByProvider
		}
	} else {
		// 如果没有指定 ACCOUNTS_PATH，尝试默认路径（可选）
		accData, _, err := LoadConfigFile("", []string{"/app/configs/accounts.yaml", "./configs/accounts.yaml"})
		if err == nil && accData != nil {
			accExpanded := expandEnv(string(accData))
			var accCfg struct {
				AccountsByProvider map[string][]CloudAccount `yaml:"accounts"`
			}
			if err := yaml.Unmarshal([]byte(accExpanded), &accCfg); err != nil {
				return nil, fmt.Errorf("failed to parse accounts config: %v", err)
			}
			if accCfg.AccountsByProvider != nil {
				cfg.AccountsByProvider = accCfg.AccountsByProvider
			}
		}
	}

	// 账号文件中若包含环境占位符，将通过 env 展开（由容器 envFrom 注入）
	return &cfg, nil
}

type ServerConf struct {
	ServiceEndpoint string `yaml:"service_endpoint"`
	Port            int    `yaml:"port"`
	PageSize        int    `yaml:"page_size"`
	// Deprecated: use Log.Output instead
	LogDest int `yaml:"log_dest"`
	// Deprecated: use Log.File.Path instead
	LogDir string `yaml:"log_dir"`
	// Deprecated: use Log.Level instead
	LogLevel   string     `yaml:"log_level"`
	Log        *LogConfig `yaml:"log"`
	HttpProxy  string     `yaml:"http_proxy"`
	HttpsProxy string     `yaml:"https_proxy"`
	NoProxy    string     `yaml:"no_proxy"`
	NoMeta     bool       `yaml:"no_meta"`
	// DiscoveryTTL 控制资源自动发现结果的缓存生命周期。
	// 支持的时间单位：
	//   - s: 秒 (second)
	//   - m: 分钟 (minute)
	//   - h: 小时 (hour)
	//   - d: 天 (day, 1d = 24h = 1440m = 86400s)
	// 若未指定单位，默认单位为纳秒（ns，Go time.ParseDuration 行为），因此建议始终显式指定单位。
	// 示例：
	//   - "1d": 缓存 1 天
	//   - "60m": 缓存 60 分钟
	//   - "24h": 缓存 24 小时
	DiscoveryTTL     string `yaml:"discovery_ttl"`
	DiscoveryRefresh string `yaml:"discovery_refresh"`
	ScrapeInterval   string `yaml:"scrape_interval"`
	// PeriodFallback 当无法从元数据获取 Period 时的默认值（秒），默认 60
	PeriodFallback int `yaml:"period_fallback"`
	// 区域级并发：同一账号下并行采集的地域数量，建议 1-8。
	RegionConcurrency int `yaml:"region_concurrency"`
	// 指标级并发：同一地域、同一产品下并行处理的指标批次数，建议 1-10。
	MetricConcurrency int `yaml:"metric_concurrency"`
	// 产品级并发：同一地域下并行处理的命名空间（云产品）数量，建议 1-4。
	ProductConcurrency int `yaml:"product_concurrency"`

	// RegionDiscovery 定义智能区域发现配置
	RegionDiscovery *RegionDiscoveryConf `yaml:"region_discovery"`

	// ResourceDimMapping 定义各云厂商、各产品（Namespace）的资源维度校验规则。
	// Key 为 "provider.namespace"，例如 "aliyun.acs_ecs_dashboard"。
	// Value 为该产品必须包含的维度键列表（任一匹配即可），例如 ["InstanceId", "instance_id"]。
	ResourceDimMapping map[string][]string `yaml:"resource_dim_mapping"`
	AdminAuthEnabled   bool                `yaml:"admin_auth_enabled"`
	AdminAuth          []BasicAuth         `yaml:"admin_auth"`
}

// RegionDiscoveryConf 定义智能区域发现配置
type RegionDiscoveryConf struct {
	Enabled           bool   `yaml:"enabled"`            // 是否启用智能区域发现，默认 true
	DiscoveryInterval string `yaml:"discovery_interval"` // 重新发现周期，如 "24h"
	EmptyThreshold    int    `yaml:"empty_threshold"`    // 连续空次数阈值，默认 3
	DataDir           string `yaml:"data_dir"`           // 数据目录路径，如 "/app/data"
	PersistFile       string `yaml:"persist_file"`       // 持久化文件名，如 "region_status.json"（相对于 data_dir）
}

type FileLogConfig struct {
	Path       string `yaml:"path"`
	MaxSize    int    `yaml:"max_size"` // in MB
	MaxBackups int    `yaml:"max_backups"`
	MaxAge     int    `yaml:"max_age"` // in days
	Compress   bool   `yaml:"compress"`
}

type LogConfig struct {
	Level  string         `yaml:"level"`  // debug, info, warn, error
	Format string         `yaml:"format"` // json, console
	Output string         `yaml:"output"` // stdout, file, both
	File   *FileLogConfig `yaml:"file"`
}

type BasicAuth struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

type RemoteProm struct {
	Endpoint  string     `yaml:"endpoint"`
	BasicAuth *BasicAuth `yaml:"basic_auth"`
}

type Credential struct {
	UserID       string `yaml:"user_id"`
	AccessKey    string `yaml:"access_key"`
	AccessSecret string `yaml:"access_secret"`
}

type DataTag struct {
	Key string `yaml:"key"`
	Val string `yaml:"val"`
}

type MetricGroup struct {
	MetricList []string `yaml:"metric_list"`
	Period     *int     `yaml:"period"`
	Statistics []string `yaml:"statistics"`
}

type Product struct {
	Namespace    string        `yaml:"namespace"`
	Period       *int          `yaml:"period"`
	AutoDiscover bool          `yaml:"auto_discover"`
	MetricInfo   []MetricGroup `yaml:"metric_info"`
}

// EstimationConf 定义估算相关的全局配置
type EstimationConf struct {
	CLB *CLBEstimationConf `yaml:"clb"`
}

// CLBEstimationConf 定义 CLB 估算策略
type CLBEstimationConf struct {
	AliyunBandwidthCapBps int            `yaml:"aliyun_bandwidth_cap_bps"`
	PerInstanceCapBps     map[string]int `yaml:"per_instance_cap_bps"`
}
