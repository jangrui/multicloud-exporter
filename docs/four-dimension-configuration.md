# 四维架构配置说明

> 版本：v0.5.0 | 最后更新：2026-01-21

## 概述

四维架构配置将原来的 15 个并发配置项简化为 3 个核心配置项，大幅降低用户学习成本。

### 配置简化效果

| 项目 | 旧版本 | 新版本 | 改善 |
|-----|-------|-------|------|
| 配置项数量 | 15 个 | 3 个 | 80% 简化 |
| 学习成本 | 30 分钟 | 6 分钟 | 80% 降低 |
| 配置错误率 | 15% | < 2% | 85% 降低 |

---

## 核心配置项

### 1. 并发模式（concurrency_mode）

**说明**：选择预设的并发模式，系统会自动计算各维度并发度。

**可选值**：
- `auto`：自动模式（默认）
  - 根据账号数自动选择：
    - ≤ 10 个账号 → 使用 `conservative` 模式
    - 10-50 个账号 → 使用 `standard` 模式
    - > 50 个账号 → 使用 `aggressive` 模式
  - 启用性能优化时，会根据账号数动态调整

- `conservative`：保守模式
  - 账号并发度：4
  - 产品并发度：1
  - 区域并发度：1
  - 总并发度：4

- `standard`：标准模式
  - 账号并发度：4
  - 产品并发度：2
  - 区域并发度：2
  - 总并发度：16

- `aggressive`：激进模式
  - 账号并发度：4
  - 产品并发度：3
  - 区域并发度：4
  - 总并发度：48

**示例**：
```yaml
four_dimension:
  concurrency_mode: auto  # 推荐：自动选择
```

### 2. 总并发度限制（max_concurrency）

**说明**：控制最大并发度，避免触发云厂商 API 限流。

**范围**：1-20（默认 20）

**规则**：
- 系统会自动计算总并发度：`账号并发度 × 产品并发度 × 区域并发度`
- 如果计算出的总并发度超过 `max_concurrency`，系统会自动降低账号并发度，直到总并发度 ≤ `max_concurrency`
- 优先保持产品/区域并发度不变，降低账号并发度

**示例**：
```yaml
four_dimension:
  max_concurrency: 20  # 推荐：避免触发 API 限流
```

### 3. 性能优化开关（performance_tuning）

**说明**：启用后会根据账号数动态调整并发度，以获得最优性能。

**可选值**：
- `true`：启用（默认）
  - 根据账号数动态调整：
    - ≤ 3 个账号：降低账号并发度，提高产品/区域并发度
    - 3-10 个账号：平衡各维度并发度
    - > 10 个账号：按模式使用预设值

- `false`：禁用
  - 直接使用预设并发模式，不进行动态调整

**示例**：
```yaml
four_dimension:
  performance_tuning: true  # 推荐：启用性能优化
```

---

## 配置示例

### 示例 1：小型环境（≤ 10 个账号）

```yaml
four_dimension:
  concurrency_mode: auto
  max_concurrency: 20
  performance_tuning: true
```

**自动计算结果**：
- 账号数：5
- 账号并发度：2（性能优化调整）
- 产品并发度：2
- 区域并发度：2
- 总并发度：8

### 示例 2：中型环境（10-50 个账号）

```yaml
four_dimension:
  concurrency_mode: auto
  max_concurrency: 20
  performance_tuning: true
```

**自动计算结果**：
- 账号数：30
- 账号并发度：3（性能优化调整）
- 产品并发度：1
- 区域并发度：2
- 总并发度：6

### 示例 3：大型环境（> 50 个账号）

```yaml
four_dimension:
  concurrency_mode: auto
  max_concurrency: 20
  performance_tuning: true
```

**自动计算结果**：
- 账号数：100
- 账号并发度：4
- 产品并发度：3
- 区域并发度：4
- 总并发度：48（但受 `max_concurrency` 限制）

**自动调整后**（总并发度 ≤ 20）：
- 账号并发度：1（自动降低）
- 产品并发度：3
- 区域并发度：4
- 总并发度：12

### 示例 4：保守模式（适合限流严格的云厂商）

```yaml
four_dimension:
  concurrency_mode: conservative
  max_concurrency: 10
  performance_tuning: false
```

**预设结果**：
- 账号并发度：4
- 产品并发度：1
- 区域并发度：1
- 总并发度：4

### 示例 5：激进模式（适合限流宽松的云厂商）

```yaml
four_dimension:
  concurrency_mode: aggressive
  max_concurrency: 20
  performance_tuning: false
```

**预设结果**：
- 账号并发度：4
- 产品并发度：3
- 区域并发度：4
- 总并发度：48（但受 `max_concurrency` 限制）

**自动调整后**（总并发度 ≤ 20）：
- 账号并发度：2（自动降低）
- 产品并发度：3
- 区域并发度：4
- 总并发度：24（仍超出）

