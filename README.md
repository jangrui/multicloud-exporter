# 多云资源监控 Exporter

支持阿里云、华为云、腾讯云的资源监控，按云平台、账号、区域区分。

## 功能特性

- 支持多云平台：阿里云、华为云、腾讯云
- 支持多账号配置
- 支持多区域监控
- 按云平台、账号、区域标签区分
- 兼容 Prometheus 格式
- 动态指标命名：按云产品命名空间+指标名生成，例如阿里云共享带宽 `acs_bandwidth_package_in_bandwidth_utilization`
- 资源发现缓存：枚举到的资源ID支持缓存与TTL，显著降低 API 次数
- 标签缓存优化：资源标签支持缓存，复用于同一资源的多个指标，大幅减少 VPC API 调用（阿里云 ↓90%）
- 自身监控：内置 API 请求耗时、限流统计与采集周期耗时指标
- 管理接口认证：`/api/discovery/*` 可选启用 BasicAuth
- 传输安全：阿里云 CMS 客户端与腾讯云 SDK 默认使用 HTTPS
- 智能区域发现：自动识别有资源的区域，优先采集活跃区域，跳过空区域，显著降低 API 调用和采集延迟

## 支持的资源类型

### 阿里云
- [x] 共享带宽包（CBWP）
- [x] 负载均衡
  - [x] 应用负载均衡（ALB）
  - [x] 传统负载均衡（CLB）
  - [x] 网络负载均衡（NLB）
  - [x] 网关负载均衡（GWLB）
- [x] 对象存储 (OSS)

### 腾讯云
- [x] 负载均衡
  - [x] 负载均衡（CLB）
  - [x] 网关负载均衡（GWLB）
- [x] 共享带宽包（BWP）
- [x] 对象存储 (COS)

### 华为云
- [x] 弹性负载均衡（ELB）
- [x] 对象存储（OBS）

### AWS
- [x] 负载均衡
  - [x] 应用负载均衡（ALB）
  - [x] 经典负载均衡（CLB）
  - [x] 网络负载均衡（NLB）
  - [x] 网关负载均衡（GWLB）
- [x] 对象存储（S3）

## 配置文件

采用拆分配置，位于 `configs/` 目录；也可通过环境变量指定任意路径。

### server.yaml

```yaml
server:
  port: 9101
  page_size: 1000
  discovery_ttl: "1h"
  scrape_interval: "60s"
  
  # 日志配置
  log:
    level: info      # debug, info, warn, error
    format: json     # json, console
    output: stdout   # stdout, file, both
    file:
      path: logs/exporter.log
      max_size: 100  # MB
      max_backups: 3
      max_age: 28    # days
      compress: true
```

### 采集周期与 Period 自动适配

- Exporter 在未显式配置 `Product.Period` 或 `MetricGroup.Period` 时，会调用云厂商元数据接口（如腾讯云 `DescribeBaseMetrics`）自动获取该指标支持的 `Periods` 列表，并选择最小值作为请求参数。
- 建议将 `server.scrape_interval` 与云侧 `Period` 保持一致或略大于等于该值，避免中间数据点丢失。
- 若需要覆盖默认行为，可在产品或指标组层级显式设置 `Period`。

### 区域枚举（regions="*")

- 当 `accounts.yaml` 中某账号的 `regions` 为空或为 `["*"]` 时，系统将自动调用云厂商区域元数据接口进行枚举：
  - 阿里云：`DescribeRegions`（ECS），遍历返回的全部 `RegionId`
  - 腾讯云：`DescribeRegions`（CVM），遍历返回的全部 `Region`
- 容错与回退：若枚举失败，可通过环境变量 `DEFAULT_REGIONS` 指定逗号分隔的区域作为回退，例如：`DEFAULT_REGIONS=cn-hangzhou,ap-guangzhou`

### 智能区域发现（Region Discovery）

智能区域发现功能可以自动识别哪些区域有资源，优先采集有资源的区域，跳过长期无资源的区域，显著提升采集性能。

```yaml
server:
  region_discovery:
    enabled: true              # 是否启用智能区域发现（默认 true）
    discovery_interval: "24h"   # 重新发现周期，定期将所有区域设为 unknown，重新探测（默认 24h）
    empty_threshold: 3           # 空区域跳过阈值，连续 N 次为空后跳过该区域（默认 3）
    data_dir: "/app/data"        # 数据目录路径（默认 /app/data）
    persist_file: "region_status.json"  # 持久化文件名，相对于 data_dir（默认 region_status.json）
```

