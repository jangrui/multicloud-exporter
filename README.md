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

## 支持的资源类型

### 阿里云
- [x] ecs - ECS实例
- [ ] rds - RDS数据库
- [ ] redis - Redis缓存
- [ ] slb - 负载均衡
- [ ] eip - 弹性公网IP
- [x] bwp - 共享带宽包
- [ ] nat - NAT网关
- [ ] oss - 对象存储
- [ ] cdn - CDN
- [ ] vpc - 专有网络
- [ ] disk - 云盘

### 华为云
- [ ] ecs - ECS实例
- [ ] rds - RDS数据库
- [ ] redis - Redis缓存
- [ ] elb - 弹性负载均衡
- [ ] eip - 弹性公网IP
- [ ] nat - NAT网关
- [ ] obs - 对象存储
- [ ] cdn - CDN
- [ ] vpc - 虚拟私有云
- [ ] evs - 云硬盘

### 腾讯云
- [ ] cvm - CVM实例
- [ ] cdb - 云数据库MySQL
- [ ] redis - Redis缓存
- [ ] clb - 负载均衡
- [ ] eip - 弹性公网IP
- [ ] nat - NAT网关
- [ ] cos - 对象存储
- [ ] cdn - CDN
- [ ] vpc - 私有网络
- [ ] cbs - 云硬盘

## 配置文件

采用拆分配置，位于 `configs/` 目录；也可通过环境变量指定任意路径。

### server.yaml

```yaml
server:
  port: 9101
  page_size: 1000
  discovery_ttl: "1h"
  discovery_refresh: ""
```

### products.yaml

```yaml
products:
  aliyun:
    - namespace: acs_bandwidth_package
      period: 60
      metric_info:
        - metric_list:
          - in_bandwidth_utilization
          - out_bandwidth_utilization
          - net_rx.rate
          - net_tx.rate
          - net_rx.Pkgs
          - net_tx.Pkgs
          - in_ratelimit_drop_pps
          - out_ratelimit_drop_pps
  huawei: []
  tencent: []
```

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
  huawei: []
  tencent: []
```

> regions 配置：
> - `regions: []` 或 `regions: ["*"]` 采集所有区域
> - 指定如 `regions: ["cn-hangzhou", "ap-guangzhou"]` 仅采集列出的区域

> resources 配置：
> - `resources: []` 或 `resources: ["*"]` 采集所有资源类型
> - 指定如 `resources: ["ecs", "bwp"]` 仅采集列出的资源类型

## 使用方法

### 本地运行

```bash
cd multicloud-exporter
curl -LO https://github.com/jangrui/multicloud-exporter/releases/latest/download/multicloud-exporter
chmod +x multicloud-exporter
export SERVER_PATH=./configs/server.yaml
export PRODUCTS_PATH=./configs/products.yaml
export ACCOUNTS_PATH=./configs/accounts.yaml
./multicloud-exporter
```

### Docker运行

```bash
docker run -d \
  -p 9101:9101 \
  -v $(pwd)/configs:/app/configs \
  -e ACCOUNTS_PATH=/app/configs/accounts.yaml \
  -e PRODUCTS_PATH=/app/configs/products.yaml \
  -e SERVER_PATH=/app/configs/server.yaml \
  multicloud-exporter
```

## 指标格式

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
- `SCRAPE_INTERVAL`: 采集间隔秒数（默认60）
- `SERVER_PATH`: 指向 `server.yaml`
- `PRODUCTS_PATH`: 指向 `products.yaml`
- `ACCOUNTS_PATH`: 指向 `accounts.yaml`

## 采集与缓存

- 采集流程：程序按配置加载产品与指标 → 枚举资源ID（按命名空间映射） → 批量拉取最新监控数据 → 暴露为Prometheus指标。
- 缓存策略：枚举的资源ID会被缓存，TTL可配置（`discovery_ttl`）。在TTL内的采集轮次直接使用缓存，避免重复枚举导致的API费用与限流。
- 日志提示：增加采集阶段日志，包括账号/区域开始结束、产品加载、资源缓存命中/枚举数量、每批次拉取点数等，便于排查与观测。
