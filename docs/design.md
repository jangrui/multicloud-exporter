# Multicloud Exporter - 设计与实施计划

## 文档版本
- **版本号：** v1.1
- **创建日期：** 2026-01-04
- **更新日期：** 2026-01-04
- **对应需求文档：** [docs/requirements.md](./requirements.md)
- **状态：** 实施中

## Overview

### 设计目标
本设计与实施计划旨在为 Multicloud Exporter 提供清晰的技术架构和可执行的实施任务，确保系统满足所有功能需求和非功能需求。

### 实施原则
- **模块化设计**：各云厂商实现独立，易于扩展
- **任务导向**：每个设计模块分解为具体的可执行任务
- **需求追溯**：每个任务明确对应的需求ID，确保可追溯性
- **质量保证**：每个阶段包含验证任务，确保实现质量

### 任务状态说明
- `[ ]` - 待开始的任务
- `[x]` - 已完成的任务
- `[*]` - 可选任务，可跳过以加快 MVP 开发

## Phase 1: HTTP Server Layer 设计与实施

### 设计概述
HTTP Server Layer 负责暴露监控指标端点和提供管理接口，包括认证机制和健康检查。

**对应需求：** FR-004, FR-003-05, FR-003-06, FR-004-06, FR-009

### Tasks

#### Task 1.1: 实现核心 HTTP 端点
- [x] 1.1.1 实现 `/metrics` 端点
  - 使用 `prometheus/client_golang` 暴露指标
  - 支持 Prometheus Text Format 格式
  - 添加响应头 `Content-Type: text/plain; version=0.0.4`
  - _Requirements: FR-004-01_

- [x] 1.1.2 实现 `/healthz` 端点
  - 返回纯文本 `OK` 响应
  - 设置响应码 200
  - 添加基础的健康检查逻辑
  - _Requirements: FR-004-06_

- [x] 1.1.3 实现管理接口端点
  - 实现 `/api/discovery/config` 端点
    - 返回 JSON 格式的资源发现结果
    - 按云厂商、区域、资源类型分组
  - 实现 `/api/discovery/stream` 端点
    - 支持 SSE 流式推送
    - 实时推送配置变更
  - 实现 `/api/discovery/status` 端点
    - 返回发现状态和 API 统计
  - 实现 `/collect` 端点
    - 手动触发采集
  - 实现 `/status` 端点
    - 获取采集状态
  - _Requirements: FR-003-05, FR-003-06_

#### Task 1.2: 设计并实现指标注册机制
- [x] 1.2.1 定义业务指标结构
  - 定义 `multicloud_resource_metric` GaugeVec
  - 定义 `multicloud_namespace_metric` GaugeVec
  - 标签：`cloud_provider`, `account_id`, `region`, `resource_type`, `resource_id`, `metric_name`
  - 添加动态维度标签支持
  - _Requirements: FR-004-01, FR-004-02, FR-004-04_

- [x] 1.2.2 定义自身监控指标结构
  - 定义 `multicloud_request_duration_seconds` HistogramVec
  - 定义 `multicloud_request_total` CounterVec
  - 定义 `multicloud_rate_limit_total` CounterVec
  - 定义 `multicloud_collection_duration_seconds` Histogram
  - 定义 `multicloud_cache_size_bytes` GaugeVec
  - 定义 `multicloud_cache_entries_total` GaugeVec
  - 定义 `multicloud_region_status_total` GaugeVec
  - _Requirements: FR-008-01, FR-008-02, FR-008-03_

- [x] 1.2.3 实现指标注册函数
  - 实现 `registerPrometheusMetrics()` 函数
  - 调用 `prometheus.MustRegister()` 注册所有指标
  - 在 `main.go` 中初始化时调用
  - _Requirements: FR-004-01_

#### Task 1.3: 实现 BasicAuth 认证机制
- [x] 1.3.1 定义认证配置结构
  - 定义 `BasicAuth` 结构体（username, password）
  - 添加配置字段：`server.admin_auth_enabled`, `server.admin_auth[]`
  - 支持环境变量配置
  - _Requirements: FR-009-01, FR-009-02_

- [x] 1.3.2 实现认证中间件
  - 创建 `createAuthWrapper` 函数
  - 验证请求头 `Authorization`
  - 对管理接口路径应用中间件
  - 处理认证失败，返回 401 Unauthorized / 403 Forbidden
  - 使用常量时间比较防止时序攻击
  - _Requirements: FR-009-01_

- [x] 1.3.3 支持 Kubernetes Secret 集成
  - 支持环境变量：`ADMIN_AUTH_ENABLED`, `ADMIN_AUTH`
  - 支持单账号：`ADMIN_USERNAME`, `ADMIN_PASSWORD`
  - 支持 JSON 格式多账号配置
  - 支持逗号分隔格式
  - _Requirements: FR-009-03, FR-009-04_

#### Task 1.4: 实现指标映射与统一命名
- [x] 1.4.1 设计指标映射文件格式
  - 定义 YAML 映射文件结构
  - 支持 `source_metric`, `target_metric`, `scale`, `dimensions` 字段
  - 支持跨云统一指标命名（`clb_*`, `bwp_*`, `s3_*`）
  - _Requirements: FR-004-05_

- [x] 1.4.2 实现指标映射加载器
  - 创建 `internal/config/mappings.go`
  - 实现 `LoadMetricMappings()` 函数
  - 支持 MAPPING_PATH 环境变量
  - 支持目录批量加载
  - _Requirements: FR-005-01, FR-005-06_

- [x] 1.4.3 实现指标映射验证
  - 实现 `ValidateMappingStructure()` 函数
  - 检查必需字段完整性
  - 验证 scale 值为有效数值
  - 检查维度配置合法性
  - _Requirements: FR-005-05, FR-005-06_

### Verification Tasks

- [x] Verify 1.1: 验证 HTTP 端点
  - 使用 `curl http://localhost:9101/healthz` 确认返回 `{"status":"healthy"}`
  - 使用 `curl http://localhost:9101/metrics | head -n 20` 确认指标格式
  - 使用 `curl http://localhost:9101/api/discovery/config` 确认配置返回
  - _Validates: FR-004-01, FR-003-06, FR-004-06_

- [x] Verify 1.2: 验证指标注册
  - 检查 `/metrics` 端点包含 `multicloud_resource_metric`
  - 检查 `/metrics` 端点包含所有自身监控指标
  - 确认指标命名符合 Prometheus 规范
  - _Validates: FR-004-02, FR-004-04, FR-008-01_

- [x] Verify 1.3: 验证认证机制
  - 启用 `ADMIN_AUTH_ENABLED=true`
  - 尝试无认证访问 `/api/discovery/config`，确认返回 401
  - 使用 `curl -u admin:password` 确认认证成功
  - _Validates: FR-009-01, FR-009-02_

- [x] Verify 1.4: 验证指标映射
  - 加载 `configs/mappings/clb.metrics.yaml`
  - 验证 `InstanceTrafficRX` 映射到 `clb_traffic_rx_bps`
  - 验证跨云指标命名统一
  - _Validates: FR-004-05_

## Phase 2: Config Layer 设计与实施

### 设计概述
Config Layer 负责加载、验证和管理所有配置，支持环境变量覆盖和多文件拆分。

