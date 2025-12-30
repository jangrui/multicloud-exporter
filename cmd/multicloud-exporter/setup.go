package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/logger"
)

// setupConfig 加载并验证配置
func setupConfig() (*config.Config, error) {
	cfg, err := config.LoadConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// 初始化日志系统
	if cfg.Server != nil && cfg.Server.Log != nil {
		logger.Init(cfg.Server.Log)
	}

	return cfg, nil
}

// logAccountStats 记录账号统计信息
func logAccountStats(cfg *config.Config) {
	totalAccounts := 0
	accountsByProvider := make(map[string]int)
	for provider, accounts := range cfg.AccountsByProvider {
		count := len(accounts)
		totalAccounts += count
		accountsByProvider[provider] = count
	}

	// 构建账号统计信息
	var accountInfo strings.Builder
	if len(accountsByProvider) > 0 {
		accountInfo.WriteString(" (")
		providers := make([]string, 0, len(accountsByProvider))
		for provider := range accountsByProvider {
			providers = append(providers, provider)
		}
		sort.Strings(providers)
		for i, provider := range providers {
			if i > 0 {
				accountInfo.WriteString(", ")
			}
			accountInfo.WriteString(fmt.Sprintf("%s=%d", provider, accountsByProvider[provider]))
		}
		accountInfo.WriteString(")")
	}

	logger.Log.Infof("配置加载完成，账号配置集合 sizes: accounts=%d%s", totalAccounts, accountInfo.String())
}

// setupMetricMappings 加载指标映射配置
func setupMetricMappings(cfg *config.Config) {
	// 优先从环境变量 MAPPING_PATH 加载
	if mappingPath := os.Getenv("MAPPING_PATH"); mappingPath != "" {
		loadMappingsFromPath(mappingPath)
		return
	}

	// 否则从默认位置加载
	loadDefaultMappings(cfg)
}

// loadMappingsFromPath 从指定路径加载指标映射
func loadMappingsFromPath(mappingPath string) {
	paths := strings.Split(mappingPath, ",")
	loaded := 0
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		fi, err := os.Stat(p)
		if err != nil {
			logger.Log.Warnf("映射路径未找到: %s (%v)", p, err)
			continue
		}
		if fi.IsDir() {
			files, err := filepath.Glob(filepath.Join(p, "*.yaml"))
			if err != nil || len(files) == 0 {
				logger.Log.Warnf("目录中无映射文件: %s", p)
				continue
			}
			for _, f := range files {
				if loadSingleMapping(f) {
					loaded++
				}
			}
		} else {
			if loadSingleMapping(p) {
				loaded++
			}
		}
	}
	if loaded == 0 {
		logger.Log.Warnf("未从 MAPPING_PATH=%s 加载任何指标映射", mappingPath)
	} else {
		logger.Log.Infof("已加载指标映射，数量=%d 来源=MAPPING_PATH", loaded)
	}
}

// loadDefaultMappings 从默认位置加载指标映射
func loadDefaultMappings(cfg *config.Config) {
	mappingDir := "configs/mappings"
	if err := config.ValidateAllMappings(mappingDir); err != nil {
		logger.Log.Warnf("指标映射验证发现问题:\n%v", err)
	} else {
		logger.Log.Infof("指标映射验证通过，目录=%s", mappingDir)
	}

	files, err := filepath.Glob(filepath.Join(mappingDir, "*.yaml"))
	if err == nil && len(files) > 0 {
		for _, f := range files {
			loadSingleMapping(f)
		}
		return
	}

	// Fallback to legacy single file check
	defaultPath := "configs/mappings/clb.metrics.yaml"
	if _, err := os.Stat(defaultPath); err == nil {
		loadSingleMapping(defaultPath)
	}
}

// loadSingleMapping 加载单个映射文件
func loadSingleMapping(filePath string) bool {
	if err := config.ValidateMappingStructure(filePath); err != nil {
		logger.Log.Warnf("指标映射验证失败，文件=%s 错误=%v", filePath, err)
		return false
	}
	if err := config.LoadMetricMappings(filePath); err != nil {
		logger.Log.Warnf("加载指标映射失败，文件=%s 错误=%v", filePath, err)
		return false
	}
	logger.Log.Infof("已加载指标映射，文件=%s", filePath)
	return true
}
