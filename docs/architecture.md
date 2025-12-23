# multicloud-exporter 集群与并行架构设计

> 更新记录：2025-12-08；修改者：@jangrui；内容：补充 Period 自动适配、来源优先级与统一指标映射；校准组件说明与技术选型。

## 1. 云原生架构（Kubernetes）

### 1.1 架构图（Mermaid）

```mermaid
---
config:
  theme: mc
  layout: elk
---
graph LR
  subgraph Kubernetes_Cluster
    subgraph Deployment
      P1[Pod]
      P2[Pod]
      P3[Pod]
    end
    S[Service ClusterIP]
    HS[Headless Service]
    HPA[HPA]
    PROBES[Probes healthz metrics]
    AFF[Pod Anti-Affinity]
    SM[ServiceMonitor]
    PROM[Prometheus]

    Deployment --> S
    PROM -- scrape --> S
    SM -- discovery --> PROM
    HS --> Deployment
    HPA --> Deployment
    PROBES --> Deployment
    AFF --> Deployment
  end
```

### 1.2 关键点

- 副本与伸缩：`replicaCount` + `HPA`（CPU 70% 目标，`minReplicas`/`maxReplicas` 可调）。
- 服务发现与负载：`Service` 提供负载均衡；可选 `Headless Service` 提供 Pod IP 列表用于成员发现。
- 反亲和与调度优化：通过 `affinity`/`topologySpreadConstraints`（Chart 可配置）实现跨节点扩散。
- 健康检查：容器暴露 `GET /healthz`（存活探针）与 `GET /metrics`（就绪探针）。
- 指标采集与导出：使用 `ServiceMonitor` 或原生注解方式供 Prometheus 抓取。
- Period 自动适配：未显式配置时，采集器调用云侧元数据接口选择指标的最小可用 `Period`，以与 `server.scrape_interval` 保持一致；实现位置见 `internal/providers/tencent/tencent.go:136-197`，调用点 `internal/providers/tencent/clb.go:79-83`、`internal/providers/tencent/bwp.go:75-79`，阿里云参考 `internal/providers/aliyun/aliyun.go:561-615`。

### 1.3 Helm 关键配置

- `values.yaml`：
  - `replicaCount`: 副本数。
  - `hpa.enabled`, `hpa.minReplicas`, `hpa.maxReplicas`, `hpa.metrics`: 自动伸缩规则。
  - `probes.liveness`, `probes.readiness`: 健康探针。
  - `headless.enabled`, `headless.name`: 是否启用 Headless Service。
  - `cluster.discovery`: `headless` | `file` | 空；`cluster.svcName`/`cluster.file` 配合使用。

- 模板：
  - `templates/deployment.yaml` 支持 `replicaCount`、探针、Cluster 相关环境变量、Downward API 注入。
  - `templates/hpa.yaml`（启用时渲染）。
  - `templates/service.yaml` 提供对外服务与 Prometheus 抓取入口。
  - `templates/headless-service.yaml`（可选）为成员发现提供 Pod IP 解析。
  - `templates/servicemonitor.yaml`（如使用 Operator）。

### 1.4 实现步骤

- 设置 `replicaCount` 与资源请求，部署服务。
- 根据负载开启 `hpa.enabled` 并配置目标指标（CPU或自定义）。
- 如需成员分片，在 `values.yaml` 中设置：
  - `headless.enabled: true`，并将 `cluster.discovery: headless`，`cluster.svcName` 指向 headless 服务名。
- 如需强约束调度，配置 `affinity`/`topologySpreadConstraints`。
- 使用 `ServiceMonitor` 或在 `Service` 上配置抓取注解，完成 Prometheus 集成。
- 配置 Period 与采集频率：`server.scrape_interval` 推荐与云侧最小 `Period` 一致；Chart 文档与 README 已补充说明。

## 2. 传统宿主机并行架构

### 2.1 架构图（Mermaid）

```mermaid
---
config:
  theme: mc
  layout: elk
---
graph LR
  subgraph Hosts
    I0[Exporter instance 0]
    I1[Exporter instance 1]
    I2[Exporter instance 2]
    FILE[Shared members file]
    SHARD[Deterministic sharding fnv]
    HR[Hot-reload SIGHUP polling]
  end

  PROM[Prometheus]

  PROM -- scrape --> I0
  PROM -- scrape --> I1
  PROM -- scrape --> I2

  FILE --> I0
  FILE --> I1
  FILE --> I2

  SHARD --> I0
  SHARD --> I1
  SHARD --> I2

  HR --> I0
  HR --> I1
  HR --> I2
```

### 2.2 核心算法