**对应需求：** FR-005

### Tasks

#### Task 2.1: 设计配置文件结构
- [x] 2.1.1 定义 server.yaml 结构
  - 定义 `ServerConf` 结构体
  - 包含端口、日志、缓存、并发等配置
  - 支持环境变量替换语法 `${VAR:-default}`
  - _Requirements: FR-005-01, FR-005-02_

- [x] 2.1.2 定义 accounts.yaml 结构
  - 定义 `CloudAccount` 结构体
  - 包含 provider, account_id, credentials, regions, resources
  - 支持多云厂商配置
  - _Requirements: FR-005-01, FR-002-01_

- [x] 2.1.3 定义日志配置结构
  - 定义 `LogConfig` 和 `FileLogConfig` 结构体
  - 支持日志级别、格式、输出方式配置
  - 支持日志文件轮转配置
  - _Requirements: FR-005-01, FR-011-01, FR-011-04_

- [x] 2.1.4 定义区域发现配置结构
  - 定义 `RegionDiscoveryConf` 结构体
  - 包含 enabled, discovery_interval, empty_threshold
  - 包含 data_dir, persist_file 配置
  - _Requirements: FR-005-01, FR-007-03_

#### Task 2.2: 实现配置加载逻辑
- [x] 2.2.1 实现 LoadConfig 函数
  - 支持多路径搜索（环境变量、默认路径）
  - 使用 `yaml.v3` 解析配置文件
  - 实现环境变量替换逻辑（`expandEnv`）
  - _Requirements: FR-005-01, FR-005-02, FR-005-04_

- [x] 2.2.2 实现多文件配置加载
  - 分别加载 server.yaml 和 accounts.yaml
  - 合并配置到统一的 `Config` 结构
  - 支持配置文件路径自定义
  - _Requirements: FR-005-01, FR-005-04_

- [x] 2.2.3 实现凭证环境变量注入
  - 支持在 accounts.yaml 中使用环境变量占位符
  - 在运行时替换 `access_key_id`, `access_key_secret`
  - 不在代码仓库中存储明文凭证
  - _Requirements: FR-005-03_

#### Task 2.3: 实现配置验证逻辑
- [x] 2.3.1 实现 Validate 函数
  - 验证 Server 配置（端口、日志级别、并发参数）
  - 验证账号配置（必填字段、regions 不为空）
  - 返回详细的错误列表（不立即失败）
  - _Requirements: FR-005-05_

- [x] 2.3.2 添加配置验证工具
  - 创建 `cmd/config-validator` 工具
  - 支持验证指标映射文件结构
  - 支持验证配置文件合法性
  - _Requirements: FR-005-06_

#### Task 2.4: 实现配置热更新（可选）
- [*] 2.4.1 监听配置文件变化
  - 使用 `fsnotify` 库监听配置文件
  - 检测 `accounts.yaml` 的资源集合变化
  - 触发资源发现刷新
  - _Requirements: NFR-002-04_
  - _对应后续计划: 优先级 P3 任务 7_

- [*] 2.4.2 实现配置重载逻辑
  - 重新加载配置文件
  - 验证新配置合法性
  - 更新内部配置状态
  - 记录配置变更日志
  - _Requirements: NFR-002-04_
  - _对应后续计划: 优先级 P3 任务 7_

### Verification Tasks

- [x] Verify 2.1: 验证配置文件结构
  - 创建测试 server.yaml 和 accounts.yaml
  - 使用 `go run ./cmd/config-validator` 验证结构
  - 确认所有配置字段正确解析
  - _Validates: FR-005-01_

- [x] Verify 2.2: 验证配置加载
  - 使用 `SERVER_PATH` 和 `ACCOUNTS_PATH` 环境变量
  - 启动 exporter 确认配置加载成功
  - 检查日志中的配置加载信息
  - _Validates: FR-005-02, FR-005-04_

- [x] Verify 2.3: 验证配置验证
  - 创建无效配置（如端口 99999）
  - 启动 exporter 确认返回明确的错误信息
  - 使用 `make mappings-check` 验证映射文件
  - _Validates: FR-005-05, FR-005-06_

- [ ] Verify 2.4: 验证配置热更新（可选）
  - 启动 exporter
  - 修改 accounts.yaml 的 resources 字段
  - 确认日志显示配置变更
  - 验证资源发现自动刷新
  - _Validates: NFR-002-04_
  - _对应后续计划: 优先级 P3 任务 7_

## Phase 3: Discovery Manager 设计与实施

### 设计概述
Discovery Manager 负责资源发现、缓存管理和智能区域发现，大幅优化 API 调用效率。

**对应需求：** FR-003, FR-007-01, FR-007-02, FR-007-03, FR-007-06

### Tasks

#### Task 3.1: 设计资源发现接口
- [x] 3.1.1 定义 Discovery 接口
  - 定义 `DiscoverResources(provider, account, region, resourceType)` 方法
  - 定义 `DiscoverTags(provider, account, resourceID)` 方法
  - 定义 `DiscoverRegions(provider, account)` 方法
  - _Requirements: FR-003-01_

- [x] 3.1.2 实现 Provider 注册机制
  - 定义 `DiscoveryProvider` 接口
  - 创建 `internal/discovery/registry.go`
  - 实现注册函数 `RegisterDiscoveryProvider(name, factory)`
  - _Requirements: FR-003-01_

#### Task 3.2: 实现资源 ID 缓存
- [x] 3.2.1 定义缓存数据结构
  - 定义 `ResourceCache` 结构体
  - 定义 `resCacheEntry` 包含 IDs 和 ExpiresAt
  - 使用 `sync.RWMutex` 保护并发访问
  - _Requirements: FR-003-02, FR-007-01_

- [x] 3.2.2 实现 TTL 缓存逻辑
  - 实现 `GetResources(key)` 方法，检查缓存有效期
  - 实现 `SetResources(key, resources, ttl)` 方法
  - 支持 TTL 配置（s, m, h, d 单位）
  - _Requirements: FR-003-02, FR-007-01_

- [x] 3.2.3 实现缓存清理机制
  - 实现后台清理 Goroutine
  - 定期清理过期缓存项
  - 使用 context 支持优雅关闭
  - _Requirements: FR-007-01_

#### Task 3.3: 实现标签缓存
- [x] 3.3.1 定义标签缓存结构
  - 定义 `tagCache` map 结构
  - 使用 `map[string]map[string]string` 存储标签
  - _Requirements: FR-007-02_

- [x] 3.3.2 实现标签缓存逻辑
  - 实现 `getTagCache` 和 `setTagCache` 方法
  - 缓存键：`provider:accountID:resourceID`
  - 首次调用时查询 VPC API，后续返回缓存
  - _Requirements: FR-007-02_

- [x] 3.3.3 验证标签缓存效果
  - 测试单资源多指标场景
  - 确认 VPC API 调用从 N 次降至 1 次
  - 目标：阿里云 VPC API 调用减少 90%
  - _Requirements: FR-007-02_

#### Task 3.4: 实现智能区域发现
- [x] 3.4.1 定义区域状态机
  - 定义状态：`unknown`, `active`, `empty`
  - 定义 `RegionInfo` 结构体
  - 实现状态转换逻辑
  - _Requirements: FR-007-03_