**工作原理**：
- **首次运行**：所有区域状态为 `unknown`，采集时检查所有区域
- **后续运行**：
  - 有资源的区域标记为 `active`，优先采集
  - 无资源的区域标记为 `empty`，连续 N 次为空后跳过（默认 3 次）
  - 定期重新发现（默认 24 小时），将所有区域重置为 `unknown`，重新探测资源变化
- **状态持久化**：区域状态保存到 JSON 文件，重启后可快速恢复，避免重复探测

**优势**：
- **性能提升**：跳过大量无资源的区域，减少 API 调用和采集延迟
- **成本降低**：减少云厂商 API 配额消耗
- **自适应**：定期重新发现，自动适应新增资源或区域
- **可观测性**：提供区域状态统计和跳过次数指标

**状态持久化选项**：

区域状态支持两种持久化方式（Kubernetes 部署时可选）：

| 方式 | 说明 | 生命周期 | 适用场景 | 配置方式 |
|------|------|----------|----------|----------|
| **emptyDir**（默认） | 临时存储，Pod 生命周期内保留 | Pod 删除后数据丢失 | 开发测试、短期运行 |
| **PVC** | 持久化存储，跨 Pod 重启保留 | Pod 删除后数据仍保留 | 生产环境、长期运行 |

#### 使用 emptyDir（默认）

```yaml
# values.yaml
server:
  regionDiscovery:
    enabled: true

regionData:
  persistence:
    enabled: false  # 默认 false，使用 emptyDir
```

**特点**：
- 无需额外配置
- Pod 重启后状态保留
- Pod 删除后状态丢失
- 适合开发和测试环境

#### 使用 PVC（持久化存储）

```yaml
# values.yaml
server:
  regionDiscovery:
    enabled: true

regionData:
  persistence:
    enabled: true           # 启用 PVC
    storageClass: standard   # StorageClass 名称（可选）
    size: 1Gi               # PVC 大小（默认 1Gi）
    accessMode: ReadWriteOnce # 访问模式（默认 ReadWriteOnce）
    # existingClaim: my-existing-pvc  # 使用已存在的 PVC（可选）
```

**特点**：
- 需要配置 StorageClass
- Pod 删除和重新调度后状态保留
- 适合生产环境
- 支持使用已存在的 PVC

**安装示例**：

```bash
# 使用 emptyDir（默认）
helm install multicloud-exporter ./chart

# 使用 PVC 持久化
helm install multicloud-exporter ./chart \
  --set regionData.persistence.enabled=true \
  --set regionData.persistence.storageClass=standard \
  --set regionData.persistence.size=2Gi

# 使用已存在的 PVC
helm install multicloud-exporter ./chart \
  --set regionData.persistence.enabled=true \
  --set regionData.persistence.existingClaim=my-region-data-pvc
```

**验证持久化**：

```bash
# 1. 检查 PVC 是否创建（仅启用 PVC 时）
kubectl get pvc | grep region-data

# 2. 检查区域状态文件
kubectl exec deployment/multicloud-exporter -- cat /app/data/region_status.json

# 3. 测试跨 Pod 保留：删除 Pod 后检查新 Pod 是否加载旧状态
kubectl delete pod -l app.kubernetes.io/name=multicloud-exporter
kubectl exec deployment/multicloud-exporter -- cat /app/data/region_status.json
```

## 部署模式

### 1. 单机模式 (Single Instance)

默认模式。适用于资源规模较小（API 请求未达限流瓶颈）的场景。
- **配置**：无需额外配置。
- **行为**：单个 Exporter 实例采集 `accounts.yaml` 中定义的所有资源。

### 2. 宿主机集群模式 (Static Sharding)

适用于非 Kubernetes 环境（如 Docker Compose、物理机集群）或网络受限无法使用 DNS 发现的场景。通过环境变量手动指定分片信息。

- **原理**：采用两级分片机制：
  1. **区域级分片**：基于 `fnv32a(AccountID|Region) % Total` 哈希算法，将采集任务按区域分配给不同实例。
  2. **产品级分片**：基于 `fnv32a(AccountID|Region|Namespace) % Total` 哈希算法，将同一区域下的不同产品分配给不同实例。
- **配置**：
  - `EXPORT_SHARD_TOTAL`: 总实例数（如 `3`）
  - `EXPORT_SHARD_INDEX`: 当前实例索引（从 `0` 开始，如 `0`, `1`, `2`）
- **示例** (3 节点集群)：
  - 节点 A: `EXPORT_SHARD_TOTAL=3 EXPORT_SHARD_INDEX=0`
  - 节点 B: `EXPORT_SHARD_TOTAL=3 EXPORT_SHARD_INDEX=1`
  - 节点 C: `EXPORT_SHARD_TOTAL=3 EXPORT_SHARD_INDEX=2`

