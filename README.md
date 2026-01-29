# Multicloud Exporter

面向多云（阿里云、腾讯云、华为云、AWS）的 Prometheus Exporter，按账号/区域/产品/资源维度采集与隔离。

## 关键特性

- 四维架构：Account → Product → Region → Resource，资源级隔离与并发控制
- 统一指标命名：跨云产品语义一致，便于统一看板
- 智能区域发现：自动跳过长期无资源区域，减少 API 调用
- 产品级指标配置：`product_metric` 支持自定义指标与 Period
- 产品级采集频率：`product_metric.scrape_interval` 支持按产品节流
- 集群分片：支持 headless/file 静态或动态分片
- 管理接口可选 BasicAuth：`/api/discovery/*`、`/collect`、`/status`

## 快速开始

### 1. 获取二进制或构建

```bash
# 二进制（示例）
curl -LO https://github.com/jangrui/multicloud-exporter/releases/latest/download/multicloud-exporter-linux-amd64
chmod +x multicloud-exporter-linux-amd64

# 或源码构建
git clone https://github.com/jangrui/multicloud-exporter.git
cd multicloud-exporter
go build -o multicloud-exporter ./cmd/multicloud-exporter
```

### 2. 基础配置

`configs/server.yaml`（最小示例）：

```yaml
server:
  port: 9101
  scrape_interval: 60s
  discovery_ttl: 1h
  region_concurrency: 4
  product_concurrency: 1
  metric_concurrency: 2
  region_discovery:
    enabled: true
    discovery_interval: 1h
    empty_threshold: 30
```

`configs/accounts.yaml`（示例）：

```yaml
accounts:
  aliyun:
    - account_id: "aliyun-prod"
      access_key_id: "${ALIYUN_ACCESS_KEY_ID}"
      access_key_secret: "${ALIYUN_ACCESS_KEY_SECRET}"
      regions: ["*"]
      resources: ["bwp", "clb", "s3", "alb", "nlb", "gwlb"]

  tencent:
    - account_id: "tencent-prod"
      access_key_id: "${TENCENT_SECRET_ID}"
      access_key_secret: "${TENCENT_SECRET_KEY}"
      regions: ["*"]
      resources: ["bwp", "clb", "s3", "gwlb"]

  aws:
    - account_id: "aws-prod"
      access_key_id: "${AWS_ACCESS_KEY_ID}"
      access_key_secret: "${AWS_SECRET_ACCESS_KEY}"
      regions: ["us-east-1"]
      resources: ["s3", "alb"]
```

### 3. 产品级指标与采集频率（可选）

```yaml
accounts:
  aws:
    - account_id: "aws-prod"
      access_key_id: "${AWS_ACCESS_KEY_ID}"
      access_key_secret: "${AWS_SECRET_ACCESS_KEY}"
      regions: []
      resources: ["s3", "alb"]
      product_metric:
        s3:
          - period: 86400
            scrape_interval: 1h
            metric_list: ["BucketSizeBytes", "NumberOfObjects"]
          - period: 60
            metric_list: ["AllRequests", "GetRequests", "PutRequests"]
        alb:
          - period: 60
            scrape_interval: 5m
            metric_list: ["RequestCount", "ActiveConnectionCount"]
```

说明：
- `period` 影响云侧指标聚合周期
- `scrape_interval` 为产品级采集频率（仅本地节流，不做跨 Pod 共享）

### 4. 启动与验证

```bash
./multicloud-exporter

# 健康检查
curl http://localhost:9101/healthz

# 指标
curl http://localhost:9101/metrics | grep multicloud
```

## 配置要点

- `server.scrape_interval` 建议 >= 云侧 `Period`，避免数据点丢失
- `server.discovery_ttl` 控制资源枚举缓存 TTL（纯内存缓存）
- `product_metric` 仅影响对应产品的指标与采集频率

## 集群与分片（简版）

- 动态分片：`CLUSTER_DISCOVERY=headless` + `CLUSTER_SVC=<headless-svc>`
- 静态分片：`CLUSTER_DISCOVERY=file` + `CLUSTER_FILE=<members-file>`
- 集群稳定性与首次采集错峰在 Helm 中配置（见 `chart/README.md`）

## 管理接口认证（可选）

- 启用：`ADMIN_AUTH_ENABLED=true`
- 单账号：`ADMIN_USERNAME` / `ADMIN_PASSWORD`
- 多账号：`ADMIN_AUTH`（JSON 或 `user:pass` 列表）

## 文档索引

- 架构与分片：`docs/architecture.md`
- 发现与缓存：`docs/discovery.md`
- 指标与映射：`docs/metrics-guide.md`
- 故障排查：`docs/troubleshooting.md`
- Helm 部署：`chart/README.md`
