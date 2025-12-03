// 配置包：定义账号与全局配置结构，提供 YAML 加载能力
package config

import (
	"io/ioutil"
	"log"
	"os"

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

// Config 汇总所有云账号配置
type Config struct {
	Server     *ServerConf `yaml:"server"`
	ServerConf *ServerConf `yaml:"serverconf"`
	RemoteProm *RemoteProm `yaml:"remote_prom"`
	Credential *Credential `yaml:"credential"`
	DataTag    []DataTag   `yaml:"datatag"`

	AccountsByProvider       map[string][]CloudAccount `yaml:"accounts"`
	AccountsByProviderLegacy map[string][]CloudAccount `yaml:"accounts_by_provider"`
	AccountsList             []CloudAccount            `yaml:"accounts_list"`

	ProductsByProvider       map[string][]Product `yaml:"products"`
	ProductsByProviderLegacy map[string][]Product `yaml:"products_by_provider"`
	ProductsList             []Product            `yaml:"products_list"`
}

// LoadConfig 从环境变量 CONFIG_PATH 指向的 YAML 文件加载配置
func LoadConfig() *Config {
	var cfg Config

	// 可选：兼容旧版单文件配置
	if configPath := os.Getenv("CONFIG_PATH"); configPath != "" {
		if data, err := ioutil.ReadFile(configPath); err == nil {
			expanded := os.ExpandEnv(string(data))
			_ = yaml.Unmarshal([]byte(expanded), &cfg)
		} else {
			log.Printf("CONFIG_PATH not loaded: %v", err)
		}
	}

	// 新版拆分：server.yaml
	if serverPath := os.Getenv("SERVER_PATH"); serverPath != "" {
		data, err := ioutil.ReadFile(serverPath)
		if err != nil {
			log.Fatalf("Failed to read server config: %v", err)
		}
		expanded := os.ExpandEnv(string(data))
		var s struct {
			Server *ServerConf `yaml:"server"`
		}
		if err := yaml.Unmarshal([]byte(expanded), &s); err != nil {
			log.Fatalf("Failed to parse server config: %v", err)
		}
		if s.Server != nil {
			cfg.Server = s.Server
			cfg.ServerConf = s.Server
		}
	}

	// 新版拆分：products.yaml
	if productsPath := os.Getenv("PRODUCTS_PATH"); productsPath != "" {
		data, err := ioutil.ReadFile(productsPath)
		if err != nil {
			log.Fatalf("Failed to read products config: %v", err)
		}
		expanded := os.ExpandEnv(string(data))
		var p struct {
			ProductsByProvider       map[string][]Product `yaml:"products"`
			ProductsByProviderLegacy map[string][]Product `yaml:"products_by_provider"`
			ProductsList             []Product            `yaml:"products_list"`
		}
		if err := yaml.Unmarshal([]byte(expanded), &p); err != nil {
			log.Fatalf("Failed to parse products config: %v", err)
		}
		if p.ProductsByProvider != nil {
			cfg.ProductsByProvider = p.ProductsByProvider
		}
		if p.ProductsByProviderLegacy != nil {
			cfg.ProductsByProviderLegacy = p.ProductsByProviderLegacy
		}
		if len(p.ProductsList) > 0 {
			cfg.ProductsList = p.ProductsList
		}
	}

	// 新版拆分：accounts.yaml
	if accountsPath := os.Getenv("ACCOUNTS_PATH"); accountsPath != "" {
		accData, err := ioutil.ReadFile(accountsPath)
		if err != nil {
			log.Fatalf("Failed to read accounts: %v", err)
		}
		accExpanded := os.ExpandEnv(string(accData))
		var accCfg struct {
			AccountsByProvider       map[string][]CloudAccount `yaml:"accounts"`
			AccountsByProviderLegacy map[string][]CloudAccount `yaml:"accounts_by_provider"`
			AccountsList             []CloudAccount            `yaml:"accounts_list"`
		}
		if err := yaml.Unmarshal([]byte(accExpanded), &accCfg); err != nil {
			log.Fatalf("Failed to parse accounts: %v", err)
		}
		if accCfg.AccountsByProvider != nil {
			cfg.AccountsByProvider = accCfg.AccountsByProvider
		}
		if accCfg.AccountsByProviderLegacy != nil {
			cfg.AccountsByProviderLegacy = accCfg.AccountsByProviderLegacy
		}
		if len(accCfg.AccountsList) > 0 {
			cfg.AccountsList = accCfg.AccountsList
		}
	}

	// 账号文件中若包含环境占位符，将通过 env 展开（由容器 envFrom 注入）
	return &cfg
}

type ServerConf struct {
	ServiceEndpoint  string `yaml:"service_endpoint"`
	Port             int    `yaml:"port"`
	PageSize         int    `yaml:"page_size"`
	LogDest          int    `yaml:"log_dest"`
	LogDir           string `yaml:"log_dir"`
	LogLevel         string `yaml:"log_level"`
	HttpProxy        string `yaml:"http_proxy"`
	HttpsProxy       string `yaml:"https_proxy"`
	NoProxy          string `yaml:"no_proxy"`
	NoMeta           bool   `yaml:"no_meta"`
	NoSavepoint      bool   `yaml:"no_savepoint"`
	DiscoveryTTL     string `yaml:"discovery_ttl"`
	DiscoveryRefresh string `yaml:"discovery_refresh"`
	// 区域级并发：同一账号下并行采集的地域数量，建议 1-8。
	RegionConcurrency int `yaml:"region_concurrency"`
	// 指标级并发：同一地域、同一产品下并行处理的指标批次数，建议 1-10。
	MetricConcurrency int `yaml:"metric_concurrency"`
	// 产品级并发：同一地域下并行处理的命名空间（云产品）数量，建议 1-4。
	ProductConcurrency int `yaml:"product_concurrency"`
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
