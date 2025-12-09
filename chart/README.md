# Multicloud Exporter Helm Chart

用于在 Kubernetes 中部署 `multicloud-exporter` 的 Helm Chart，暴露 Metrics 指标。

## 快速开始

```bash
kubectl -n monitoring create secret generic aliyun-accounts \
  --from-literal=account_id=xxx \
  --from-literal=access_key_id=xxx \
  --from-literal=access_key_secret=xxxx

helm repo add jangrui https://jangrui.com/chart --force-update

helm -n monitoring upgrade -i multicloud-exporter jangrui/multicloud-exporter --version v0.1.6

# 检查
kubectl -n monitoring get po,svc -l app.kubernetes.io/name=multicloud-exporter
```

默认监听 `9101` 端口并创建 `ClusterIP` Service，采集间隔为 `60s`。

## 自身监控

Exporter 暴露了 `/metrics` 端点，其中包含自身运行状态指标：
- `multicloud_request_duration_seconds`: API 请求耗时
- `multicloud_rate_limit_total`: API 限流次数
- `multicloud_collection_duration_seconds`: 采集周期总耗时

建议在 Prometheus 中配置相应的告警规则（如限流激增、采集超时）。

## 采集分片 (Sharding)

为支持大规模资源采集，Chart 支持两种分片模式：

### 1. 动态分片 (推荐)

利用 Kubernetes Headless Service 进行自动发现与分片。所有 Pod 自动组成集群，无需手动指定索引。

- **配置**：
  ```yaml
  replicaCount: 3        # 副本数即分片数
  headless:
    enabled: true        # 启用 Headless Service
  cluster:
    discovery: headless  # 开启 DNS 自动发现
    svcName: multicloud-exporter-headless # 对应 Headless Service 名称
  ```
- **扩缩容**：直接修改 `replicaCount`，集群会自动重新平衡分片。

### 2. 静态分片

适用于网络受限或无法使用 DNS 发现的场景。需要手动部署多个 Release，每个 Release 负责一个固定的分片索引。

- **配置**：
  ```yaml
  cluster:
    discovery: ""        # 关闭自动发现
    sharding:
      enabled: true
      total: 3           # 总分片数
      index: 0           # 当前分片索引 (0 ~ total-1)
  ```
- **部署示例** (部署 2 个分片)：
  ```bash
  # 分片 0
  helm install exporter-0 jangrui/multicloud-exporter \
    --set cluster.sharding.enabled=true \
    --set cluster.sharding.total=2 \
    --set cluster.sharding.index=0

  # 分片 1
  helm install exporter-1 jangrui/multicloud-exporter \
    --set cluster.sharding.enabled=true \
    --set cluster.sharding.total=2 \
    --set cluster.sharding.index=1
  ```

## 参数

- 镜像
  - `image.registry`：镜像注册中心（默认 `ghcr.io/jangrui`）
  - `image.repository`：镜像仓库（默认 `multicloud-exporter`）
  - `image.tag`：镜像标签（默认 `Chart.AppVersion`）
  - `image.pullPolicy`：镜像拉取策略（默认 `IfNotPresent`）

- 服务
  - `service.port`：容器与服务端口（默认 `9101`）
  - `service.type`：Service 类型（默认 `ClusterIP`）
  - `headless.enabled`：是否启用 Headless Service (用于动态分片)

- 集群与分片
  - `cluster.discovery`：发现模式 (`headless` / `file` / `""`)
  - `cluster.svcName`：Headless Service 名称 (当 discovery=headless)
  - `cluster.sharding.enabled`：是否启用静态分片配置
  - `cluster.sharding.total`：静态分片总数
  - `cluster.sharding.index`：静态分片索引

- 环境变量
  - `values.env`：按需覆盖运行环境变量（如 `SCRAPE_INTERVAL`）

- 配置文件
  - `server.yaml`：由 `values.server` 渲染为 ConfigMap 并挂载到容器固定路径
  - `products.yaml`：已废弃；Exporter 采用自动发现机制
  - `accounts.yaml`：由用户预创建 Secret 提供并挂载到容器固定路径

-- 账号 Secret 引用
  - `accounts.secrets`：分散的每账号 Secret 列表，Chart 会生成 `accounts.yaml` 占位符并注入对应环境变量

    ```yaml
    accounts:
      aliyun:
        - provider: aliyun
          account_id: "aliyun-prod"
          access_key_id: "${ALIYUN_AK}"
          access_key_secret: "${ALIYUN_SK}"
          regions: ["*"]
          resources:
            - cbwp
            - slb
            - oss

      tencent:
        - provider: tencent
          account_id: "tencent-prod"
          access_key_id: "${TENCENT_SECRET_ID}"
          access_key_secret: "${TENCENT_SECRET_KEY}"
          regions: ["*"]
          resources:
            - bwp
            - clb
            - cos
    ```

- 调度与资源
  - `resources`：容器资源限制与请求
  - `nodeSelector`、`tolerations`、`affinity`：节点选择与亲和/容忍

## 升级与卸载

- 升级：
  ```bash
  helm -n monitoring upgrade multicloud-exporter jangrui/multicloud-exporter
  ```

- 卸载：
  ```bash
  helm -n monitoring uninstall multicloud-exporter
  ```

## 配置最佳实践

### 采集频率与数据周期

配置 `server.scrape_interval` (采集频率) 与云厂商 API 的 `Period` (数据聚合周期) 的关系至关重要。Exporter 在未显式配置时会自动从云侧元数据选择该指标的最小可用 `Period`。

#### 1. 场景推演

假设 `Period=60s` (云厂商每60s生成一个点)，`scrapeInterval=300` (Exporter每300s采集一次)：

* **T=0s**：云产生数据点 A（覆盖 0~60s）。
* **T=60s**：云产生数据点 B（覆盖 60~120s）。
* ...
* **T=240s**：云产生数据点 E（覆盖 240~300s）。
* **T=300s**：**Exporter 采集**，API 返回**最新**的一个点（即数据点 E）。
* **结果**：数据点 A, B, C, D 永远丢失。

#### 2. 存在的风险

* **漏报故障**：如果故障发生在未采集的时间窗口（如 T=100s），监控将无法捕捉。
* **曲线失真**：Prometheus 绘制曲线时，会把相隔 5 分钟的两个点连成直线，忽略了中间的波动。

#### 3. 配置策略对比

| 关系 | 现象 | 优缺点 | 适用场景 |
| :--- | :--- | :--- | :--- |
| **Scrape > Period**<br>(300s > 60s) | **数据丢失**<br>(漏采中间的点) | ✅ **省钱**（API 调用少）<br>❌ **有盲区**（可能漏过尖峰） | **非关键指标**<br>（如磁盘空间、每日费用） |
| **Scrape < Period**<br>(15s < 60s) | **数据冗余**<br>(重复采同一个点) | ✅ **全覆盖**（不丢数据）<br>❌ **浪费**（配额与存储） | **不推荐** |
| **Scrape ≈ Period**<br>(60s ≈ 60s) | **完美匹配** | ✅ **无盲区且不浪费** | **核心业务指标**<br>（推荐配置） |

## 备注

- 建议将敏感配置通过 Secret 管理，避免直接提交到版本库。
- Chart 支持以 `v*.*.*` 的版本标签进行安装与升级；请确保 Helm 3.x。
- Exporter 通过监听 `accounts.yaml` 的资源集合变化触发发现刷新。
