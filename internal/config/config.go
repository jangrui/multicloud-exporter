// 配置包：定义账号与全局配置结构，提供 YAML 加载能力
package config

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

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

// expandEnv 根据当前环境变量的值替换字符串中的 ${var} 或 $var
// 支持使用 ${var:-default} 语法设置默认值
func expandEnv(s string) string {
	return os.Expand(s, func(key string) string {
		// 处理 ${VAR:-default} 语法
		if k, def, cut := strings.Cut(key, ":-"); cut {
			if v, ok := os.LookupEnv(k); ok && v != "" {
				return v
			}
			return def
		}
		return os.Getenv(key)
	})
}

// parseDuration 解析时间间隔字符串，支持 d(天) 单位
// 示例：1d -> 24h, 30m -> 30m, 1h -> 1h
func parseDuration(s string) (time.Duration, error) {
	if s == "" {
		return 0, fmt.Errorf("empty duration string")
	}

	// 支持 d(天) 单位
	if strings.HasSuffix(s, "d") {
		days := strings.TrimSuffix(s, "d")
		var d int
		if _, err := fmt.Sscanf(days, "%d", &d); err != nil {
			return 0, fmt.Errorf("invalid duration format: %s", s)
		}
		return time.Duration(d) * 24 * time.Hour, nil
	}

	// 使用标准库解析其他格式
	return time.ParseDuration(s)
}

// Config 汇总所有云账号配置
type Config struct {
	Mu sync.RWMutex `yaml:"-"`

	Server     *ServerConf     `yaml:"server"`
	RemoteProm *RemoteProm     `yaml:"remote_prom"`
	Credential *Credential     `yaml:"credential"`
	DataTag    []DataTag       `yaml:"datatag"`
	Estimation *EstimationConf `yaml:"estimation"`

	AccountsByProvider map[string][]CloudAccount `yaml:"accounts"`

	ProductsByProvider map[string][]Product `yaml:"products"`
}