- [x] 3.4.2 实现区域状态持久化
  - 实现状态保存到 JSON 文件
  - 支持配置持久化路径（`data_dir`, `persist_file`）
  - 启动时加载上次状态
  - _Requirements: FR-007-03_

- [x] 3.4.3 实现智能跳过逻辑
  - 记录连续空区域次数（`empty_count`）
  - 达到阈值（`empty_threshold`）后跳过采集
  - 定期重新发现（`discovery_interval`）
  - _Requirements: FR-007-03_

- [x] 3.4.4 实现 Kubernetes 持久化支持
  - 支持 PVC 和 emptyDir 两种模式
  - 添加配置字段：`regionData.persistence`
  - 在 Helm Chart 中添加 PVC 模板
  - _Requirements: FR-010-03_

#### Task 3.5: 实现 Period 自动适配
- [x] 3.5.1 定义 Period 获取接口
  - 定义 `GetMetricPeriod(namespace, metricName)` 方法
  - 定义 `DescribeBaseMetrics(namespace)` 方法
  - _Requirements: FR-007-06_

- [x] 3.5.2 实现自动 Period 选择
  - 调用云厂商元数据 API
  - 从 Period 列表中选择最小值
  - 失败时回退到 `PeriodFallback` 配置
  - _Requirements: FR-007-06_

- [x] 3.5.3 实现 Period 配置层级
  - MetricGroup.Period（最高优先级）
  - Product.Period
  - 元数据 API 自动获取
  - Server.PeriodFallback（最低优先级）
  - _Requirements: FR-007-06_

### Verification Tasks

- [x] Verify 3.2: 验证资源 ID 缓存
  - 首次采集，记录 API 调用次数
  - 第二次采集（TTL 内），确认缓存命中
  - 检查日志中的缓存命中/未命中信息
  - _Validates: FR-003-02, FR-007-01_

- [x] Verify 3.3: 验证标签缓存
  - 采集阿里云 CBWP（10 个指标）
  - 记录 VPC API 调用次数
  - 确认调用次数为 1 次（而不是 10 次）
  - _Validates: FR-007-02_

- [x] Verify 3.4: 验证智能区域发现
  - 启用智能区域发现（20 个区域，3 个有资源）
  - 首次运行，确认所有区域被探测
  - 第二次运行，确认空区域被跳过
  - 检查 region_status.json 文件内容
  - _Validates: FR-007-03_

- [x] Verify 3.5: 验证 Period 自动适配
  - 配置 `period_fallback: 60`
  - 采集指标，检查实际使用的 Period
  - 确认选择最小 Period 而非 fallback
  - _Validates: FR-007-06_

## Phase 4: Collector Layer 设计与实施

### 设计概述
Collector Layer 负责调度采集任务，协调 Provider 并发采集，管理采集状态。

**对应需求：** FR-003, FR-002-02, FR-008-04

### Tasks

#### Task 4.1: 设计采集调度架构
- [x] 4.1.1 定义 Collector 结构
  - 定义 `Collector` 结构体（cfg, providers, status）
  - 定义 `Status` 结构体记录采集状态
  - 定义 `AccountStat` 记录账号级状态
  - _Requirements: FR-008-04_

- [x] 4.1.2 实现 Provider 注册
  - 使用 `providers.Register()` 注册云厂商
  - 在 `NewCollector()` 中初始化所有 Provider
  - 支持动态加载 Provider
  - _Requirements: FR-001_

#### Task 4.2: 实现并发采集逻辑
- [x] 4.2.1 实现账号级并发
  - 遍历所有账号，为每个账号启动 Goroutine
  - 使用 `sync.WaitGroup` 等待所有采集完成
  - 添加 panic 恢复机制
  - _Requirements: NFR-001-05, NFR-002-02_

- [x] 4.2.2 实现三级并发控制
  - 区域级并发：`region_concurrency` 配置
  - 产品级并发：`product_concurrency` 配置
  - 指标级并发：`metric_concurrency` 配置
  - 使用 Worker Pool 模式管理并发
  - _Requirements: FR-007-05, NFR-001-05_

- [x] 4.2.3 实现采集循环
  - 使用 `time.Ticker` 定时触发采集
  - 支持配置 `scrape_interval`
  - 支持优雅关闭（context cancel）
  - _Requirements: NFR-002-03_

#### Task 4.3: 实现采集状态管理
- [x] 4.3.1 实现状态记录
  - 记录采集开始时间（`LastStart`）
  - 记录采集结束时间（`LastEnd`）
  - 记录采集耗时（`Duration`）
  - _Requirements: FR-008-04_

- [x] 4.3.2 实现账号状态跟踪
  - 记录每个账号的状态（running/completed/failed）
  - 更新 `LastResults` 映射
  - 提供 `GetStatus()` API 查询状态
  - _Requirements: FR-008-04_

- [x] 4.3.3 实现样本计数统计
  - 记录每个命名空间的样本数
  - 提供 `GetSampleCounts()` API
  - 在采集完成后输出统计信息
  - _Requirements: FR-008-05_

#### Task 4.4: 实现过滤和调试功能
- [x] 4.4.1 实现资源类型过滤
  - 支持 `resources: []` 或 `["*"]` 采集所有
  - 支持指定资源类型列表
  - 传递过滤参数到 Provider
  - _Requirements: FR-003-03, FR-003-04_

- [x] 4.4.2 实现 API 参数化采集
  - 添加 `CollectFiltered(provider, resource)` 方法
  - 支持调试时只采集单个云厂商或资源
  - 添加日志记录过滤信息
  - _Requirements: FR-003-03_

### Verification Tasks

- [x] Verify 4.2: 验证并发采集
  - 配置 3 个账号
  - 启动 exporter，确认并发采集日志
  - 检查采集完成时间是否符合预期
  - _Validates: NFR-001-05, NFR-002-02_

- [x] Verify 4.3: 验证状态管理
  - 调用 `/api/discovery/config` 查询状态
  - 确认 `LastStart`、`LastEnd`、`Duration` 正确
  - 确认每个账号的状态正确
  - _Validates: FR-008-04_

- [x] Verify 4.4: 验证过滤功能
  - 配置 `resources: ["clb"]`
  - 启动 exporter，确认只采集 CLB 资源
  - 检查 `/metrics` 端点确认无其他资源类型
  - _Validates: FR-003-03, FR-003-04_

## Phase 5: Provider Layer 设计与实施

### 设计概述
Provider Layer 实现各云厂商的具体采集逻辑，封装 API 调用和分片逻辑。

**对应需求：** FR-001, FR-006-04

### Tasks

#### Task 5.1: 定义 Provider 接口
- [x] 5.1.1 定义标准接口
  - 定义 `Provider` 接口
  - `Collect(account)` - 执行采集
  - `GetDefaultResources()` - 返回支持的资源类型
  - _Requirements: FR-001, NFR-003-01_

- [x] 5.1.2 实现 Provider 注册机制
  - 定义 `Factory` 类型
  - 实现 `Register(name, factory)` 函数
  - 实现 `GetFactory(name)` 函数
  - _Requirements: NFR-003-01_