- 成员发现（K8s/宿主机通用）：`internal/utils/sharding.go` 的 `ClusterConfig` 函数。
    - `headless`: 解析 `CLUSTER_SVC` DNS 获取 Pod IP 列表，匹配 `POD_IP` 得到 `(wTotal,wIndex)`。
    - `file`: 读取 `CLUSTER_FILE` 列表，与 `POD_NAME`/`HOSTNAME` 匹配计算 `(wTotal,wIndex)`。

- 分片与路由：
  - 核心算法：集中于 `internal/utils/sharding.go`，提供 `ClusterConfig`（获取总分片数与当前索引）与 `ShouldProcess`（判断是否处理当前 Key）。
  - 分片策略：
    - **两级分片机制**：
      1. **区域级分片**：由各 Provider 在 `Collect` 方法中调用 `ShouldProcess(AccountID|Region)`，决定是否处理该区域。
      2. **产品级分片**：由各 Provider 在产品采集循环中调用 `ShouldProcess(AccountID|Region|Namespace)`，决定是否处理该产品。
  - 分片键格式：
    - 区域级：`AccountID|Region`（例如：`acc-1|cn-hangzhou`）
    - 产品级：`AccountID|Region|Namespace`（例如：`acc-1|cn-hangzhou|acs_ecs_dashboard`）
  - 实现位置：
    - 区域级分片：`internal/providers/aliyun/aliyun.go:161-164`、`internal/providers/tencent/tencent.go:56-60`
    - 产品级分片：`internal/providers/aliyun/aliyun.go:277-283`、`internal/providers/tencent/tencent.go:175-183`、`internal/providers/aws/lb.go:217-235`
  - 哈希函数：`ShardIndex` 在 `internal/utils/sharding.go`，使用 FNV-32a 算法。

- 配置热更新：
  - K8s：使用 ConfigMap + `stakater/reloader` 注解已集成；Chart 已支持。
  - 宿主机：SIGHUP 信号触发配置重载，或定时轮询文件更新时间（推荐 15–60s）。

### 2.3 关键环境变量

- `CLUSTER_DISCOVERY`: `headless`/`file`/空。
- `CLUSTER_SVC`: 成员服务名（headless）。
- `CLUSTER_FILE`: 成员列表文件路径（file）。
- `CLUSTER_WORKERS`/`CLUSTER_INDEX`: 静态分片参数回退。

## 3. 通用要求实现

### 3.1 监控指标

- 采集成效与性能：
  - `collection_duration_seconds` 在 `cmd/multicloud-exporter/main.go:128` 统计周期时长。
  - `request_total` 与 `request_duration_seconds` 在各 provider 中记录云 API 成功/失败与耗时。
  - `rate_limit_total` 统计限流触发次数。
  - 资源指标：统一暴露在 `metrics.NamespaceMetric`/`metrics.ResourceMetric`，示例见 `internal/metrics/*`。
- 统一命名与映射：通过 `configs/mappings/*.yaml` 与别名函数保持跨云一致（如 ALB/BWP/CBWP/CLB/COS/NLB/GWLB/OSS）；Aliyun SLB 别名函数见 `internal/metrics/aliyun/slb.go:22-46`，Tencent CLB 别名注册见 `internal/metrics/tencent/clb.go:9-32`，BWP 前缀注册见 `internal/metrics/tencent/bwp.go:9-18`。

### 3.2 自动化部署

- Kubernetes：
  - 安装：`helm install mce ./chart -f values.yaml`。
  - 副本与伸缩：设置 `replicaCount`，或启用 `hpa.enabled`。
  - 成员发现：`headless.enabled: true`；`cluster.discovery: headless`；`cluster.svcName: <svc-name>`。
  - 健康检查：`probes.liveness/readiness` 默认启用。
  - 认证与安全：`server.admin_auth_enabled` 启用管理接口 BasicAuth；通过 Ingress/ServiceMesh 终止 TLS；云 SDK 强制 HTTPS。

- 宿主机：
  - Systemd 单元示例：
    ```
    [Unit]
    Description=multicloud-exporter
    After=network.target

    [Service]
    ExecStart=/usr/local/bin/multicloud-exporter
    Environment="SERVER_PATH=/etc/mce/server.yaml"
    Environment="CLUSTER_DISCOVERY=file"
    Environment="CLUSTER_FILE=/var/run/mce/members.txt"
    Restart=always

    [Install]
    WantedBy=multi-user.target
    ```
  - Prometheus `static_configs` 指向各实例 `:9101`。

### 3.3 高可用与故障转移

- K8s：Deployment 多副本 + HPA；探针失败自动重启；Pod 反亲和减少同机失败概率；Prometheus 多 target 抓取容错。
- 宿主机：多实例并行；外部维护成员列表文件（自动化运维工具更新）；分片哈希稳定，实例宕机后剩余实例仍覆盖其分片（通过总成员变化重算）。

