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
  - `cluster.stabilityCheck.enabled`：是否启用集群稳定性检测（默认 `true`）
  - `cluster.stabilityCheck.maxWait`：最长等待时间（默认 `30s`）
  - `cluster.stabilityCheck.checkInterval`：检查间隔（默认 `2s`）
  - `cluster.stabilityCheck.requiredStable`：需要连续稳定的次数（默认 `3`）

- 首次采集策略（智能错峰）
  - `firstRun.strategy`：首次采集策略（默认 `auto`）
    - `auto`：自动判断（单 Pod 立即，2-10 个 Pod 线性延迟，>10 个 Pod 指数退避）
    - `immediate`：强制所有 Pod 立即采集
    - `staggered`：强制线性延迟，均匀分布
  - `firstRun.maxDelay`：首次采集最大延迟秒数（默认 `180`）

- 区域发现配置（智能区域选择）
  - `regionDiscovery.enabled`：是否启用智能区域发现（默认 `true`）
  - `regionDiscovery.discoveryInterval`：重新发现周期（默认 `1h`）
  - `regionDiscovery.emptyThreshold`：连续空次数阈值（默认 `3`）
  - `regionDiscovery.dataDir`：数据目录路径（默认 `/app/data`）
  - `regionDiscovery.persistFile`：持久化文件名（默认 `region_status.json`）

- 华为云缓存配置（限流优化）
  - `huaweiCache.enabled`：是否启用华为云缓存（默认 `true`）
  - `huaweiCache.resourceTTL`：资源缓存 TTL（默认 `10m`）
  - `huaweiCache.tagTTL`：标签缓存 TTL（默认 `30m`）

- 区域数据持久化配置
  - `regionData.persistence.enabled`：是否启用 PVC 持久化存储（默认 `false`，使用 emptyDir）
  - `regionData.persistence.storageClass`：StorageClass 名称（留空使用集群默认）
  - `regionData.persistence.size`：PVC 大小（默认 `1Gi`）
  - `regionData.persistence.accessMode`：访问模式（默认 `ReadWriteOnce`）
  - `regionData.persistence.existingClaim`：使用已存在的 PVC 名称（可选）

 - 环境变量
   - `values.env`：按需覆盖运行环境变量（如 `SCRAPE_INTERVAL`）
   - `ALIYUN_INSTANCE_BATCH_SIZE`：阿里云实例级采集每批次维度数量（默认 50，范围 1-200）
 
 - 安全配置
   - `security.adminSecretName`：管理认证 Secret 名称；若设置，将以 `envFrom.secretRef` 注入 Secret 中的键（如 `ADMIN_USERNAME`、`ADMIN_PASSWORD`）

 - 配置文件
   - `server.yaml`：默认使用镜像内置配置；如需覆盖，提供 `values.server`，Chart 将渲染为 ConfigMap 并挂载到容器固定路径
     - 支持通过 `values.server.period_fallback` 配置 Period Fallback 值（默认 60 秒）
     - 支持通过环境变量 `PERIOD_FALLBACK` 覆盖（如果同时设置，环境变量优先）
  - `products.yaml`：已废弃；Exporter 采用自动发现机制
  - `accounts.yaml`：由用户预创建 Secret 提供并挂载到容器固定路径