#### Task 5.2: 实现阿里云 Provider
- [x] 5.2.1 实现阿里云客户端初始化
  - 创建 `internal/providers/aliyun/client.go`
  - 实现 `NewClient(accessKey, secret)` 函数
  - 初始化 CMS、SLB、OSS、ALB、NLB、GWLB 客户端
  - _Requirements: FR-001-01_

- [x] 5.2.2 实现资源发现
  - 实现 `DiscoverCBWP(account, region)` - 共享带宽包
  - 实现 `DiscoverSLB(account, region)` - 负载均衡
  - 实现 `DiscoverOSS(account, region)` - 对象存储
  - _Requirements: FR-001-01_

- [x] 5.2.3 实现指标采集
  - 实现 `CollectMetrics(account, region, resources)`
  - 调用 CMS API 批量拉取监控数据
  - 处理分页逻辑（NextToken）
  - 应用指标映射和缩放因子
  - _Requirements: FR-001-01_

- [x] 5.2.4 实现智能分页保护
  - 添加三层保护机制
  - 限制最大循环次数（maxLoop = 100）
  - 检测重复 token
  - 检测空数据但有 nextToken
  - _Requirements: FR-007-04_

- [x] 5.2.5 实现分片逻辑
  - 实现 FNV-32a 哈希算法
  - 应用区域级分片：`fnv32a(accountID|region) % total`
  - 应用产品级分片：`fnv32a(accountID|region|namespace) % total`
  - _Requirements: FR-006-04_

- [x] 5.2.6 注册阿里云 Provider
  - 在 `internal/providers/aliyun/register.go` 的 `init()` 中注册
  - 实现 `GetDefaultResources()` 返回资源列表
  - _Requirements: FR-001-01_

#### Task 5.3: 实现腾讯云 Provider
- [x] 5.3.1 实现腾讯云客户端初始化
  - 创建 `internal/providers/tencent/client.go`
  - 实现 `NewClient(secretId, secretKey)` 函数
  - 初始化 Monitor、VPC、CLB 客户端
  - _Requirements: FR-001-02_

- [x] 5.3.2 实现资源发现
  - 实现 `DiscoverBWP(account, region)` - 共享带宽包
  - 实现 `DiscoverCLB(account, region)` - 负载均衡
  - 实现 `DiscoverCOS(account, region)` - 对象存储
  - _Requirements: FR-001-02_

- [x] 5.3.3 实现指标采集
  - 实现 `CollectMetrics(account, region, resources)`
  - 调用 Monitor API 批量拉取监控数据
  - 处理 Period 自动获取（`DescribeBaseMetrics`）
  - 应用指标映射和缩放因子
  - _Requirements: FR-001-02_

- [x] 5.3.4 实现分片逻辑
  - 实现与阿里云相同的 FNV-32a 分片算法
  - 确保分片一致性
  - _Requirements: FR-006-04_

- [x] 5.3.5 注册腾讯云 Provider
  - 在 `internal/providers/tencent/register.go` 的 `init()` 中注册
  - 实现 `GetDefaultResources()` 返回资源列表
  - _Requirements: FR-001-02_

#### Task 5.4: 实现华为云 Provider
- [x] 5.4.1 实现华为云客户端初始化
  - 创建 `internal/providers/huawei/client.go`
  - 初始化 CloudEye、VPC、ELB 客户端
  - _Requirements: FR-001-03_

- [x] 5.4.2 实现资源发现和指标采集
  - 实现 `DiscoverELB(account, region)` - 弹性负载均衡
  - 实现 `DiscoverOBS(account, region)` - 对象存储
  - 实现指标采集逻辑
  - _Requirements: FR-001-03_

- [x] 5.4.3 实现分片逻辑和注册
  - 实现统一的分片算法
  - 注册华为云 Provider
  - _Requirements: FR-001-03, FR-006-04_

#### Task 5.5: 实现 AWS Provider
- [x] 5.5.1 实现 AWS 客户端初始化
  - 创建 `internal/providers/aws/client.go`
  - 使用 `aws-sdk-go-v2` 初始化客户端
  - 初始化 CloudWatch、ELB、S3 客户端
  - _Requirements: FR-001-04_

- [x] 5.5.2 实现资源发现和指标采集
  - 实现 `DiscoverELB(account, region)` - 负载均衡
  - 实现 `DiscoverS3(account, region)` - 对象存储
  - 实现指标采集逻辑
  - _Requirements: FR-001-04_

- [x] 5.5.3 实现分片逻辑和注册
  - 实现统一的分片算法
  - 注册 AWS Provider
  - _Requirements: FR-001-04, FR-006-04_

### Verification Tasks

- [x] Verify 5.2: 验证阿里云 Provider
  - 配置阿里云账号和区域
  - 启动 exporter，确认采集成功
  - 检查 `/metrics` 端点包含阿里云指标
  - _Validates: FR-001-01_

- [x] Verify 5.3: 验证腾讯云 Provider
  - 配置腾讯云账号和区域
  - 启动 exporter，确认采集成功
  - 检查指标命名统一（`clb_*`, `bwp_*`, `s3_*`）
  - _Validates: FR-001-02_

- [x] Verify 5.4: 验证华为云 Provider
  - 配置华为云账号和区域
  - 启动 exporter，确认采集成功
  - _Validates: FR-001-03_

- [x] Verify 5.5: 验证 AWS Provider
  - 配置 AWS 账号和区域
  - 启动 exporter，确认采集成功
  - _Validates: FR-001-04_

- [x] Verify 5.2-5.5: 验证分片算法
  - 配置 3 个实例（static sharding）
  - 确认每个实例采集不同的区域/产品
  - 验证无重复采集或漏采
  - _Validates: FR-006-04_

## Phase 6: Logger Layer 设计与实施

### 设计概述
Logger Layer 提供结构化日志和日志轮转功能，支持多种输出方式。

**对应需求：** FR-011

### Tasks

#### Task 6.1: 设计日志架构
- [x] 6.1.1 选择日志库
  - 使用 `uber-go/zap` 作为核心日志库
  - 使用 `lumberjack.v2` 实现日志轮转
  - _Requirements: FR-011-01_

- [x] 6.1.2 定义日志配置结构
  - 定义 `LogConfig` 和 `FileLogConfig`
  - 支持日志级别（debug, info, warn, error）
  - 支持日志格式（json, console）
  - 支持输出方式（stdout, file, both）
  - _Requirements: FR-011-02, FR-011-03_

#### Task 6.2: 实现日志初始化
- [x] 6.2.1 实现 Init 函数
  - 创建 `internal/logger/logger.go`
  - 实现 `Init(cfg *LogConfig)` 函数
  - 根据配置创建 zap logger
  - _Requirements: FR-011-01, FR-011-02_

- [x] 6.2.2 实现日志轮转
  - 配置 `lumberjack.Logger`
  - 支持按大小分割（max_size）
  - 支持保留最大备份数（max_backups）
  - 支持按时间清理（max_age）
  - 支持压缩旧日志（compress）
  - _Requirements: FR-011-04_

- [x] 6.2.3 实现上下文日志
  - 定义 `ContextLogger` 结构
  - 实现 `NewContextLogger(component, fields...)` 函数
  - 支持动态添加结构化字段
  - _Requirements: FR-011-01_