### 3. Kubernetes 集群模式 (Dynamic Sharding)

推荐模式。利用 Kubernetes Headless Service 实现自动发现与动态分片。

- **原理**：Pod 启动时解析 Headless Service 域名获取所有对等节点 IP，按 IP 排序确定自身索引。支持 StatefulSet 或 Deployment。
- **配置**：
  - 环境变量 `CLUSTER_DISCOVERY=headless`
  - 环境变量 `CLUSTER_SVC=<headless-service-name>`
- **扩缩容**：直接调整 `replicas` 数量，集群会自动重新平衡分片（注意：扩缩容期间可能会有短暂的重复采集或漏采）。

### LB/BWP 指标统一与映射

- 统一映射文件：
  - `configs/mappings/clb.metrics.yaml`：负载均衡
  - `configs/mappings/bwp.metrics.yaml`：共享带宽包
  - `configs/mappings/s3.metrics.yaml`：对象存储 (OSS/COS/S3)（只保留跨云语义最稳的统一指标集合）
  - 带宽：`clb_traffic_rx_bps` ← Aliyun `InstanceTrafficRX`；Tencent `VIntraffic`（`Mbps`→`bit/s`，`scale: 1000000`）
  - 丢失带宽：`clb_drop_traffic_rx_bps` ← Aliyun `DropTrafficRX`；`clb_drop_traffic_tx_bps` ← Aliyun `DropTrafficTX`
  - 包速率/丢包：`clb_packet_rx/tx`、`clb_drop_packet_rx/tx`（Aliyun/Tencent 对齐）
  - 利用率：`clb_traffic_rx_utilization_pct/tx_utilization_pct` ← Tencent `IntrafficVipRatio/OuttrafficVipRatio`
- 监听维度标签：
  - 阿里云 SLB：支持动态维度标签 `port/protocol`，并注入标签服务的 `code_name`；维度选择参考命名空间元数据中的 `dimensions`
  - 腾讯云 CLB：按 `vip` 维度采集；`code_name` 留空
- 快速验证（本地）：
  - `curl -s http://localhost:9101/metrics | grep -E '^clb_traffic_(rx|tx)_bps' | head -n 20`
  - `curl -s http://localhost:9101/metrics | grep -E '^clb_drop_traffic_(rx|tx)_bps' | head -n 20`
  - `curl -s http://localhost:9101/metrics | grep -E '^clb_traffic_(rx|tx)_utilization_pct$' | head -n 20`


### accounts.yaml

```yaml
accounts:
  aliyun:
    - provider: aliyun
      account_id: "aliyun-prod"
      access_key_id: "${ALIYUN_AK}"
      access_key_secret: "${ALIYUN_SK}"
      regions: ["*"]
      resources:
        - bwp
        - clb
        - s3
        - alb
        - nlb
        - gwlb

  tencent:
    - provider: tencent
      account_id: "tencent-prod"
      access_key_id: "${TENCENT_SECRET_ID}"
      access_key_secret: "${TENCENT_SECRET_KEY}"
      regions: ["*"]
      resources:
        - clb
        - bwp
        - s3
```

> regions 配置：
> - `regions: []` 或 `regions: ["*"]` 采集所有区域（AWS/Aliyun/Tencent 均支持自动发现）
> - 指定如 `regions: ["cn-hangzhou", "ap-guangzhou"]` 仅采集列出的区域

> resources 配置：
> - `resources: []` 或 `resources: ["*"]` 采集所有资源类型
> - 指定如 `resources: ["clb", "bwp"]` 仅采集列出的资源类型

## 使用方法

### 本地运行

```bash
cd multicloud-exporter
curl -LO https://github.com/jangrui/multicloud-exporter/releases/latest/download/multicloud-exporter
chmod +x multicloud-exporter
export SERVER_PATH=./configs/server.yaml
# 不设置 PRODUCTS_PATH 时启用自动发现
export ACCOUNTS_PATH=./configs/accounts.yaml
./multicloud-exporter
```

### Docker运行

```bash
docker run -d \
  -p 9101:9101 \
  -v $(pwd)/configs:/app/configs \
  -e ACCOUNTS_PATH=/app/configs/accounts.yaml 
  -e SERVER_PATH=/app/configs/server.yaml 
  multicloud-exporter
```

## 指标格式

### 业务指标

```
multicloud_resource_metric{
  cloud_provider="aliyun",
  account_id="aliyun-prod",
  region="cn-hangzhou",
  resource_type="ecs",
  resource_id="i-xxxxx",
  metric_name="cpu_cores"
} 4
```

