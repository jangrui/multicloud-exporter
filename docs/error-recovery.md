# 错误恢复机制使用指南

## 概述

本项目提供了完整的错误恢复机制，包括：

1. **降级策略**：自动禁用失败资源，避免重复调用失败的 API
2. **自动恢复**：定期尝试恢复被禁用的资源
3. **热加载**：支持通过 SIGHUP 信号或 HTTP API 重新加载配置

## 降级策略

### 工作原理

当资源（账号、区域、产品）连续失败达到阈值时，系统会自动将其标记为禁用状态，停止采集该资源的数据，避免浪费 API 调用。

### 配置项

```go
type DegradationConfig struct {
    MaxFailures      int           // 最大失败次数，默认 3
    FailureWindow     time.Duration // 失败时间窗口，默认 5 分钟
    RecoveryInterval  time.Duration // 自动恢复检查间隔，默认 10 分钟
    RecoveryTimeout   time.Duration // 恢复尝试超时时间，默认 30 秒
}
```

### 使用示例

```go
import (
    "multicloud-exporter/internal/providers/common"
)

// 创建降级管理器
cfg := common.DefaultDegradationConfig()
cfg.MaxFailures = 5 // 修改最大失败次数为 5
degradeMgr := common.NewManager(cfg, logger)

// 记录失败
key := "aliyun:123456:cn-beijing"
if err := someAPI() != nil {
    disabled := degradeMgr.RecordFailure(key, common.ResourceTypeAccount, err.Error())
    if disabled {
        log.Warn("资源已降级", "key", key)
    }
}

// 检查资源是否被禁用
if degradeMgr.IsDisabled(key, common.ResourceTypeAccount) {
    return nil, fmt.Errorf("资源已禁用")
}

// 记录成功（用于恢复）
if err := someAPI() == nil {
    degradeMgr.RecordSuccess(key, common.ResourceTypeAccount)
}
```

## 自动恢复

### 工作原理

降级管理器会定期尝试恢复被禁用的资源。如果恢复成功，资源将被重新启用。

### 启动自动恢复

```go
// 定义恢复函数
recoverFunc := func(key string, rtype common.ResourceType) bool {
    // 尝试恢复资源
    err := tryRecoverResource(key, rtype)
    return err == nil
}

// 启动自动恢复协程
degradeMgr.StartAutoRecovery(recoverFunc, shutdownCtx)
```

## 热加载

### SIGHUP 信号

发送 SIGHUP 信号可以触发热加载，重新加载配置文件：

```bash
# 查找进程 PID
ps aux | grep multicloud-exporter

# 发送 SIGHUP 信号
kill -HUP <PID>
```

### HTTP API

通过 HTTP API 也可以触发热加载：

```bash
# 触发热加载
curl -X POST -u admin:admin http://localhost:8080/api/v1/reload
```

### 热加载范围

热加载会重新加载以下内容：

1. 配置文件（`configs/server.yaml`）
2. 指标映射文件（`configs/mappings/*.yaml`）
3. 账号配置（`configs/accounts.yaml`）

### 注意事项

1. 热加载不会重启服务，但会重新初始化采集器和发现管理器
2. 热加载期间可能会有短暂的采集停顿
3. 建议在业务低峰期进行热加载

## 监控指标

降级管理器不直接暴露指标，但可以通过日志和 API 查询资源状态。

## 故障排查

### 资源被禁用后不恢复

1. 检查日志中是否有"尝试恢复资源"的记录
2. 检查恢复函数是否正确实现
3. 检查网络和 API 访问是否正常
4. 检查恢复超时配置是否合理

### 热加载失败

1. 检查配置文件语法是否正确
2. 检查文件权限是否正确
3. 检查日志中的具体错误信息
4. 检查认证配置是否正确

## 最佳实践

1. **合理设置失败阈值**：根据 API 的稳定性和重要程度设置合适的失败次数
2. **监控降级状态**：定期检查哪些资源被禁用，及时排查问题
3. **及时修复问题**：资源被禁用后应尽快排查并修复根本原因
4. **测试恢复功能**：在测试环境中验证自动恢复功能
5. **谨慎使用热加载**：确保配置文件正确后再触发热加载

## 示例：完整的使用流程

```go
package main

import (
    "context"
    "multicloud-exporter/internal/providers/common"
    "multicloud-exporter/internal/logger"
)

func main() {
    // 初始化日志
    logger.Init()

    // 创建降级管理器
    cfg := common.DefaultDegradationConfig()
    degradeMgr := common.NewManager(cfg, logger.Get())

    // 定义恢复函数
    recoverFunc := func(key string, rtype common.ResourceType) bool {
        logger.Info("尝试恢复资源", "key", key, "type", rtype)

        // 根据资源类型执行不同的恢复逻辑
        switch rtype {
        case common.ResourceTypeAccount:
            return tryRecoverAccount(key)
        case common.ResourceTypeRegion:
            return tryRecoverRegion(key)
        case common.ResourceTypeProduct:
            return tryRecoverProduct(key)
        }

        return false
    }

    // 启动自动恢复
    shutdownCtx, shutdownCancel := context.WithCancel(context.Background())
    defer shutdownCancel()

    go degradeMgr.StartAutoRecovery(recoverFunc, shutdownCtx)

    // 采集逻辑
    for {
        collectMetrics(degradeMgr)
        time.Sleep(60 * time.Second)
    }
}

func collectMetrics(degradeMgr *common.Manager) {
    key := "aliyun:123456:cn-beijing"

    // 检查资源是否被禁用
    if degradeMgr.IsDisabled(key, common.ResourceTypeAccount) {
        logger.Warn("资源已禁用，跳过采集", "key", key)
        return
    }

    // 尝试采集
    err := doCollection(key)
    if err != nil {
        disabled := degradeMgr.RecordFailure(key, common.ResourceTypeAccount, err.Error())
        if disabled {
            logger.Warn("资源已被降级", "key", key, "error", err)
        }
    } else {
        degradeMgr.RecordSuccess(key, common.ResourceTypeAccount)
    }
}
```