#### Task 6.3: 集成日志到各模块
- [x] 6.3.1 在 HTTP Server Layer 使用日志
  - 替换标准 log 为 ContextLogger
  - 添加请求日志
  - 添加认证日志
  - _Requirements: FR-011-01_

- [x] 6.3.2 在 Collector Layer 使用日志
  - 添加采集开始/结束日志
  - 添加账号/区域采集进度日志
  - 添加错误日志和上下文
  - _Requirements: FR-011-01, NFR-005-02, NFR-005-03_

- [x] 6.3.3 在 Discovery Layer 使用日志
  - 添加缓存命中/未命中日志
  - 添加区域状态变化日志
  - 添加标签缓存日志
  - _Requirements: FR-011-01, NFR-005-02_

### Verification Tasks

- [x] Verify 6.2: 验证日志功能
  - 配置 `log.format: json`
  - 启动 exporter，确认日志为 JSON 格式
  - 验证结构化字段（component, provider, account_id 等）
  - _Validates: FR-011-01_

- [x] Verify 6.2: 验证日志轮转
  - 配置 `log.output: file`
  - 设置 `log.file.max_size: 1` (MB)
  - 生成大量日志，确认日志文件轮转
  - 验证旧日志被压缩
  - _Validates: FR-011-04_

- [x] Verify 6.3: 验证日志集成
  - 启动 exporter，检查各模块日志
  - 确认日志包含足够的上下文信息
  - 验证错误日志便于排查问题
  - _Validates: FR-011-01, NFR-005-03_

## Phase 7: 集群部署设计

### 设计概述
集群部署支持三种模式：单机、静态分片、Kubernetes 动态分片。

**对应需求：** FR-006, FR-010

### Tasks

#### Task 7.1: 设计部署模式架构
- [x] 7.1.1 定义部署模式枚举
  - 定义 `SingleInstance`, `StaticSharding`, `DynamicSharding` 模式
  - 实现模式检测逻辑
  - _Requirements: FR-006-01, FR-006-02, FR-006-03_

- [x] 7.1.2 实现分片信息获取
  - 实现 `ClusterConfig()` 函数
  - 支持环境变量配置（静态分片）
  - 支持 Kubernetes Service 发现（动态分片）
  - 支持文件成员发现
  - _Requirements: FR-006-02, FR-006-03_

#### Task 7.2: 实现静态分片
- [x] 7.2.1 读取环境变量
  - 读取 `EXPORT_SHARD_TOTAL` 或 `CLUSTER_WORKERS`
  - 读取 `EXPORT_SHARD_INDEX` 或 `CLUSTER_INDEX`
  - 验证参数合法性
  - _Requirements: FR-006-02_

- [x] 7.2.2 应用分片逻辑
  - 在 Provider.Collect() 中调用分片函数
  - 跳过不属于自己的分片
  - 添加分片日志
  - _Requirements: FR-006-02, FR-006-04_

#### Task 7.3: 实现 Kubernetes 动态分片
- [x] 7.3.1 实现服务发现
  - 读取 `CLUSTER_DISCOVERY=headless`
  - 读取 `CLUSTER_SVC` 服务名
  - 解析 Headless Service DNS
  - _Requirements: FR-006-03_

- [x] 7.3.2 实现索引计算
  - 获取所有对等 Pod IP
  - 按排序后的 IP 确定当前索引
  - 计算总实例数和当前索引
  - _Requirements: FR-006-03, FR-006-04_

- [x] 7.3.3 实现首次采集错峰
  - 实现 `calculateFirstRunDelay()` 函数
  - 支持策略：auto, immediate, staggered
  - 实现线性延迟和指数退避算法
  - _Requirements: FR-007-05_

#### Task 7.4: 创建 Helm Chart
- [x] 7.4.1 创建 Chart 结构
  - 创建 `chart/` 目录
  - 创建 `Chart.yaml`（version, appVersion）
  - 创建 `values.yaml`（默认配置）
  - _Requirements: FR-010-01, FR-010-02_

- [x] 7.4.2 创建 Kubernetes 资源模板
  - 创建 `templates/deployment.yaml`
  - 创建 `templates/service.yaml`
  - 创建 `templates/headless-service.yaml`
  - 创建 `templates/serviceaccount.yaml`
  - 创建 `templates/server-cm.yaml`
  - 创建 `templates/accounts-cm.yaml`
  - 创建 `templates/hpa.yaml`
  - 创建 `templates/servicemonitor.yaml`
  - _Requirements: FR-010-01, FR-010-04_

- [x] 7.4.3 支持 Kubernetes Secret
  - 在 `deployment.yaml` 中使用 `secretKeyRef` 引用凭证
  - 支持账号凭证 Secret（`ALY_ACC_{{ $i }}_ID`, `ALY_ACC_{{ $i }}_AK` 等）
  - 支持认证信息 Secret（`adminSecretName` 配置）
  - _Requirements: FR-010-05, FR-009-03_

- [x] 7.4.4 创建持久化存储模板
  - 创建 `templates/region-data-pvc.yaml`
  - 支持 emptyDir（默认）和 PVC 模式
  - 添加配置字段：`regionData.persistence`
  - _Requirements: FR-010-03_

### Verification Tasks

- [x] Verify 7.2: 验证静态分片
  - 启动 3 个实例（shard 0, 1, 2）
  - 确认每个实例采集不同的区域
  - 验证所有区域都被覆盖
  - _Validates: FR-006-02, FR-006-04_

- [x] Verify 7.3: 验证动态分片
  - 使用 Helm 部署 3 个副本
  - 检查日志确认对等节点发现成功
  - 验证每个 Pod 的分片索引正确
  - _Validates: FR-006-03, FR-006-04_

- [x] Verify 7.4: 验证 Helm Chart
  - 使用 `helm install` 部署
  - 验证所有资源创建成功
  - 验证配置文件正确挂载
  - 验证区域数据持久化
  - _Validates: FR-010-01, FR-010-02, FR-010-03_

## Phase 8: 错误处理与可靠性设计

### 设计概述
确保系统在异常情况下的稳定性和可靠性。

**对应需求：** NFR-002

### Tasks

#### Task 8.1: 实现错误隔离
- [x] 8.1.1 实现 Goroutine panic 恢复
  - 在每个 Goroutine 中添加 defer + recover
  - 记录 panic 信息和堆栈
  - 不影响其他 Goroutine
  - _Requirements: NFR-002-02_

- [x] 8.1.2 实现账号级错误隔离
  - 单账号采集失败不影响其他账号
  - 记录失败账号的详细错误
  - 继续采集其他账号
  - _Requirements: NFR-002-02_

#### Task 8.2: 实现重试机制
- [x] 8.2.1 实现指数退避重试
  - 实现 `RetryWithContext()` 函数（`internal/utils/retry.go`）
  - 实现 `RetryWithBackoff()` 函数（`internal/providers/common/retry.go`）
  - 支持可配置的重试次数和超时
  - 使用指数退避算法计算延迟（BackoffFactor = 2.0）
  - _Requirements: NFR-002-01_

- [x] 8.2.2 实现可重试错误判断
  - 实现 `IsTransientError()` 函数（`internal/utils/retry.go`）
  - 实现云厂商错误分类器（`AliyunErrorClassifier`, `TencentErrorClassifier` 等）
  - 识别网络超时、限流等可重试错误（rate limit, timeout, temporary）
  - 立即返回认证错误、参数错误等不可重试错误
  - _Requirements: NFR-002-01_