### 自身监控指标

```
# API 请求耗时（直方图）
multicloud_request_duration_seconds_bucket{cloud_provider="aliyun", api="DescribeInstances", le="0.1"} 10
multicloud_request_duration_seconds_sum{...} 5.2
multicloud_request_duration_seconds_count{...} 100

# API 限流统计
multicloud_rate_limit_total{cloud_provider="tencent", api="GetMonitorData"} 5

# 采集周期耗时
multicloud_collection_duration_seconds_bucket{le="10"} 1
```

动态命名空间指标（已统一命名为 bwp_*，跨云一致）：

```
bwp_in_utilization_pct{
  cloud_provider="aliyun",
  account_id="xxx",
  region="cn-hangzhou",
  resource_type="bwp",
  resource_id="cbwp-xxx"
} 0.23
```

更多共享带宽指标命名与映射说明，参见 `docs/bwp-metrics.md`。

## Prometheus配置

```yaml
scrape_configs:
  - job_name: 'multicloud'
    static_configs:
      - targets: ['localhost:9101']
```

## 环境变量

- `EXPORTER_PORT`: 监听端口；优先级高于配置文件。未设置则读取 `server.port`，再回退为 `9101`。
- `SCRAPE_INTERVAL`: 采集间隔（默认60s），支持时间格式（如 30s, 1m）。
- `SERVER_PATH`: 指向 `server.yaml`
 
- `ACCOUNTS_PATH`: 指向 `accounts.yaml`
- `DEFAULT_REGIONS`: 当云侧区域枚举失败时的回退区域列表（逗号分隔），例如：`DEFAULT_REGIONS=cn-hangzhou,ap-guangzhou`

## 管理接口认证

可为 `/api/discovery/*` 启用 BasicAuth 认证，推荐使用环境变量与 Kubernetes Secret 管理凭据，避免在 `values.yaml` 或 ConfigMap 中出现明文。

方式一（推荐，Kubernetes 环境）：

1) 创建 Secret，仅包含用户名与密码键：

```bash
kubectl -n monitoring create secret generic multicloud-exporter-admin \
  --from-literal=ADMIN_USERNAME=admin \
  --from-literal=ADMIN_PASSWORD='<secure-password>'
```

2) 在 Helm 值中启用认证并引用该 Secret：

```yaml
env:
  ADMIN_AUTH_ENABLED: "true"

security:
  adminSecretName: "multicloud-exporter-admin"
```

方式二（本地或临时场景）：

```bash
export ADMIN_AUTH_ENABLED=true
export ADMIN_USERNAME=admin
export ADMIN_PASSWORD='<secure-password>'
```

可选：支持通过 `ADMIN_AUTH` 注入多账号，JSON 或逗号分隔均可：

```bash
export ADMIN_AUTH='[{"username":"admin","password":"<secure>"}]'
# 或
export ADMIN_AUTH='admin:<secure>,ops:<secure2>'
```

访问示例：

```bash
curl -u admin:<secure-password> http://<host>:9101/api/discovery/config
```

建议在生产环境通过 Ingress/ServiceMesh 终止 TLS，确保认证信息经由 HTTPS 传输。

## 安全与合规

- 账号凭证请使用环境变量注入，不要在仓库中保存明文密钥：
- 阿里云：`ALIYUN_AK`、`ALIYUN_SK`
- 腾讯云：`TENCENT_SECRET_ID`、`TENCENT_SECRET_KEY`
- 建议本地使用 `accounts.example.yaml` 模板并在 `.gitignore` 中忽略个人 `accounts.local.yaml`。
- 生产环境通过 CI/Secrets 管理凭证并在部署时注入。

## 版本规范

- 发布与安装统一采用带前缀的语义化版本标签：`vX.Y.Z`
- Helm Chart 的 `version` 与 `appVersion` 保持 `v*.*.*`，CI 触发条件匹配 `v*.*.*` 标签

## 采集与缓存

- 采集流程：程序按配置加载产品与指标 → 枚举资源ID（按命名空间映射） → 批量拉取最新监控数据 → 暴露为Prometheus指标。
- 缓存策略：
  - **资源ID缓存**：枚举的资源ID会被缓存，TTL可配置（`discovery_ttl`）。在TTL内的采集轮次直接使用缓存，避免重复枚举导致的API费用与限流。
  - **标签缓存**：资源标签（如 `code_name`）会在首次采集时获取并缓存，同一资源的多个指标复用缓存结果，大幅减少 VPC API 调用（阿里云 ↓90%）。