// GetServer 获取 Server 配置
func (c *Config) GetServer() *ServerConf {
	return c.Server
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
	var warnings []string

	// 验证 Server 配置
	if c.Server == nil {
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

		// 验证区域发现配置
		if server.RegionDiscovery != nil {
			rd := server.RegionDiscovery

			// 验证 empty_threshold 范围
			if rd.EmptyThreshold < 0 || rd.EmptyThreshold > 100 {
				errs = append(errs, fmt.Sprintf("invalid region_discovery.empty_threshold: %d (must be 0-100)", rd.EmptyThreshold))
			}

			// 验证 discovery_interval 格式（如果设置了）
			if rd.DiscoveryInterval != "" {
				discoveryDuration, err := parseDuration(rd.DiscoveryInterval)
				if err != nil {
					errs = append(errs, fmt.Sprintf("invalid region_discovery.discovery_interval: %s (must be valid duration like '1h', '30m')", rd.DiscoveryInterval))
				} else {
					// 需求 9.3: 当区域重新发现周期小于采集周期时发出警告
					if server.ScrapeInterval != "" {
						scrapeDuration, err := parseDuration(server.ScrapeInterval)
						if err == nil && discoveryDuration < scrapeDuration {
							warnings = append(warnings, fmt.Sprintf(
								"region_discovery.discovery_interval (%s) is less than scrape_interval (%s), "+
									"this may cause frequent region rediscovery and impact performance",
								rd.DiscoveryInterval, server.ScrapeInterval))
						}
					}
				}
			}
		}

		// 验证华为云缓存配置
		if server.HuaweiCache != nil {
			hc := server.HuaweiCache

			// 验证 resource_ttl 格式（如果设置了）
			if hc.ResourceTTL != "" {
				if _, err := parseDuration(hc.ResourceTTL); err != nil {
					errs = append(errs, fmt.Sprintf("invalid huawei_cache.resource_ttl: %s (must be valid duration like '10m', '1h')", hc.ResourceTTL))
				}
			}

			// 验证 tag_ttl 格式（如果设置了）
			if hc.TagTTL != "" {
				if _, err := parseDuration(hc.TagTTL); err != nil {
					errs = append(errs, fmt.Sprintf("invalid huawei_cache.tag_ttl: %s (must be valid duration like '30m', '1h')", hc.TagTTL))
				}
			}
		}

		// 验证首次采集策略
		if server.FirstRun != nil {
			validStrategies := map[string]bool{"auto": true, "immediate": true, "staggered": true}
			if !validStrategies[server.FirstRun.Strategy] {
				warnings = append(warnings, fmt.Sprintf(
					"invalid first_run.strategy: %s (valid values: auto, immediate, staggered), will use 'auto'",
					server.FirstRun.Strategy))
				server.FirstRun.Strategy = "auto"
			}

			// 需求 9.2: 当首次采集最大延迟小于 0 时，使用 180 秒作为默认值
			if server.FirstRun.MaxDelay < 0 {
				warnings = append(warnings, fmt.Sprintf(
					"first_run.max_delay is negative (%d), will use default value 180 seconds",
					server.FirstRun.MaxDelay))
				server.FirstRun.MaxDelay = 180
			}
		}

		// 验证集群稳定性检测配置
		if server.ClusterStabilityCheck != nil {
			csc := server.ClusterStabilityCheck

			// 验证 max_wait 格式（如果设置了）
			if csc.MaxWait != "" {
				if _, err := parseDuration(csc.MaxWait); err != nil {
					warnings = append(warnings, fmt.Sprintf(
						"invalid cluster_stability_check.max_wait: %s, will use default 30s",
						csc.MaxWait))
				}
			}

			// 验证 check_interval 格式（如果设置了）
			if csc.CheckInterval != "" {
				if _, err := parseDuration(csc.CheckInterval); err != nil {
					warnings = append(warnings, fmt.Sprintf(
						"invalid cluster_stability_check.check_interval: %s, will use default 2s",
						csc.CheckInterval))
				}
			}

			// 验证 required_stable 范围
			if csc.RequiredStable < 1 || csc.RequiredStable > 10 {
				warnings = append(warnings, fmt.Sprintf(
					"cluster_stability_check.required_stable (%d) is out of range (1-10), will use default 3",
					csc.RequiredStable))
			}
		}

		// 验证四维架构配置（v0.5.0 新增）
		if server.FourDimension != nil {
			fd := server.FourDimension

			// 验证并发模式
			validModes := map[string]bool{"auto": true, "conservative": true, "standard": true, "aggressive": true}
			if !validModes[fd.ConcurrencyMode] {
				warnings = append(warnings, fmt.Sprintf(
					"invalid four_dimension.concurrency_mode: %s (valid: auto, conservative, standard, aggressive), will use 'auto'",
					fd.ConcurrencyMode))
				fd.ConcurrencyMode = "auto"
			}

			// 验证总并发度范围
			if fd.MaxConcurrency < 1 || fd.MaxConcurrency > 20 {
				warnings = append(warnings, fmt.Sprintf(
					"invalid four_dimension.max_concurrency: %d (must be 1-20), will use default 20",
					fd.MaxConcurrency))
				fd.MaxConcurrency = 20
			}
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

	// 需求 9.5: 记录验证结果
	// 注意：这里不能直接使用 logger，因为 logger 可能还未初始化
	// 警告信息将在 main.go 中通过 logger 记录
	if len(warnings) > 0 {
		// 将警告信息存储到配置对象中，供后续使用
		// 由于 Config 结构体没有 warnings 字段，我们在这里直接输出到 stderr
		for _, warning := range warnings {
			fmt.Fprintf(os.Stderr, "配置验证警告: %s\n", warning)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("config validation failed:\n  - %s", strings.Join(errs, "\n  - "))
	}

	// 需求 9.5: 记录验证成功
	fmt.Fprintf(os.Stderr, "配置验证通过: 检查了 %d 个配置项, 发现 %d 个警告\n",
		len(c.AccountsByProvider), len(warnings))

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
			for provider, accounts := range accCfg.AccountsByProvider {
				for i := range accounts {
					accounts[i].Provider = provider
				}
			}
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
				for provider, accounts := range accCfg.AccountsByProvider {
					for i := range accounts {
						accounts[i].Provider = provider
					}
				}
			}
		}
	}

	// 账号文件中若包含环境占位符，将通过 env 展开（由容器 envFrom 注入）
	return &cfg, nil
}