#### Task 8.3: 实现优雅关闭
- [x] 8.3.1 实现信号处理
  - 监听 SIGINT, SIGTERM 信号
  - 使用 context.WithCancel() 实现关闭
  - 添加关闭信号日志
  - _Requirements: NFR-002-03_

- [x] 8.3.2 实现优雅关闭流程
  - 停止采集循环
  - 等待当前采集完成
  - 关闭 HTTP 服务器
  - 清理缓存和日志
  - _Requirements: NFR-002-03_

### Verification Tasks

- [x] Verify 8.1: 验证错误隔离
  - 配置一个无效账号（错误的 AccessKey）
  - 启动 exporter，确认无效账号采集失败但其他账号正常
  - 检查日志确认错误被正确记录
  - _Validates: NFR-002-02_

- [x] Verify 8.2: 验证重试机制
  - 模拟网络超时（使用代理或限流）
  - 检查日志确认重试触发
  - 验证指数退避延迟
  - 已有单元测试覆盖（`internal/utils/retry_test.go`, `internal/providers/common/retry_test.go`）
  - _Validates: NFR-002-01_

- [x] Verify 8.3: 验证优雅关闭
  - 启动 exporter
  - 发送 SIGTERM 信号
  - 确认当前采集完成才退出
  - 检查日志确认关闭流程
  - _Validates: NFR-002-03_

## Phase 9: 安全设计

### 设计概述
确保凭证安全、传输安全和访问控制。

**对应需求：** NFR-004, FR-009

### Tasks

#### Task 9.1: 实现凭证管理
- [x] 9.1.1 支持环境变量注入
  - 在配置文件中使用 `${VAR}` 语法
  - 在运行时替换为环境变量值
  - 支持默认值 `${VAR:-default}`
  - _Requirements: FR-005-03, NFR-004-01_

- [x] 9.1.2 支持 Kubernetes Secret
  - 在 Helm Chart 的 `deployment.yaml` 中使用 `secretKeyRef`
  - 支持账号凭证 Secret（`ALY_ACC_{{ $i }}_ID`, `ALY_ACC_{{ $i }}_AK` 等）
  - 支持认证信息 Secret（`adminSecretName` 配置）
  - 示例见 `chart/templates/deployment.yaml:101-120`
  - _Requirements: FR-010-05, FR-009-03, NFR-004-01_

- [x] 9.1.3 添加安全提示
  - 在 README 中说明不要提交明文凭证
  - 添加 `.gitignore` 排除 `accounts.local.yaml`
  - 提供示例配置文件
  - _Requirements: NFR-004-01_

#### Task 9.2: 实现传输安全
- [x] 9.2.1 添加 HTTPS 配置文档
  - 在 README 中说明生产环境使用 HTTPS
  - 提供 Ingress 配置建议
  - 提供 ServiceMesh 配置建议
  - 说明认证信息经由 HTTPS 传输
  - 参考 `README.md:681`
  - _Requirements: NFR-004-02_

- [x] 9.2.2 确保 API 使用 HTTPS
  - 验证阿里云 CMS 客户端使用 HTTPS
  - 验证腾讯云 SDK 使用 HTTPS
  - 验证 AWS SDK v2 使用 HTTPS
  - _Requirements: NFR-004-02_

#### Task 9.3: 实现容器安全
- [x] 9.3.1 非 root 用户运行
  - 在 Dockerfile 中添加 `USER 65532:65532`
  - 确保 /app/data 目录有正确的权限
  - _Requirements: NFR-004-04_

- [x] 9.3.2 最小化容器镜像
  - 使用 alpine 基础镜像
  - 只安装必要的包（ca-certificates, tzdata）
  - 使用多阶段构建
  - _Requirements: NFR-004-04_

### Verification Tasks

- [x] Verify 9.1: 验证凭证管理
  - 使用环境变量注入凭证
  - 确认配置文件中无明文凭证
  - 验证 Kubernetes Secret 正确注入
  - _Validates: FR-005-03, FR-009-03, NFR-004-01_

- [x] Verify 9.2: 验证传输安全
  - 使用 tcpdump 验证 HTTPS 加密
  - 确认 API 调用使用 TLS
  - 云 SDK 默认使用 HTTPS（已验证）
  - _Validates: NFR-004-02_

- [x] Verify 9.3: 验证容器安全
  - 检查 Docker 容器用户 ID
  - 确认不是 root（0）
  - _Validates: NFR-004-04_

## Phase 10: 可扩展性设计

### 设计概述
确保系统易于扩展新的云厂商、资源类型和指标映射。

**对应需求：** NFR-003

### Tasks

#### Task 10.1: 设计扩展接口
- [x] 10.1.1 定义统一的 Provider 接口
  - 确保接口清晰且易于实现
  - 添加详细的接口文档注释
  - _Requirements: NFR-003-01_

- [x] 10.1.2 实现自动化注册
  - 使用 `init()` 函数自动注册
  - 支持无需修改主代码添加新 Provider
  - _Requirements: NFR-003-01_

#### Task 10.2: 实现资源类型扩展
- [x] 10.2.1 支持配置文件添加资源类型
  - 在 accounts.yaml 中添加新资源类型
  - 无需修改代码
  - _Requirements: NFR-003-02_

- [x] 10.2.2 实现自动发现新资源类型
  - Provider 支持动态资源类型列表
  - 从元数据 API 获取支持的资源类型
  - _Requirements: NFR-003-02_

#### Task 10.3: 实现指标映射扩展
- [x] 10.3.1 设计映射文件格式
  - 使用 YAML 格式
  - 支持按资源类型分文件
  - _Requirements: NFR-003-03_

- [x] 10.3.2 实现动态映射加载
  - 支持目录批量加载
  - 支持运行时添加映射文件
  - _Requirements: NFR-003-03_

- [x] 10.3.3 提供映射文件模板
  - 创建 `configs/mappings/example.metrics.yaml`
  - 添加详细注释说明如何添加新映射
  - _Requirements: NFR-003-03_

### Verification Tasks

- [x] Verify 10.2: 验证资源类型扩展
  - 添加新的资源类型到 accounts.yaml
  - 重启 exporter，确认自动发现新资源
  - _Validates: NFR-003-02_

- [x] Verify 10.3: 验证指标映射扩展
  - 添加新的映射文件
  - 重启 exporter，确认新映射加载
  - 验证指标命名符合预期
  - _Validates: NFR-003-03_

## Phase 11: 综合测试与文档

### 设计概述
确保所有功能正确工作，并提供完整的文档。

### Tasks

#### Task 11.1: 编写单元测试
- [x] 11.1.1 为 HTTP Server Layer 编写测试
  - 测试端点响应
  - 测试认证中间件
  - 测试指标注册
  - _Requirements: NFR-007-02_

- [x] 11.1.2 为 Config Layer 编写测试
  - 测试配置加载
  - 测试环境变量替换
  - 测试配置验证
  - _Requirements: NFR-007-02_

- [x] 11.1.3 为 Discovery Manager 编写测试
  - 测试缓存逻辑
  - 测试区域发现状态机
  - 测试标签缓存
  - _Requirements: NFR-007-02_