- 智能分页：为阿里云 CMS API 的 NextToken 分页机制添加三层保护（循环限制、重复token检测、空数据检测），避免因 API bug 导致的无限循环，确保采集稳定性。
- 自动发现：仅监听 `accounts.yaml` 的资源集合变化（`resources`），有变化时触发发现刷新；不再支持周期刷新参数。
- 日志提示：增加采集阶段日志，包括账号/区域开始结束、产品加载、资源缓存命中/枚举数量、每批次拉取点数等，便于排查与观测。

## 性能优化

### 智能区域发现性能优化

对于多区域账号（如阿里云、腾讯云等），启用智能区域发现可以显著提升性能：

- **API 调用减少**：跳过无资源区域，减少 50%-90% 的区域枚举 API 调用
- **采集延迟降低**：优先采集有资源的区域，缩短采集周期
- **云配额节省**：减少云厂商 API 配额消耗，降低成本

**适用场景**：
- 账号下区域数量较多（>5 个区域）
- 部分区域长期无资源或资源稀少
- 采集速度要求较高

**典型收益**（阿里云 20 个区域，仅 3 个区域有资源）：
- API 调用减少约 85%
- 采集周期从 60 秒降低到约 15 秒

### 标签缓存与分页优化

对于指标采集密集的场景（如多个 rx/tx 指标），启用标签缓存和智能分页可以进一步提升性能：

**标签缓存优化**：
- **VPC API 调用减少**：资源标签从每个指标调用一次优化为每个资源调用一次（阿里云 ↓90%）
- **适用场景**：单个资源有多个指标需要采集（如 BWP 的 5 个 rx + 5 个 tx 指标）
- **典型收益**（阿里云 CBWP，10 个指标）：
  - VPC API 调用从 10 次/region 降至 1 次/region（↓90%）
  - 标签获取耗时几乎归零

**智能分页优化**：
- **采集稳定性提升**：为阿里云 CMS API 的 NextToken 分页添加三层保护机制，避免因 API bug 导致的无限循环
- **分页循环减少**：从异常情况下的 60-100 次循环降至正常 3 次循环（↓97%）
- **采集可靠性**：彻底解决 tx 指标因分页阻塞导致的缺失问题（100% 修复）
- **典型收益**（阿里云 eu-central-1 区域）：
  - 单区域采集时间从 50 秒+（超时）降至 1 秒（↓98%）
  - 总采集周期从 30 秒+ 降至 14 秒（↓53%）

## 采集频率与数据周期

配置 `scrape_interval` (采集频率) 与云厂商 API 的 `Period` (数据聚合周期) 的关系至关重要。

### 1. 场景推演

假设 `Period=60s` (云厂商每60s生成一个点)，`scrape_interval=300s` (Exporter每300s采集一次)：

* **T=0s**：云产生数据点 A（覆盖 0~60s）。
* **T=60s**：云产生数据点 B（覆盖 60~120s）。
* ...
* **T=240s**：云产生数据点 E（覆盖 240~300s）。
* **T=300s**：**Exporter 采集**，API 返回**最新**的一个点（即数据点 E）。
* **结果**：数据点 A, B, C, D 永远丢失。

### 2. 存在的风险

* **漏报故障**：如果故障发生在未采集的时间窗口（如 T=100s），监控将无法捕捉。
* **曲线失真**：Prometheus 绘制曲线时，会把相隔 5 分钟的两个点连成直线，忽略了中间的波动。

### 3. 配置策略对比

| 关系 | 现象 | 优缺点 | 适用场景 |
| :--- | :--- | :--- | :--- |
| **Scrape > Period**<br>(300s > 60s) | **数据丢失**<br>(漏采中间的点) | ✅ **省钱**（API 调用少）<br>❌ **有盲区**（可能漏过尖峰） | **非关键指标**<br>（如磁盘空间、每日费用） |
| **Scrape < Period**<br>(15s < 60s) | **数据冗余**<br>(重复采同一个点) | ✅ **全覆盖**（不丢数据）<br>❌ **浪费**（配额与存储） | **不推荐** |
| **Scrape ≈ Period**<br>(60s ≈ 60s) | **完美匹配** | ✅ **无盲区且不浪费** | **核心业务指标**<br>（推荐配置） |

### 4. 测试要求

- 本地（与 CI 保持一致）：`make lint && go test -race -cover ./...`
- 基准测试：`go test -bench . -benchmem -run ^$ ./...`
- 压力测试：`go test -race -run . -parallel 16 ./...`
- CI 强制全局覆盖率 ≥ 80%，目标 ≥ 90%