type ServerConf struct {
	ServiceEndpoint string     `yaml:"service_endpoint"`
	Port            int        `yaml:"port"`
	PageSize        int        `yaml:"page_size"`
	Log             *LogConfig `yaml:"log"`
	HttpProxy       string     `yaml:"http_proxy"`
	HttpsProxy      string     `yaml:"https_proxy"`
	NoProxy         string     `yaml:"no_proxy"`
	NoMeta          bool       `yaml:"no_meta"`
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
	DiscoveryTTL   string `yaml:"discovery_ttl"`
	ScrapeInterval string `yaml:"scrape_interval"`
	// TagCacheTTL 控制标签缓存的 TTL（分钟），默认 30
	TagCacheTTL int `yaml:"tag_cache_ttl"`
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

	// Cluster 定义集群同步配置
	Cluster *ClusterConf `yaml:"cluster"`

	// HuaweiCache 定义华为云缓存配置
	HuaweiCache *HuaweiCacheConf `yaml:"huawei_cache"`

	// FirstRun 定义首次采集策略配置
	FirstRun *FirstRunConf `yaml:"first_run"`

	// ClusterStabilityCheck 定义集群稳定性检测配置
	ClusterStabilityCheck *ClusterStabilityCheckConf `yaml:"cluster_stability_check"`

	// FourDimension 定义四维架构配置（v0.5.0 新增）
	FourDimension *FourDimensionConfig `yaml:"four_dimension"`

	// ResourceDimMapping 定义各云厂商、各产品的资源维度校验规则。
	// Key 为 "provider.namespace"，例如 "aliyun.acs_ecs_dashboard"。
	// Value 为该产品必须包含的维度键列表（任一匹配即可），例如 ["InstanceId", "instance_id"]。
	ResourceDimMapping map[string][]string `yaml:"resource_dim_mapping"`
	AdminAuthEnabled   bool                `yaml:"admin_auth_enabled"`
	AdminAuth          []BasicAuth         `yaml:"admin_auth"`
}

// RegionDiscoveryConf 定义智能区域发现配置
type RegionDiscoveryConf struct {
	Enabled           bool   `yaml:"enabled"`            // 是否启用智能区域发现，默认 true
	DiscoveryInterval string `yaml:"discovery_interval"` // 重新发现周期，如 "1h"
	EmptyThreshold    int    `yaml:"empty_threshold"`    // 连续空次数阈值，默认 3
}

// FirstRunConf 定义首次采集策略配置
type FirstRunConf struct {
	// 策略类型：auto（自动判断）、immediate（立即采集）、staggered（强制错峰）
	Strategy string `yaml:"strategy"`
	// 最大延迟时间（秒），默认 180
	MaxDelay int `yaml:"max_delay"`
}

// ClusterConf 定义集群同步配置
type ClusterConf struct {
	Enabled     bool   `yaml:"enabled"`      // 是否启用集群同步，默认 true
	ServiceName string `yaml:"service_name"` // Kubernetes Service 名称，用于自动发现，默认 "multicloud-exporter"
	Port        string `yaml:"port"`         // 同步端口，默认使用服务端口
	Secret      string `yaml:"secret"`       // 集群共享密钥，用于验证请求
}

// HuaweiCacheConf 定义华为云缓存配置
type HuaweiCacheConf struct {
	Enabled     bool   `yaml:"enabled"`      // 是否启用华为云缓存，默认 true
	ResourceTTL string `yaml:"resource_ttl"` // 资源缓存 TTL，如 "10m"
	TagTTL      string `yaml:"tag_ttl"`      // 标签缓存 TTL，如 "30m"
}

