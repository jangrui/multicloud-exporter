# 统一指标映射关系文档

## 文档目的

- 汇总各产品（CLB/ALB/NLB/GWLB/S3/BWP）的统一指标命名、描述与平台映射
- 说明指标映射分类规则（全映射、部分映射、单独指标）
- 作为 `configs/mappings/*.yaml` 配置的结构说明与审阅入口

## 映射分类定义

| 分类 | 定义 | 说明 |
|------|------|------|
| 全映射（4家） | 阿里云、腾讯云、AWS、华为云都支持 | 跨云厂商完全统一的指标 |
| 全映射（3家） | 任意 3 家云厂商支持 | 部分云厂商不提供该指标 |
| 全映射（2家） | 任意 2 家云厂商支持 | 仅少数云厂商支持 |
| 部分映射 | 同一指标有 2-3 家云厂商支持 | 参见全映射（2/3家） |
| 单独指标 | 仅 1 家云厂商支持 | 云厂商特有指标 |

## 配置结构

- 顶层字段：
  - `prefix`：统一指标前缀，如 `clb`、`alb`、`s3`
  - `namespaces`：云厂商命名空间映射，如 `aliyun: acs_slb_dashboard`
  - `canonical`：统一指标集合，键为统一指标名，值为条目
- 条目字段（canonical entry）：
  - `description`：指标中文描述，准确反映业务含义与技术定义
  - `aliyun`/`tencent`/`aws`/`huawei`：平台原始指标定义，含 `metric`、`dimensions`、`unit`、`scale`

## 指标文件组织规则

`configs/mappings/*.yaml` 文件按以下规则组织：

1. 全映射指标（4 家云厂商支持）
2. 部分映射指标（3 家云厂商）
3. 部分映射指标（2 家云厂商）
4. 单独指标（按云厂商分类：AWS 专用 / 阿里云专用 / 腾讯云专用 / 华为云专用）
5. 各分类内指标按字母顺序排序

## 指标覆盖统计

| 产品 | 全映射(4) | 全映射(3) | 全映射(2) | 单独指标 | 合计 |
|------|-----------|-----------|-----------|----------|------|
| CLB | 0 | 3 | 6 | 43 | 52 |
| ALB | 0 | - | 8 | 131 | 139 |
| NLB | 0 | - | 4 | 33 | 37 |
| GWLB | 0 | 3 | 2 | 6 | 11 |
| S3 | 6 | - | 4 | 37 | 47 |
| BWP | 0 | - | 6 | 2 | 10 |
| **合计** | **6** | **6** | **30** | **252** | **294** |

> 说明：严格意义的全映射仅 S3 的 6 个指标，其他产品因各云厂商 API 限制无法实现全映射。

## 各产品核心指标集合

### CLB（configs/mappings/clb.metrics.yaml）

**全映射（3家）：**
- `active_connection`：活跃连接数（count）
- `healthy_server_count`：健康后端主机数（count）
- `unhealthy_server_count`：异常后端主机数（count）

**部分映射（3家）：**
- `drop_connection`：丢弃连接数（count/s）
- `drop_packet_rx`：入网丢包速率（count/s）
- `drop_packet_tx`：出网丢包速率（count/s）
- `packet_rx`：入网包速率（count/s）
- `packet_tx`：出网包速率（count/s）
- `qps`：每秒请求数（count/s）
- `rt`：响应时间（ms）
- `status_code_2xx`：HTTP 2XX 状态码数量
- `status_code_4xx`：HTTP 4XX 状态码数量
- `status_code_5xx`：HTTP 5XX 状态码数量
- `traffic_rx_bps`：入网流量（bit/s）
- `traffic_tx_bps`：出网流量（bit/s）

**部分映射（2家）：**
- `drop_traffic_rx_bps`：入网丢弃流量（bit/s）
- `drop_traffic_tx_bps`：出网丢弃流量（bit/s）
- `new_connection`：新建连接数（count/s）
- `status_code_3xx`：HTTP 3XX 状态码数量
- `traffic_rx_utilization_pct`：入网带宽利用率（percent）
- `traffic_tx_utilization_pct`：出网带宽利用率（percent）

**单独指标：**
- AWS 专用：6 个（如 `surge_queue_length`、`spillover_count` 等）
- 阿里云专用：20 个（实例级 HTTP 状态码、请求类型等）
- 腾讯云专用：15 个（协议级连接数、QPS 等）

### ALB（configs/mappings/alb.metrics.yaml）

**全映射（2家）：**
- `active_connection`：活跃连接数（count）
- `new_connection`：新建连接数（count/s）
- `qps`：每秒请求数（count/s）
- `status_code_2xx`：HTTP 2XX 状态码数量
- `status_code_3xx`：HTTP 3XX 状态码数量
- `status_code_4xx`：HTTP 4XX 状态码数量
- `status_code_5xx`：HTTP 5XX 状态码数量
- `traffic_rx_bps`：入网流量（bit/s）