- 账号 Secret 引用
  - `accounts.grouped`：按云平台分组的账号配置（推荐），Chart 会生成 `accounts.yaml` 并注入对应环境变量
    - 支持在账号下配置 `product_metric`，透传到 `accounts.yaml` 的 `product_metric`

    ```yaml
    accounts:
      grouped:
        aliyun:
          - name: aliyun-acc-0  # Secret 名称
            regions: ["*"]
            resources: ["bwp", "clb", "s3", "alb", "nlb", "gwlb"]
            product_metric:
              s3:
                - period: 3600
                  scrape_interval: 1h
                  metric_list: ["UserStorage", "ObjectNumber"]
              bwp:
                - period: 600
                  scrape_interval: 10m
                  metric_list: ["InBandwidth", "OutBandwidth"]
        tencent:
          - name: tencent-acc-0
            regions: ["*"]
            resources: ["bwp", "clb", "s3"]
        aws:
          - name: aws-acc-0
            # AWS 的 S3 采集使用全局接口（ListBuckets），regions 可留空或使用 ["*"]
            regions: ["*"]
            resources: ["s3"]
    ```

    对应的 Secret 创建示例：
    ```bash
    # 阿里云账号
    kubectl -n monitoring create secret generic aliyun-acc-0 \
      --from-literal=account_id=xxx \
      --from-literal=access_key_id=xxx \
      --from-literal=access_key_secret=xxx

    # 腾讯云账号
    kubectl -n monitoring create secret generic tencent-acc-0 \
      --from-literal=account_id=xxx \
      --from-literal=access_key_id=xxx \
      --from-literal=access_key_secret=xxx

    # AWS 账号
    kubectl -n monitoring create secret generic aws-acc-0 \
      --from-literal=account_id=xxx \
      --from-literal=access_key_id=xxx \
      --from-literal=access_key_secret=xxx
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

### 管理接口认证（安全）

为 `/api/discovery/*` 启用 BasicAuth 时，推荐通过环境变量开关 + Secret 管理凭据：

```bash
kubectl -n monitoring create secret generic multicloud-exporter-admin \
  --from-literal=ADMIN_USERNAME=admin \
  --from-literal=ADMIN_PASSWORD='<secure-password>'
```

Helm 值示例：

```yaml
env:
  ADMIN_AUTH_ENABLED: "true"

security:
  adminSecretName: "multicloud-exporter-admin"
```

本地或临时场景可直接设置环境变量：

```bash
export ADMIN_AUTH_ENABLED=true
export ADMIN_USERNAME=admin
export ADMIN_PASSWORD='<secure-password>'
```

如需多个账号，支持 `ADMIN_AUTH`（JSON 或 `user:pass,user2:pass2`）。

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

## 故障排查指南

### 多副本场景常见问题

#### 1. 滚动更新时出现重复采集

**症状**：在滚动更新期间，Prometheus 中出现重复的指标数据点。

**原因**：新旧 Pod 同时运行时，集群拓扑不稳定，导致分片计算不一致。

**解决方案**：
- 确认 `cluster.stabilityCheck.enabled=true`（默认已启用）
- 检查 Pod 日志，确认稳定性检测是否正常工作：
  ```bash
  kubectl logs -f deployment/multicloud-exporter | grep "cluster stable"
  ```
- 如果稳定性检测超时，可以适当增加 `cluster.stabilityCheck.maxWait`：
  ```yaml
  cluster:
    stabilityCheck:
      maxWait: 60s  # 从默认 30s 增加到 60s
  ```

#### 2. 首次采集时云 API 限流

**症状**：多个 Pod 同时启动后，日志中出现大量限流错误（如 `Throttling`, `RateLimit`）。

**原因**：所有 Pod 同时开始首次采集，瞬间向云厂商 API 发起大量请求。

**解决方案**：
- 确认 `firstRun.strategy=auto`（默认已启用）
- 对于大规模部署（>10 个 Pod），可以增加最大延迟：
  ```yaml
  firstRun:
    strategy: auto
    maxDelay: 300  # 从默认 180s 增加到 300s
  ```
- 对于极度敏感的场景，可以强制使用线性延迟：
  ```yaml
  firstRun:
    strategy: staggered
    maxDelay: 180
  ```

#### 3. 华为云 API 频繁限流

**症状**：华为云账号的指标采集频繁失败，日志中出现 `APIGW.0308` 或 `throttling` 错误。

**原因**：华为云的 API 限流策略较为严格。

**解决方案**：
- 确认华为云缓存已启用（默认已启用）：
  ```yaml
  huaweiCache:
    enabled: true
    resourceTTL: 10m
    tagTTL: 30m
  ```
- 如果仍然限流，可以增加缓存 TTL：
  ```yaml
  huaweiCache:
    enabled: true
    resourceTTL: 30m  # 从 10m 增加到 30m
    tagTTL: 1h        # 从 30m 增加到 1h
  ```
- 检查限流统计指标：
  ```promql
  multicloud_rate_limit_total{cloud_provider="huawei"}
  ```

#### 4. 区域状态在 Pod 重启后丢失

**症状**：Pod 重启后，之前跳过的空区域又重新开始采集，导致采集时间变长。

**原因**：默认使用 emptyDir，Pod 删除后数据丢失。

**解决方案**：
- 启用 PVC 持久化存储：
  ```yaml
  regionData:
    persistence:
      enabled: true
      storageClass: standard
      size: 1Gi
  ```
- 验证 PVC 是否创建成功：
  ```bash
  kubectl get pvc | grep region-data
  ```
- 检查区域状态文件：
  ```bash
  kubectl exec deployment/multicloud-exporter -- cat /app/data/region_status.json
  ```

#### 5. 集群拓扑发现失败

**症状**：Pod 日志中出现 `failed to resolve headless service` 或 `cluster discovery timeout`。

**原因**：Headless Service 配置错误或 DNS 解析失败。

**解决方案**：
- 检查 Headless Service 是否创建：
  ```bash
  kubectl get svc | grep headless
  ```
- 检查 Service 名称是否正确：
  ```yaml
  cluster:
    discovery: headless
    svcName: multicloud-exporter-headless  # 确保与实际 Service 名称一致
  ```
- 测试 DNS 解析：
  ```bash
  kubectl exec deployment/multicloud-exporter -- nslookup multicloud-exporter-headless
  ```

### 监控与告警

建议配置以下 Prometheus 告警规则：

```yaml
groups:
  - name: multicloud-exporter
    rules:
      # 集群配置刷新失败
      - alert: ClusterConfigRefreshFailed
        expr: rate(multicloud_cluster_config_refresh_total{status="error"}[5m]) > 0
        for: 5m
        annotations:
          summary: "集群配置刷新失败"
          description: "Pod {{ $labels.pod }} 集群配置刷新失败"

      # API 限流告警
      - alert: HighRateLimitRate
        expr: rate(multicloud_rate_limit_total[5m]) > 1
        for: 5m
        annotations:
          summary: "云 API 限流频繁"
          description: "{{ $labels.cloud_provider }} API 限流频繁，可能需要调整采集策略"

      # 采集周期超时
      - alert: CollectionTimeout
        expr: multicloud_collection_duration_seconds > 300
        for: 5m
        annotations:
          summary: "采集周期超时"
          description: "采集周期耗时 {{ $value }}s，超过 5 分钟"

      # 区域跳过率过高
      - alert: HighRegionSkipRate
        expr: sum(multicloud_region_status{status="skipped"}) / sum(multicloud_region_status) > 0.8
        for: 10m
        annotations:
          summary: "区域跳过率过高"
          description: "超过 80% 的区域被跳过，可能需要检查区域配置"
```