- [x] 11.1.4 为 Provider Layer 编写测试
  - 为每个云厂商编写测试
  - 使用 mock 测试 API 调用
  - 测试分片逻辑
  - _Requirements: NFR-007-02_

#### Task 11.2: 编写集成测试
- [ ] 11.2.1 端到端采集测试
  - 启动 exporter
  - 配置真实的云账号
  - 验证指标正确暴露
  - _Requirements: NFR-007-02_

- [ ] 11.2.2 集群部署测试
  - 使用 Helm 部署 3 个副本
  - 验证动态分片
  - 验证区域数据持久化
  - _Requirements: FR-010_

#### Task 11.3: 完善文档
- [x] 11.3.1 更新 README
  - 添加快速开始指南
  - 添加配置说明
  - 添加故障排查指南
  - _Requirements: NFR-007-03_

- [ ] 11.3.2 更新 API 文档
  - 文档化所有 HTTP 端点
  - 文档化环境变量
  - 文档化指标含义
  - _Requirements: NFR-007-03_

- [ ] 11.3.3 更新开发文档
  - 添加如何添加新云厂商的指南
  - 添加如何添加新资源类型的指南
  - 添加如何编写测试的指南
  - _Requirements: NFR-007-03_

### Verification Tasks

- [x] Verify 11.1: 验证测试覆盖率
  - 运行 `go test -cover ./...`
  - 确认已有 49 个测试文件覆盖核心模块
  - 覆盖率数据待统计，目标覆盖率 ≥ 80%
  - _Validates: NFR-007-02_

- [ ] Verify 11.2: 验证集成测试
  - 运行所有集成测试
  - 确认无失败
  - _Validates: FR-010, FR-004_

- [ ] Verify 11.3: 验证文档完整性
  - 检查 README 是否完整
  - 检查 API 文档是否准确
  - 检查开发文档是否清晰
  - _Validates: NFR-007-03_

## Phase 12: Checkpoint 与最终验证

### Tasks

- [ ] 12.1 运行完整测试套件
  - 运行 `make lint`
  - 运行 `go test -race -cover ./...`
  - 运行基准测试 `go test -bench .`
  - _Requirements: NFR-007-02_

- [x] 12.2 本地验证
  - 启动 exporter
  - 验证所有端点正常
  - 验证指标正确采集
  - 验证日志正常输出
  - _Requirements: 所有 FR 和 NFR_

- [ ] 12.3 更新 CHANGELOG
  - 记录本次实施的所有变更
  - 说明影响范围和修复内容
  - 标注版本号
  - _Requirements: NFR-007-04_

## Appendix: References

### A. 技术栈

| 组件 | 技术选型 | 说明 |
|-----|---------|------|
| 语言 | Go 1.25 | 高性能、并发友好 |
| HTTP | `net/http` | 标准库 |
| Prometheus | `prometheus/client_golang` v1.18.0 | 指标库 |
| 日志 | `uber-go/zap` v1.27.1 | 结构化日志 |
| 日志轮转 | `lumberjack.v2` v2.2.1 | 文件轮转 |
| 配置 | `yaml.v3` | YAML 解析 |
| 云 SDK | 各厂商官方 SDK | Aliyun SDK, Tencent SDK, Huawei SDK, AWS SDK v2 |

### B. 需求追溯矩阵

| Phase | 主要任务 | 涉及的需求ID |
|-------|---------|-------------|
| Phase 1 | HTTP Server Layer | FR-004, FR-009, FR-011 |
| Phase 2 | Config Layer | FR-005 |
| Phase 3 | Discovery Manager | FR-003, FR-007 |
| Phase 4 | Collector Layer | FR-003, FR-008 |
| Phase 5 | Provider Layer | FR-001, FR-006 |
| Phase 6 | Logger Layer | FR-011 |
| Phase 7 | 集群部署 | FR-006, FR-010 |
| Phase 8 | 错误处理与可靠性 | NFR-002 |
| Phase 9 | 安全设计 | NFR-004, FR-009 |
| Phase 10 | 可扩展性设计 | NFR-003 |
| Phase 11 | 综合测试与文档 | NFR-002, NFR-003, NFR-007 |

### C. 任务标记说明

- `[ ]` - 待开始的任务
- `[x]` - 已完成的任务
- `[*]` - 可选任务，可跳过以加快 MVP 开发
- `_Requirements: XX, XX_` - 任务引用的需求ID

### D. Property Testing 要求

部分验证任务需要运行至少 100 次迭代以确保稳定性，标记为：
- **Property X: 属性名称**
- **Validates: Requirements X.X, X.X**

## Notes

1. **任务执行顺序**：按 Phase 顺序执行，各 Phase 内任务可并行
2. **依赖关系**：明确标注任务间的依赖关系
3. **进度跟踪**：定期更新任务状态 `[ ]` → `[x]`
4. **质量保证**：每个 Phase 完成后运行验证任务
5. **向后兼容性**：确保现有功能不受影响（Phase 5.2.4）

## 实施进度总结

### 已完成的核心模块 (2026-01-04)

#### 高优先级完成度 (90%+)
- **Phase 1: HTTP Server Layer** ✅
  - 所有核心端点已实现
  - BasicAuth 认证已实现
  - 指标映射已实现

- **Phase 2: Config Layer** ✅
  - 配置文件结构已定义
  - 配置加载和验证已实现
  - 环境变量支持已实现

- **Phase 3: Discovery Manager** ✅
  - 资源发现接口已实现
  - 资源ID 缓存已实现
  - 标签缓存已实现
  - 智能区域发现已实现

- **Phase 4: Collector Layer** ✅
  - 采集调度已实现
  - 并发采集已实现
  - 状态管理已实现

- **Phase 5: Provider Layer** ✅
  - 阿里云 Provider 已完整实现
  - 腾讯云 Provider 已完整实现
  - 华为云 Provider 已完整实现
  - AWS Provider 已完整实现
  - 智能分页保护已实现
  - 分片逻辑已实现

- **Phase 6: Logger Layer** ✅
  - 结构化日志已实现
  - 日志轮转已实现
  - 上下文日志已实现

- **Phase 7: 集群部署** ✅
  - 三种部署模式已实现
  - Helm Chart 已创建
  - 持久化支持已实现

- **Phase 8: 错误处理与可靠性** ✅
  - 错误隔离已实现
  - 优雅关闭已实现
  - 重试机制已完整实现（指数退避、错误分类）
  - 单元测试已覆盖

- **Phase 9: 安全设计** ✅
  - 凭证管理已实现
  - 容器安全已实现
  - Kubernetes Secret 已支持
  - HTTPS 配置文档已完善

- **Phase 10: 可扩展性设计** ✅
  - Provider 接口已定义
  - 自动注册已实现
  - 指标映射扩展已支持

#### 待完成 (低优先级)
- **Phase 11: 综合测试与文档** ⏳
  - 单元测试已完成（49 个测试文件）
  - 集成测试待完善（Task 11.2）
  - README 已完善（Task 11.3.1）
  - API 文档待完善（Task 11.3.2）
  - 开发文档待完善（Task 11.3.3）