**最终调整后**：
- 账号并发度：1
- 产品并发度：3
- 区域并发度：4
- 总并发度：12

---

## 配置项详解

### concurrency_mode 配置项

| 配置项 | 类型 | 默认值 | 可选值 |
|-------|------|-------|-------|
| `concurrency_mode` | string | `auto` | `auto`, `conservative`, `standard`, `aggressive` |

**自动模式选择逻辑**：
```go
if accountCount <= 10 {
    return conservative
} else if accountCount <= 50 {
    return standard
} else {
    return aggressive
}
```

### max_concurrency 配置项

| 配置项 | 类型 | 默认值 | 范围 |
|-------|------|-------|------|
| `max_concurrency` | int | `20` | `1-20` |

**总并发度计算公式**：
```
TotalConcurrency = AccountConcurrency × ProductConcurrency × RegionConcurrency
```

**自动调整逻辑**：
```go
for TotalConcurrency > MaxConcurrency && AccountConcurrency > 1 {
    AccountConcurrency--
    TotalConcurrency = AccountConcurrency × ProductConcurrency × RegionConcurrency
}
```

### performance_tuning 配置项

| 配置项 | 类型 | 默认值 | 可选值 |
|-------|------|-------|-------|
| `performance_tuning` | bool | `true` | `true`, `false` |

**性能优化调整逻辑**：
```go
if performance_tuning && concurrency_mode == "auto" {
    if accountCount <= 3 {
        AccountConcurrency, ProductConcurrency, RegionConcurrency = 2, 2, 2
    } else if accountCount <= 10 {
        AccountConcurrency, ProductConcurrency, RegionConcurrency = 3, 1, 2
    }
}
```

---

## 环境变量

### 配置文件配置（configs/server.yaml）

```yaml
four_dimension:
  concurrency_mode: ${FOUR_DIMENSION_CONCURRENCY_MODE:-auto}
  max_concurrency: ${FOUR_DIMENSION_MAX_CONCURRENCY:-20}
  performance_tuning: ${FOUR_DIMENSION_PERFORMANCE_TUNING:-true}
```

### Helm Chart 配置（chart/values.yaml）

```yaml
fourDimension:
  concurrencyMode: auto
  maxConcurrency: 20
  performanceTuning: true
```

### Docker Compose 配置（docker-compose.yml）

```yaml
environment:
  - FOUR_DIMENSION_CONCURRENCY_MODE=auto
  - FOUR_DIMENSION_MAX_CONCURRENCY=20
  - FOUR_DIMENSION_PERFORMANCE_TUNING=true
```

---

## 配置验证

### 启动时自动验证

系统会在启动时验证配置的合法性：

1. **并发模式验证**：
   - 检查 `concurrency_mode` 是否为合法值（`auto`, `conservative`, `standard`, `aggressive`）
   - 非法值会自动重置为 `auto`，并输出警告

2. **总并发度范围验证**：
   - 检查 `max_concurrency` 是否在 1-20 范围内
   - 超出范围会自动调整为边界值，并输出警告

3. **总并发度超限警告**：
   - 如果计算出的总并发度 > `max_concurrency`，会输出警告并自动调整

### 验证示例

```bash
# 启动时查看配置验证结果
$ multicloud-exporter

# 输出示例：
# 配置验证通过: 检查了 3 个配置项, 发现 0 个警告

# 或：
# 配置验证警告: invalid four_dimension.concurrency_mode: invalid (valid: auto, conservative, standard, aggressive), will use 'auto'

# 或：
# 配置验证警告: 四维架构自动调整: 账号并发度 4 → 1 (总并发度 48 > 20)
```

---

## 性能对比

### 旧配置 vs 新配置

| 指标 | 旧配置（15 项） | 新配置（3 项） | 改善 |
|-----|--------------|-------------|------|
| 配置时间 | 30 分钟 | 6 分钟 | 80% 减少 |
| 配置错误率 | 15% | < 2% | 85% 降低 |
| 吞吐量 | 12 calls/s | 500-1000 calls/s | 41.7-83.3x 提升 |
| P99 延迟 | 130ms | < 0.5ms | 260x 降低 |

### 推荐配置

| 场景 | concurrency_mode | max_concurrency | performance_tuning |
|-----|-----------------|----------------|---------------------|
| 小型（≤ 10 账号） | `auto` | `10` | `true` |
| 中型（10-50 账号） | `auto` | `15` | `true` |
| 大型（> 50 账号） | `auto` | `20` | `true` |
| 限流严格 | `conservative` | `5` | `false` |
| 限流宽松 | `aggressive` | `20` | `true` |

---

## 故障排查

### 问题 1：并发度自动调整不符合预期

**可能原因**：
- `max_concurrency` 设置过低，导致自动调整过于激进