// ClusterStabilityCheckConf 定义集群稳定性检测配置
type ClusterStabilityCheckConf struct {
	Enabled        bool   `yaml:"enabled"`         // 是否启用集群稳定性检测，默认 true
	MaxWait        string `yaml:"max_wait"`        // 最长等待时间，如 "30s"
	CheckInterval  string `yaml:"check_interval"`  // 检查间隔，如 "2s"
	RequiredStable int    `yaml:"required_stable"` // 需要连续稳定的次数，默认 3
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

// FourDimensionConfig 定义四维架构配置（v0.5.0 新增）
type FourDimensionConfig struct {
	// 并发模式：auto（自动选择）、conservative（保守）、standard（标准）、aggressive（激进）
	ConcurrencyMode string `yaml:"concurrency_mode"`

	// 总并发度限制：避免触发云 API 限流，建议范围 1-20
	MaxConcurrency int `yaml:"max_concurrency"`

	// 性能优化开关：启用后会自动计算最优并发度
	PerformanceTuning bool `yaml:"performance_tuning"`
}

// GetFourDimensionConfig 获取四维配置，如果未设置则返回默认值
func (c *Config) GetFourDimensionConfig() FourDimensionConfig {
	if c.Server == nil || c.Server.FourDimension == nil {
		return defaultFourDimensionConfig()
	}
	return *c.Server.FourDimension
}

// defaultFourDimensionConfig 返回四维配置默认值
func defaultFourDimensionConfig() FourDimensionConfig {
	return FourDimensionConfig{
		ConcurrencyMode:   "auto",
		MaxConcurrency:    20,
		PerformanceTuning: true,
	}
}

// CalculateConcurrency 根据四维配置计算各维度并发度
// 返回：账号并发度、产品并发度、区域并发度、总并发度
func CalculateConcurrency(fd FourDimensionConfig, accountCount int) (accountConc, productConc, regionConc, totalConc int) {
	// 根据并发模式或账号数选择预设
	var (
		accConc  int
		prodConc int
		regConc  int
	)

	switch fd.ConcurrencyMode {
	case "conservative":
		// 保守模式：4×1×1=4
		accConc, prodConc, regConc = 4, 1, 1
	case "standard":
		// 标准模式：4×2×2=16
		accConc, prodConc, regConc = 4, 2, 2
	case "aggressive":
		// 激进模式：4×3×4=48
		accConc, prodConc, regConc = 4, 3, 4
	case "auto":
		// 自动模式：根据账号数自动选择
		if accountCount <= 10 {
			accConc, prodConc, regConc = 4, 1, 1
		} else if accountCount <= 50 {
			accConc, prodConc, regConc = 4, 2, 2
		} else {
			accConc, prodConc, regConc = 4, 3, 4
		}
	default:
		// 默认使用保守模式
		accConc, prodConc, regConc = 4, 1, 1
	}

	// 性能优化：启用后根据账号数动态调整
	if fd.PerformanceTuning && fd.ConcurrencyMode == "auto" {
		// 账号数较少时可以降低账号并发度，提高产品/区域并发度
		if accountCount <= 3 {
			accConc, prodConc, regConc = 2, 2, 2
		} else if accountCount <= 5 {
			accConc, prodConc, regConc = 2, 2, 2
		} else if accountCount <= 10 {
			accConc, prodConc, regConc = 3, 1, 2
		} else if accountCount <= 30 {
			accConc, prodConc, regConc = 3, 1, 2
		}
	}

	// 计算总并发度
	calculatedTotal := accConc * prodConc * regConc

	// 验证总并发度不超过 max_concurrency
	if calculatedTotal > fd.MaxConcurrency {
		// 自动调整：逐步降低各维度并发度，直到满足限制
		// 优先级：账号并发度 > 产品并发度 > 区域并发度
		for calculatedTotal > fd.MaxConcurrency {
			if accConc > 1 {
				accConc--
			} else if prodConc > 1 {
				prodConc--
			} else if regConc > 1 {
				regConc--
			} else {
				// 所有维度都已降至最小，无法继续降低
				break
			}
			calculatedTotal = accConc * prodConc * regConc
		}
	}

	return accConc, prodConc, regConc, calculatedTotal
}