### 3.4 性能指标与目标

- 采集周期建议与云 API Period 匹配（详见 README）。
- 并发控制：
  - 区域并发：`server.region_concurrency`；
  - 产品并发：`server.product_concurrency`（默认 2，控制同一地域内不同命名空间的并行度）；
  - 指标并发：`server.metric_concurrency`（默认 5，控制同一命名空间下多个指标批次的并行度）。

## 4. 故障排查指南

### 4.1 指标丢失问题

**症状**：Prometheus 中某些指标没有数据或数据不连续。

**排查步骤**：

1. **检查采集状态**：
   ```bash
   curl http://localhost:9101/api/discovery/status
   ```
   查看各账号的采集状态和最后完成时间。

2. **检查日志**：
   ```bash
   kubectl logs -f deployment/multicloud-exporter | grep -i error
   ```
   关注以下错误：
   - `auth_error`：认证失败，检查 AccessKey 配置
   - `limit_error`：API 限流，检查 `multicloud_rate_limit_total` 指标
   - `region_skip`：区域不支持，检查账号的区域权限

3. **检查资源权限**：
   - 确认账号配置中的 `resources` 字段包含要采集的资源类型
   - 检查云厂商控制台中的资源是否存在

4. **检查 Period 配置**：
   - 确认 `server.scrape_interval` 与云 API 的 `Period` 匹配
   - 如果 `scrape_interval > Period`，会导致数据丢失（详见 README）

5. **检查分片配置**：
   - 在集群模式下，确认资源是否被正确分片
   - 检查 `CLUSTER_WORKERS` 和 `CLUSTER_INDEX` 配置

### 4.2 API 限流问题

**症状**：日志中出现大量 `limit_error`，采集速度变慢。

**排查步骤**：

1. **查看限流统计**：
   ```bash
   curl http://localhost:9101/metrics | grep multicloud_rate_limit_total
   ```
   查看各云厂商和 API 的限流次数。

2. **检查并发配置**：
   - 降低 `server.region_concurrency`（默认 3）
   - 降低 `server.product_concurrency`（默认 2）
   - 降低 `server.metric_concurrency`（默认 5）

3. **检查采集频率**：
   - 增加 `server.scrape_interval`，减少 API 调用频率
   - 增加 `server.discovery_ttl`，减少资源发现频率

4. **检查缓存配置**：
   - 确认 `server.discovery_ttl` 设置合理（建议 ≥ 1h）
   - 查看 `multicloud_cache_entries_total` 指标，确认缓存生效

### 4.3 内存增长问题

**症状**：Pod 内存使用持续增长，可能触发 OOM。

**排查步骤**：

1. **查看缓存指标**：
   ```bash
   curl http://localhost:9101/metrics | grep multicloud_cache
   ```
   关注 `multicloud_cache_size_bytes` 和 `multicloud_cache_entries_total`。

2. **检查缓存 TTL**：
   - 确认 `server.discovery_ttl` 设置合理
   - 如果资源数量很大，考虑缩短 TTL 或增加 Pod 内存限制

3. **检查资源数量**：
   - 确认账号中的资源数量是否异常增长
   - 检查是否有资源泄漏（已删除的资源仍在缓存中）

4. **调整资源配置**：
   ```yaml
   resources:
     limits:
       memory: "512Mi"
     requests:
       memory: "256Mi"
   ```

### 4.4 采集超时问题

**症状**：采集任务长时间未完成，日志中出现超时错误。

**排查步骤**：

1. **检查网络连接**：
   - 确认 Pod 可以访问云厂商 API 端点
   - 检查防火墙和网络策略

2. **检查 API 响应时间**：
   ```bash
   curl http://localhost:9101/metrics | grep multicloud_request_duration_seconds
   ```
   查看各 API 的响应时间，如果持续很高，可能是网络问题。

3. **调整超时配置**：
   - 如果网络较慢，可以增加 HTTP 客户端超时时间
   - 检查云厂商 API 的服务状态

4. **检查并发配置**：
   - 降低并发数，避免过多并发请求导致超时
   - 检查云厂商的 API 限流策略

## 5. 性能调优建议

### 5.1 并发参数调优

**区域并发（region_concurrency）**：
- **默认值**：3
- **调优建议**：
  - 账号区域数量少（< 5）：可以增加到 5-10
  - 账号区域数量多（> 10）：保持默认或降低到 2
  - 如果遇到限流，降低到 1-2

**产品并发（product_concurrency）**：
- **默认值**：2
- **调优建议**：
  - 命名空间数量少（< 3）：可以增加到 3-5
  - 命名空间数量多（> 5）：保持默认或降低到 1
  - 如果遇到限流，降低到 1

