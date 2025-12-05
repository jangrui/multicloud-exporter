# SLB 指标统一命名规范（slb_*）

## 目标
- 为多云负载均衡（SLB/CLB）在 Prometheus/Grafana 中提供统一的指标命名与查询前缀 `slb_`。
- 覆盖跨云共有核心指标，并兼容平台特有指标，保证可扩展性。

## 命名原则
- 前缀：`slb_`。
- 风格：`snake_case`；单位或含义体现在名称中（如 `*_bps`、`*_pct`）。
- 共性指标：跨云语义一致，统一命名，如 `slb_traffic_rx_bps`、`slb_packet_tx`。
- 特有指标：保持语义清晰，统一到 `slb_<feature>`；若仅某云提供，使用标签 `cloud_provider` 进行筛选；必要时可增加 `slb_<provider>_<feature>` 扩展名（预留，不强制）。
- 维度标签：`cloud_provider`、`account_id`、`region`、`resource_type`、`resource_id`、`namespace`、`metric_name`、`code_name`。

## 共有核心指标（跨云）
- `slb_traffic_rx_bps`：入方向带宽（bit/s）。
- `slb_traffic_tx_bps`：出方向带宽（bit/s）。
- `slb_packet_rx`：入方向包速率（packets/s）。
- `slb_packet_tx`：出方向包速率（packets/s）。
- `slb_drop_packet_rx`：入方向丢包速率（packets/s）。
- `slb_drop_packet_tx`：出方向丢包速率（packets/s）。

> 单位统一：若原始单位为 Mbps，查询中统一转换为 bit/s（乘以 1,000,000）。

## 阿里云特有/拓展指标
- `slb_qps`：七层监听 QPS。
- `slb_rt`：七层监听 RT（ms）。
- `slb_status_code_2xx|3xx|4xx|5xx`：七层状态码计数（count/s）。
- `slb_unhealthy_server_count`：后端异常实例数（count）。
- `slb_healthy_server_count_with_rule`：七层规则后端健康数（count）。
- `slb_instance_qps|rt|packet_rx|packet_tx`：实例级七层指标。
- `slb_instance_traffic_rx_utilization_pct|slb_instance_traffic_tx_utilization_pct`：实例带宽使用率（ratio→展示为 %）。

参考：`internal/metrics/aliyun/slb.go:45` 的前缀注册与别名规范化。

## 腾讯云特有/拓展指标（样例）
- `slb_tencent_vconnum`：连接数（原始指标 `VConnum`，单位 count）。
- `slb_tencent_new_connection`：新建连接（原始指标 `VNewConn`，单位 count/s 或 count）。
- 其他维度型指标如 `Pvv*`、`Rv*`、`Rrv*` 根据需要映射到统一语义或以 `slb_tencent_*` 暴露。

> 统一映射建议：优先映射到共有核心指标，避免平台耦合；无法等价时以 `slb_tencent_*` 暴露并通过标签筛选。

## 兼容与扩展
- 新增云平台：为其命名空间注册统一前缀 `slb` 与别名规范化函数；将平台原生指标映射到上述统一集合。
- 单位与尺度：在查询层做轻量转换（如 Mbps→bps，ratio→%），避免在采集侧硬编码；采集端仅规范化名称与维度。

## 示例：Grafana 查询变量
- `cloud_provider`：`label_values(slb_traffic_rx_bps, cloud_provider)`。
- `account_id`：`label_values(slb_traffic_rx_bps, account_id)`。
- `region`：`label_values(slb_traffic_rx_bps, region)`。
- `resource_id`：`label_values(slb_traffic_rx_bps, resource_id)`。
- `code_name`：`label_values(slb_traffic_rx_bps, code_name)`。

## 参考位置
- 统一命名实现：`internal/metrics/metrics.go:86`、`internal/metrics/aliyun/slb.go:45`。
- 阿里云配置：`configs/products/aliyun/slb.yaml`。
- 腾讯云配置：`configs/products/tencent/slb.yaml`。
