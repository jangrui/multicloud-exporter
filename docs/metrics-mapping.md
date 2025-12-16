# 统一指标映射关系文档

梳理并对齐 `configs/mappings/*.yaml` 的统一指标命名、描述与平台映射。

## 文档目的

- 汇总各产品（ALB/NLB/GWLB/CLB/S3）的统一指标命名（canonical），说明含义与单位
- 指明各云厂商原始指标与维度映射关系
- 作为 YAML 配置的结构说明与审阅入口

## 配置结构

- 顶层字段：
  - `prefix`：统一指标前缀，如 `clb`、`alb`、`s3`
  - `namespaces`：云厂商命名空间映射，如 `aliyun: acs_slb_dashboard`
  - `canonical`：统一指标集合，键为统一指标名，值为条目
- 条目字段（canonical entry）：
  - `description`：指标中文描述，准确反映业务含义与技术定义
  - `aliyun`/`tencent`：平台原始指标定义，含 `metric`、`dimensions`、`unit`、`scale`

## 指标汇总（核心集合）

### CLB（configs/mappings/clb.metrics.yaml）

- `clb_traffic_rx_bps`：入网流量（bit/s）
- `clb_traffic_tx_bps`：出网流量（bit/s）
- `clb_packet_rx`：入网包速率（count/s）
- `clb_packet_tx`：出网包速率（count/s）
- `clb_drop_packet_rx`：入网丢包速率（count/s）
- `clb_drop_packet_tx`：出网丢包速率（count/s）
- `clb_drop_traffic_rx_bps`：入网丢弃流量（bit/s）
- `clb_drop_traffic_tx_bps`：出网丢弃流量（bit/s）
- `clb_traffic_rx_utilization_pct`：入网带宽利用率（percent）
- `clb_traffic_tx_utilization_pct`：出网带宽利用率（percent）
- `clb_qps`：每秒请求数（count/s，Aliyun）
- `clb_rt`：响应时间（ms，Aliyun）
- `clb_status_code_2xx`：HTTP 2XX 状态码数量（count/s，Aliyun）

### NLB（configs/mappings/nlb.metrics.yaml）

- `nlb_active_connection`：活跃连接数（count）
- `nlb_new_connection`：新建连接数（count/s）
- `nlb_dropped_connection`：丢弃连接数（count/s）
- `nlb_traffic_rx_bps`：入网流量（bit/s）
- `nlb_traffic_tx_bps`：出网流量（bit/s）
- `nlb_packet_rx`：入网包速率（count/s）
- `nlb_packet_tx`：出网包速率（count/s）
- `nlb_listener_*`：监听器维度的健康数、包速率、丢包、流量等（按名称含义）
- `nlb_instance_*`：实例维度的连接、丢包、流量等（按名称含义）
- `nlb_vip_*`：VIP 维度的连接、丢包、流量等（按名称含义）

### GWLB（configs/mappings/gwlb.metrics.yaml）

- `gwlb_active_connection`：活跃连接数（count）
- `gwlb_new_connection`：新建连接数（count/s）
- `gwlb_traffic_rx_bps`：入网流量（bit/s）
- `gwlb_traffic_tx_bps`：出网流量（bit/s）
- `gwlb_packet_rx`：入网包速率（count/s）
- `gwlb_packet_tx`：出网包速率（count/s）
- `gwlb_unhealthy_server_count`：异常后端服务数（count）
- `gwlb_healthy_server_count`：健康后端服务数（count）

### ALB（configs/mappings/alb.metrics.yaml）

- `alb_active_connection`/`alb_new_connection`/`alb_dropped_connection`
- `alb_traffic_rx_bps`/`alb_traffic_tx_bps`
- `alb_qps`/`alb_rt`
- `alb_status_code_*`：HTTP 状态码数量（2xx/3xx/4xx/5xx）
- `alb_listener_*`：监听器维度的 HTTP 状态码、流量、连接、健康数、重定向/固定响应、上游错误与响应时间等
- `alb_dual_stack_*`：双栈（IPv4/IPv6）下 Listener/VIP/ServerGroup/Rule 的对应指标（名称即含义）

备注：ALB 指标数量较多，完整集合以 YAML 为准，均已补充 `description` 字段。

### S3（configs/mappings/s3.metrics.yaml）

- `s3_storage_usage_bytes`：存储空间使用量（Bytes）
- `s3_traffic_internet_rx_bytes`/`s3_traffic_internet_tx_bytes`：公网入/出流量（Bytes）
- `s3_traffic_intranet_rx_bytes`/`s3_traffic_intranet_tx_bytes`：内网入/出流量（Bytes）
- `s3_traffic_cdn_rx_bytes`/`s3_traffic_cdn_tx_bytes`：CDN 回源入/出流量（Bytes）
- `s3_requests_total`：总请求数（count）
- `s3_availability_pct`：可用性/成功率（percent）
- `s3_response_server_error_count`：服务端错误数（count）
- `s3_latency_e2e_get_ms`/`s3_latency_server_get_ms`：GET 延迟（ms）
- `s3_latency_e2e_put_ms`/`s3_latency_server_put_ms`：PUT 延迟（ms）
- `s3_traffic_replication_bytes`：跨区域复制流量（Bytes）

## 术语与格式规范

- 缩进：YAML 统一使用 2 空格缩进
- 编码：文件统一使用 UTF-8
- 命名规范：遵循 `<prefix>_<name>_<unit>`（如 `clb_traffic_rx_bps`、`s3_latency_e2e_get_ms`）
- 描述规范：简洁一致的中文描述，包含关键语义与单位

## 变更说明

### 2025-12-17
- **变更原因**：修复腾讯云 CLB 指标获取问题，统一文件命名规范
- **具体变更**：
  - 修复映射注册机制：`RegisterNamespaceMetricAlias` 和 `RegisterNamespaceMetricScale` 改为合并模式，避免覆盖
  - 更新腾讯云 CLB 指标映射：使用实际采集到的指标名称（`VIntraffic`/`VOuttraffic`/`VInpkg`/`VOutpkg`/`VConnum`）
  - 修复维度配置：腾讯云 CLB 所有指标统一使用 `vip` 维度（原配置误用 `eip`）
  - 文件重命名：`internal/metrics/tencent/lb.go` → `clb.go`，统一使用云厂商产品名称
  - 缓存键统一：腾讯云 CLB 缓存键从 `"lb"` 改为 `"clb"`

### 2025-12-15
- **变更原因**：补齐 ALB 描述、对齐 S3 与 OSS/COS 映射、修正 CDN 映射一致性
- **具体变更**：
  - 为 `alb.metrics.yaml` 中所有缺失的 canonical 指标补充 `description`
  - S3 补充：授权错误（计数/占比）、客户端超时（计数/占比）、首字节延迟、Copy/Append 延迟与请求、腾讯云归档（容量/对象/碎片/取回/读写请求）、复制流量（Aliyun 计量 RX/TX，Tencent 总量）
  - 修正腾讯云 `CdnOriginTraffic` 的映射位置为“CDN 回源出流量”
  - 文档与 YAML 同步了单位与 `scale` 规则（如 MB → Bytes 使用 `scale: 1048576`）

## 参考实现

- 解析代码：`internal/config/mappings.go`
  - `MetricDef`/`CanonicalEntry`/`MetricMapping` 结构体
  - 别名与单位缩放注册：调用 `internal/metrics` 中的注册函数