**指标并发（metric_concurrency）**：
- **默认值**：5
- **调优建议**：
  - 指标数量少（< 10）：可以增加到 10-20
  - 指标数量多（> 50）：保持默认或降低到 3
  - 如果遇到限流，降低到 1-2

### 5.2 TTL 调优

**发现 TTL（discovery_ttl）**：
- **默认值**：1h
- **调优建议**：
  - 资源变化频繁：缩短到 30m
  - 资源变化不频繁：延长到 2h-4h
  - 如果遇到限流，延长到 4h-8h

**缓存 TTL**：
- 资源 ID 缓存：与 `discovery_ttl` 一致
- 元数据缓存：由各 Provider 内部管理，通常为 1h

### 5.3 采集频率调优

**Scrape Interval**：
- **默认值**：60s
- **调优建议**：
  - 关键指标：与云 API 的 `Period` 匹配（通常为 60s）
  - 非关键指标：可以设置为 300s（5分钟）以节省 API 调用
  - 注意：如果 `scrape_interval > Period`，会导致数据丢失

**Period 自动适配**：
- Exporter 会自动从云侧元数据选择指标的最小可用 `Period`
- 如果云 API 支持多个 Period，优先选择与 `scrape_interval` 最接近的值
- 如果元数据不可用，使用 `server.period_fallback`（默认 60s）

### 5.4 资源限制调优

**内存限制**：
- **建议值**：
  - 小规模（< 100 资源）：256Mi
  - 中规模（100-1000 资源）：512Mi
  - 大规模（> 1000 资源）：1Gi-2Gi

**CPU 限制**：
- **建议值**：
  - 小规模：100m-200m
  - 中规模：200m-500m
  - 大规模：500m-1000m

### 5.5 监控指标调优

**关键指标**：
- `multicloud_collection_duration_seconds`：采集周期耗时
- `multicloud_request_total`：API 调用总数（按状态分类）
- `multicloud_rate_limit_total`：限流次数
- `multicloud_cache_size_bytes`：缓存大小
- `multicloud_cache_entries_total`：缓存条目数

**告警规则建议**：
- 采集耗时 > 5 分钟：可能存在问题
- 限流次数持续增长：需要降低并发或增加采集间隔
- 缓存大小持续增长：可能需要调整 TTL 或增加内存限制
  - 产品并发：`server.product_concurrency`；
  - 指标并发：`server.metric_concurrency`；
  - 最终目标：P95 周期完成时间 ≤ 周期时长的 0.6；错误率 ≤ 0.1%。
  - 基准与压力：CI 执行 `go test -bench . -benchmem -run ^$ ./...` 与并行压力 `go test -race -run . -parallel 16 ./...`，收集 `benchmem` 指标并观察限流错误。

### 3.5 平滑升级与回滚

- K8s：RollingUpdate（`maxUnavailable=0` 推荐）；保留旧版本镜像；Chart 版本化。
- 宿主机：逐台滚动，利用系统级负载均衡与 Prometheus 抓取冗余避免可见中断；可在灰度窗口观察核心指标。

## 4. 关键实现引用

- `cmd/multicloud-exporter/main.go:137` 注册 `/metrics`；`cmd/multicloud-exporter/main.go:137`–`179` 周期采集与事件流接口；`cmd/multicloud-exporter/main.go:137` 新增 `/healthz`。
- `internal/collector/collector.go:103` 成员发现；`internal/collector/collector.go:182` 账号分片；`internal/collector/collector.go:173` 哈希函数。
- `internal/providers/aliyun/aliyun.go:119` 区域分片；`internal/providers/tencent/tencent.go:52` 区域分片。
- `chart/templates/deployment.yaml`、`chart/templates/hpa.yaml`、`chart/templates/headless-service.yaml`：部署、伸缩与成员发现支持。

## 5. 配置参数总览（Helm）

- `replicaCount`
- `hpa.enabled`, `hpa.minReplicas`, `hpa.maxReplicas`, `hpa.metrics`
- `probes.liveness.*`, `probes.readiness.*`
- `headless.enabled`, `headless.name`
- `cluster.discovery`, `cluster.svcName`, `cluster.file`
- `server.*`（采集并发、日志、周期）

## 版本历史

- 2025-12-08：补充 Period 自动适配、统一指标映射与来源优先级；修改者： @jangrui。

## 6. 实施清单

- 部署前校验：`helm lint chart/`；`go build`；`go vet`。
- 监控接入：配置 `ServiceMonitor` 或抓取注解；加载 Grafana Dashboard。
- 压测与配额：根据 API 限流优化并发与周期；观察 `request_total` 的 `limit_error` 维度。