**单独指标：**
- 阿里云专用：119 个（Listener/VIP/ServerGroup/Rule/DualStack 等多维度指标）
- AWS 专用：12 个（LCU 消耗、Target 健康/错误等）

### NLB（configs/mappings/nlb.metrics.yaml）

**全映射（2家）：**
- `active_connection`：活跃连接数（count）
- `listener_healthy_server_count`：监听器健康后端数（count）
- `listener_unhealthy_server_count`：监听器异常后端数（count）
- `new_connection`：新建连接数（count/s）

**单独指标：**
- 阿里云专用：29 个（Instance/Listener/VIP 维度的包速率、丢包等）
- AWS 专用：14 个（TCP/TLS/UDP 协议、端口分配、处理字节/包数等）

### GWLB（configs/mappings/gwlb.metrics.yaml）

**全映射（3家）：**
- `active_connection`：活跃连接数（count）
- `new_connection`：新建连接数（count/s）
- `unhealthy_server_count`：异常后端服务数（count）

**部分映射（2家）：**
- `healthy_server_count`：健康后端服务数（count）
- `traffic_rx_bps`：入网流量（bit/s）
- `traffic_tx_bps`：出网流量（bit/s）

**单独指标：**
- 阿里云专用：2 个（packet_rx、packet_tx）
- AWS 专用：4 个（LCU 消耗、跨可用区负载均衡、处理字节/包数等）

### S3（configs/mappings/s3.metrics.yaml）

**全映射（4家）：**
- `requests_get`：GET 请求数（count）
- `requests_head`：HEAD 请求数（count）
- `requests_put`：PUT 请求数（count）
- `requests_total`：总请求数（count）
- `storage_usage_bytes`：存储空间使用量（Bytes）
- `traffic_internet_rx_bytes`：公网入流量（Bytes）
- `traffic_internet_tx_bytes`：公网出流量（Bytes）

**部分映射（3家）：**
- `availability_pct`：可用性/成功率（percent）
- `response_server_error_count`：服务端错误数（count）

**部分映射（2家）：**
- `latency_e2e_get_ms`：GET 端到端延迟（ms）
- `latency_first_byte_ms`：首字节延迟（ms）
- `traffic_intranet_rx_bytes`：内网入流量（Bytes）
- `traffic_intranet_tx_bytes`：内网出流量（Bytes）

**单独指标：**
- 阿里云专用：19 个（授权错误、客户端超时、延迟、CDN、复制流量等）
- 腾讯云专用：16 个（归档存储、响应码占比、QPS 等）
- AWS 专用：16 个（对象数量、SELECT、复制延迟/操作等）

### BWP（configs/mappings/bwp.metrics.yaml）

**全映射（2家）：**
- `packet_rx`：入网包速率（count/s）
- `packet_tx`：出网包速率（count/s）
- `traffic_rx_bps`：入网流量（bit/s）
- `traffic_tx_bps`：出网流量（bit/s）
- `utilization_rx_pct`：入网带宽利用率（percent）
- `utilization_tx_pct`：出网带宽利用率（percent）

**单独指标：**
- 阿里云专用：2 个（drop_rx_pps、drop_tx_pps）

## 标签规范

所有产品统一标签：
- `cloud_provider`：云厂商标识（aliyun/tencent/aws/huawei）
- `account_id`：账号标识
- `region`：区域标识
- `resource_type`：资源类型（clb/alb/nlb/gwlb/s3/bwp）
- `resource_id`：资源 ID（负载均衡器 ID、Bucket 名称等）

**产品特有标签：**
- CLB：`code_name`（阿里云实例名称）、`port`（监听端口）、`protocol`（协议）
- ALB：`listener_protocol`/`listener_port`、`vip`、`server_group_id`、`rule_id` 等
- NLB：`listener_protocol`/`listener_port`、`vip` 等
- GWLB：`address_ip_version`、`listener_id`、`server_group_id` 等
- S3：`storage_type`、`filter_id`、`replication_rule_id` 等
- BWP：`bandwidth_package_id` 等

## 术语与格式规范

- 缩进：YAML 统一使用 2 空格缩进
- 编码：文件统一使用 UTF-8
- 命名规范：`<name>_<unit>`（如 `traffic_rx_bps`、`latency_e2e_get_ms`）
- 描述规范：简洁一致的中文描述，包含关键语义与单位
- 单位转换：使用 `scale` 字段统一单位（如 MB → Bytes 使用 `scale: 1048576`）

## 参考实现

- 解析代码：`internal/config/mappings.go`
  - `MetricDef`/`CanonicalEntry`/`MetricMapping` 结构体
  - 别名与单位缩放注册：调用 `internal/metrics` 中的注册函数
- 指标暴露：`internal/metrics/collector.go`
  - 根据 `canonical` 配置生成统一的 Prometheus 指标
  - 自动处理单位转换（`scale`）和维度映射