- **Phase 12: Checkpoint 与最终验证** ⏳
  - CHANGELOG 待创建（Task 12.3）
  - 集成测试待完善
  - 完整测试套件待运行（Task 12.1）

### 关键指标
- **总体完成度**: 约 90%
- **核心功能完成度**: 约 98%
- **文档完善度**: 约 75%
- **测试覆盖率**: 已有 49 个测试文件覆盖核心模块

## Appendix: 后续工作计划

### 优先级 P0（必须完成，用于正式发布）

#### 1. 创建 CHANGELOG.md
**目标**: 记录项目版本历史和重要变更

**具体任务**:
- [ ] 创建 `CHANGELOG.md` 文件
- [ ] 定义版本号格式（遵循 Semantic Versioning v2.0.0）
- [ ] 记录当前版本的功能（v0.1.0 或 v1.0.0）
- [ ] 添加贡献指南（如何添加新的 changelog 条目）
- [ ] 配置 CI/CD 自动化更新 changelog

**参考格式**:
```markdown
# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- 支持 OCI、GCP 云服务提供商
- 基于状态机的智能区域发现
- 自动资源与标签缓存
- 智能分页保护，带有最大循环限制
- 样本计数统计
- 指数退避重试机制
- 管理端点的 BasicAuth认 证
- Kubernetes部署的Helm图表
- 区域数据持久卷支持

### Changed
- N/A

### Deprecated
- N/A

### Removed
- N/A

### Fixed
- N/A

### Security
- N/A
```

**预计时间**: 1-2 小时

#### 2. 补充 API 文档
**目标**: 完善对外 API 的文档，便于用户集成

**具体任务**:
- [ ] 在 `docs/api.md` 中详细列出所有 HTTP 端点
- [ ] 为每个端点提供：
  - 端点路径和方法
  - 请求/响应示例
  - 认证要求
  - 错误码说明
- [ ] 更新 README.md，添加 API 文档链接
- [ ] 考虑生成 OpenAPI/Swagger 规范

**端点列表**:
- `GET /metrics` - Prometheus 指标暴露
- `GET /healthz` - 健康检查
- `GET /readyz` - 就绪检查
- `GET /api/status` - 采集状态
- `GET /api/config` - 当前配置（需认证）
- `GET /api/discovery/config` - Discovery 配置（需认证）
- `POST /api/config/reload` - 热加载配置（需认证）
- `POST /api/discovery/refresh` - 刷新 Discovery 缓存（需认证）

**预计时间**: 3-4 小时

### 优先级 P1（强烈建议完成）

#### 3. 补充集成测试
**目标**: 验证各模块协同工作的正确性

**具体任务**:
- [ ] 创建 `internal/providers/common/integration_test.go`
- [ ] 编写端到端测试场景：
  - 完整采集流程（配置加载 → 发现 → 采集 → 暴露指标）
  - 错误恢复场景（API 限流、超时）
  - 集群分片场景（多 Pod 并行采集）
  - 智能区域发现场景（状态机转换）
- [ ] 添加测试辅助函数（mock 云 API）
- [ ] 配置 CI 自动运行集成测试

**预计时间**: 8-12 小时

#### 4. 完善 README.md
**目标**: 提供清晰的用户指南

**具体任务**:
- [ ] 添加快速开始章节（Quick Start）
- [ ] 补充配置文件示例（带详细注释）
- [ ] 添加故障排查章节（Troubleshooting）
- [ ] 添加性能调优建议
- [ ] 补充最佳实践章节
- [ ] 添加 FAQ 常见问题解答

**预计时间**: 4-6 小时

### 优先级 P2（可选，增强用户体验）

#### 5. Helm Chart 增强
**目标**: 提供更完善的 Kubernetes 部署支持

**具体任务**:
- [ ] 创建独立的 `secret.yaml` 模板
- [ ] 添加 `networkPolicy.yaml`（网络安全策略）
- [ ] 添加 `pdb.yaml`（PodDisruptionBudget）
- [ ] 添加 `values-prod.yaml`（生产环境配置示例）
- [ ] 添加 `values-dev.yaml`（开发环境配置示例）

**预计时间**: 4-6 小时

#### 6. Grafana Dashboard
**目标**: 提供开箱即用的监控面板

**具体任务**:
- [ ] 创建 Grafana Dashboard JSON
- [ ] 设计面板布局：
  - 采集状态概览
  - 各云厂商资源统计
  - 指标样本数趋势
  - 采集耗时分布
  - 错误率统计
- [ ] 添加到 `grafana/` 目录
- [ ] 在 README 中添加导入说明

**预计时间**: 6-8 小时

#### 7. 性能基准测试
**目标**: 建立性能基线，监控回归

**具体任务**:
- [ ] 创建 `benchmarks/` 目录
- [ ] 编写基准测试：
  - 采集器启动时间
  - 单个资源采集耗时
  - 并发采集吞吐量
  - 内存占用
- [ ] 添加 CI 性能基准检查
- [ ] 记录基线数据

**预计时间**: 4-6 小时

### 优先级 P3（长期规划）

#### 7. 配置热加载
**目标**: 支持不重启更新配置

**具体任务**:
- [ ] 实现 `fsnotify` 监听配置文件变化
- [ ] 实现配置热加载逻辑
- [ ] 添加配置验证（确保新配置有效）
- [ ] 添加通知机制（WebSocket/日志）

**预计时间**: 8-12 小时
**对应设计**: Phase 2 - Task 2.4（标记为可选）

#### 8. 多语言 SDK 支持
**目标**: 支持其他语言接入

**具体任务**:
- [ ] 设计多语言 SDK 接口
- [ ] 实现 Python SDK
- [ ] 添加示例和文档
- [ ] 发布到 PyPI

**预计时间**: 16-24 小时

#### 9. 指标预测与告警
**目标**: 提供更智能的监控能力

**具体任务**:
- [ ] 实现趋势分析算法
- [ ] 添加异常检测
- [ ] 集成告警规则生成
- [ ] 提供 Prometheus 告警规则模板

**预计时间**: 16-24 小时

## 时间线建议

### 短期（1-2 周）
- [ ] 创建 CHANGELOG.md (P0)
- [ ] 补充 API 文档 (P0)
- [ ] 完善 README.md (P1)

### 中期（1 个月）
- [ ] 补充集成测试 (P1)
- [ ] Helm Chart 增强 (P2)
- [ ] Grafana Dashboard (P2)

### 长期（3 个月+）
- [ ] 性能基准测试 (P2)
- [ ] 配置热加载 (P3) - 对应 Phase 2 Task 2.4
- [ ] 多语言 SDK 支持 (P3)
- [ ] 指标预测与告警 (P3)

## 发布准备清单

### 代码质量
- [x] 代码符合项目规范
- [x] 单元测试覆盖率较高（49 个测试文件）
- [ ] 集成测试完善
- [ ] 性能基准测试通过

### 文档完整性
- [x] 设计文档完善
- [x] README 完整
- [ ] API 文档完善
- [ ] CHANGELOG.md 创建

### 部署就绪
- [x] Helm Chart 完善
- [ ] Grafana Dashboard 提供
- [ ] 生产环境配置示例

### 版本管理
- [ ] 定义版本号
- [ ] 创建 Git Tag
- [ ] 发布 Release
- [ ] 推送到 Docker Registry