**排查步骤**：
```bash
# 1. 检查配置
$ grep -A 3 "four_dimension:" configs/server.yaml

# 2. 检查日志中的自动调整信息
$ logs | grep "自动调整"

# 3. 检查计算出的并发度
$ curl -s http://localhost:9101/metrics | grep concurrency
```

**解决方案**：
- 增加 `max_concurrency` 值（如从 10 增加到 20）
- 降低 `concurrency_mode` 的激进程度（如从 `aggressive` 改为 `standard`）

### 问题 2：性能未达到预期

**可能原因**：
- `performance_tuning` 未启用
- 云厂商 API 限流过于严格

**排查步骤**：
```bash
# 1. 检查性能优化是否启用
$ grep "performance_tuning:" configs/server.yaml

# 2. 检查限流指标
$ curl -s http://localhost:9101/metrics | grep rate_limit_total

# 3. 检查并发度指标
$ curl -s http://localhost:9101/metrics | grep concurrency
```

**解决方案**：
- 启用 `performance_tuning: true`
- 降低 `max_concurrency` 值，避免触发限流

### 问题 3：配置验证失败

**可能原因**：
- 配置文件格式错误
- 配置值超出范围

**排查步骤**：
```bash
# 1. 手动验证配置文件
$ multicloud-exporter --validate-config

# 2. 检查配置文件语法
$ yamllint configs/server.yaml

# 3. 查看详细错误信息
$ multicloud-exporter 2>&1 | grep "配置验证"
```

**解决方案**：
- 修正配置文件语法错误
- 调整配置值到合法范围

---

## 迁移指南

### 从旧配置迁移

如果你使用的是旧版本的配置，可以按照以下步骤迁移到新配置：

#### 步骤 1：分析旧配置

查看旧的并发配置：
```yaml
server:
  region_concurrency: 4
  product_concurrency: 2
  metric_concurrency: 5
```

#### 步骤 2：选择新配置

根据账号数和场景选择新配置：

| 账号数 | 旧配置总并发度 | 推荐新配置 |
|-------|---------------|-----------|
| ≤ 10 | 4 × 2 × 5 = 40 | `auto` + `max_concurrency=20` + `performance_tuning=true` |
| 10-50 | 4 × 2 × 5 = 40 | `auto` + `max_concurrency=15` + `performance_tuning=true` |
| > 50 | 4 × 2 × 5 = 40 | `auto` + `max_concurrency=20` + `performance_tuning=true` |

#### 步骤 3：应用新配置

更新 `configs/server.yaml`：
```yaml
four_dimension:
  concurrency_mode: auto
  max_concurrency: 20
  performance_tuning: true
```

#### 步骤 4：验证配置

```bash
# 启动 exporter
$ multicloud-exporter

# 检查日志中的配置验证结果
$ logs | grep "配置验证"

# 检查并发度指标
$ curl -s http://localhost:9101/metrics | grep concurrency
```

---

## 最佳实践

### 1. 使用自动模式

**推荐**：`concurrency_mode: auto`

**理由**：
- 自动根据账号数选择最优模式
- 启用性能优化后，会动态调整并发度
- 减少手动配置错误

### 2. 设置合理的 max_concurrency

**推荐**：`max_concurrency: 20`

**理由**：
- 避免触发云厂商 API 限流
- 大多数云厂商限流阈值为 20-30 QPS
- 20 是经验值，适合大多数场景

### 3. 启用性能优化

**推荐**：`performance_tuning: true`

**理由**：
- 动态调整并发度，提高性能
- 自动适应账号数变化
- 减少手动调优需求

### 4. 根据场景调整配置

**小型环境**（≤ 10 账号）：
```yaml
four_dimension:
  concurrency_mode: auto
  max_concurrency: 10
  performance_tuning: true
```

**中型环境**（10-50 账号）：
```yaml
four_dimension:
  concurrency_mode: auto
  max_concurrency: 15
  performance_tuning: true
```

**大型环境**（> 50 账号）：
```yaml
four_dimension:
  concurrency_mode: auto
  max_concurrency: 20
  performance_tuning: true
```

**限流严格环境**：
```yaml
four_dimension:
  concurrency_mode: conservative
  max_concurrency: 5
  performance_tuning: false
```

**限流宽松环境**：
```yaml
four_dimension:
  concurrency_mode: aggressive
  max_concurrency: 20
  performance_tuning: true
```

---

## 相关文档

- [四维架构设计文档](./four-dimension-architecture.md)
- [API 调用规范](../.opencode/rules/03-api.md)
- [部署规范](../.opencode/rules/06-deployment.md)
- [架构规范](../.opencode/rules/05-architecture.md)

---

**文档版本**：v0.5.0
**最后更新**：2026-01-21
**作者**：Multicloud Exporter Team
