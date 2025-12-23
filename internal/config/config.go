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

	Server     *ServerConf `yaml:"server"`
	ServerConf *ServerConf `yaml:"serverconf"`
	RemoteProm *RemoteProm `yaml:"remote_prom"`
	Credential *Credential `yaml:"credential"`
	DataTag    []DataTag   `yaml:"datatag"`
	Estimation *EstimationConf `yaml:"estimation"`

	AccountsByProvider map[string][]CloudAccount `yaml:"accounts"`

	ProductsByProvider map[string][]Product `yaml:"products"`
}

// DefaultResourceDimMapping 返回默认的资源维度映射配置
func DefaultResourceDimMapping() map[string][]string {
	return map[string][]string{
		// Aliyun
		"aliyun.acs_ecs_dashboard":     {"InstanceId", "instanceId", "instance_id"},
		"aliyun.acs_slb_dashboard":     {"InstanceId", "instanceId", "instance_id", "groupId", "group_id"},
		"aliyun.acs_bandwidth_package": {"BandwidthPackageId", "bandwidthPackageId", "sharebandwidthpackages"},
		"aliyun.acs_oss_dashboard":     {"BucketName", "bucketName", "bucket_name"},
		"aliyun.acs_alb":               {"loadBalancerId", "LoadBalancerId", "serverGroupId", "listenerId", "vip"},
		"aliyun.acs_nlb":               {"InstanceId", "instanceId", "instance_id", "listenerId", "vip"},
		"aliyun.acs_gwlb":              {"instanceId", "InstanceId", "instance_id"},
		// Tencent
		"tencent.QCE/CVM":  {"InstanceId"},
		"tencent.QCE/LB":   {"LoadBalancerId", "vip"},
		"tencent.qce/gwlb": {"gwLoadBalancerId", "GwLoadBalancerId"},
		// AWS (Example)
		"aws.AWS/EC2": {"InstanceId"},
		"aws.AWS/ELB": {"LoadBalancerName"},
	}
}

// LoadConfig 从环境变量加载拆分配置文件
func LoadConfig() (*Config, error) {
	var cfg Config

	// 加载 server.yaml
	serverPath := os.Getenv("SERVER_PATH")
	if serverPath == "" {
		// 默认回退路径：优先容器挂载路径，其次本地开发路径
		for _, p := range []string{"/app/configs/server.yaml", "./configs/server.yaml"} {
			if _, err := os.Stat(p); err == nil {
				serverPath = p
				break
			}
		}
	}
	if serverPath != "" {
		data, err := os.ReadFile(serverPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read server config: %v", err)
		}
		expanded := expandEnv(string(data))
		var s struct {
			Server *ServerConf `yaml:"server"`
		}
		if err := yaml.Unmarshal([]byte(expanded), &s); err != nil {
			return nil, fmt.Errorf("failed to parse server config: %v", err)
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
	if accountsPath := os.Getenv("ACCOUNTS_PATH"); accountsPath != "" {
		accData, err := os.ReadFile(accountsPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read accounts: %v", err)
		}
		accExpanded := expandEnv(string(accData))
		var accCfg struct {
			AccountsByProvider map[string][]CloudAccount `yaml:"accounts"`
		}
		if err := yaml.Unmarshal([]byte(accExpanded), &accCfg); err != nil {
			return nil, fmt.Errorf("failed to parse accounts: %v", err)
		}
		if accCfg.AccountsByProvider != nil {
			cfg.AccountsByProvider = accCfg.AccountsByProvider
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
	// 区域级并发：同一账号下并行采集的地域数量，建议 1-8。
	RegionConcurrency int `yaml:"region_concurrency"`
	// 指标级并发：同一地域、同一产品下并行处理的指标批次数，建议 1-10。
	MetricConcurrency int `yaml:"metric_concurrency"`
	// 产品级并发：同一地域下并行处理的命名空间（云产品）数量，建议 1-4。
	ProductConcurrency int `yaml:"product_concurrency"`

	// ResourceDimMapping 定义各云厂商、各产品（Namespace）的资源维度校验规则。
	// Key 为 "provider.namespace"，例如 "aliyun.acs_ecs_dashboard"。
	// Value 为该产品必须包含的维度键列表（任一匹配即可），例如 ["InstanceId", "instance_id"]。
	ResourceDimMapping map[string][]string `yaml:"resource_dim_mapping"`
	AdminAuthEnabled   bool                `yaml:"admin_auth_enabled"`
	AdminAuth          []BasicAuth         `yaml:"admin_auth"`
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
