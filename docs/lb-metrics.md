# 负载均衡（LB）指标规范与映射

> 更新记录：2025-12-09；修改者：@jangrui；内容：统一 LB 指标命名与映射。

## 统一命名

- 前缀：`lb_`
- 配置文件：`configs/mappings/lb.metrics.yaml`
- 核心指标集合：
  - `lb_traffic_rx_bps`：入方向流量（bit/s）
  - `lb_traffic_tx_bps`：出方向流量（bit/s）
  - `lb_packet_rx`：入方向包速率（pps）
  - `lb_packet_tx`：出方向包速率（pps）
  - `lb_drop_packet_rx`：入方向丢包速率（pps）
  - `lb_drop_packet_tx`：出方向丢包速率（pps）
  - `lb_drop_traffic_rx_bps`：入方向丢失流量（bit/s）
  - `lb_drop_traffic_tx_bps`：出方向丢失流量（bit/s）
  - `lb_traffic_rx_utilization_pct`：入方向带宽利用率（%）
  - `lb_traffic_tx_utilization_pct`：出方向带宽利用率（%）

标签规范：
- `cloud_provider`、`account_id`、`region`、`resource_type`（统一为 `lb`）、`resource_id`
- 阿里云特有：`code_name`（注入实例名称）、`port`（监听端口）、`protocol`（协议）
- 腾讯云：仅提供实例级维度，无监听端口维度。

## 阿里云映射 (SLB)

- 命名空间：`acs_slb_dashboard`
- 维度键：`instanceId`, `port`, `protocol`
- 指标映射：
  - `TrafficRXNew` → `lb_traffic_rx_bps`
  - `TrafficTXNew` → `lb_traffic_tx_bps`
  - `PacketRX` → `lb_packet_rx`
  - `PacketTX` → `lb_packet_tx`
  - `DropPacketRX` → `lb_drop_packet_rx`
  - `DropPacketTX` → `lb_drop_packet_tx`
  - `DropTrafficRX` → `lb_drop_traffic_rx_bps`
  - `DropTrafficTX` → `lb_drop_traffic_tx_bps`
  - `Qps` → `lb_qps` (Aliyun Only)
  - `Rt` → `lb_rt` (Aliyun Only)
  - `StatusCode2xx` → `lb_status_code_2xx` (Aliyun Only)
  - ...

## 腾讯云映射 (CLB)

- 命名空间：`QCE/CLB`
- 维度键：`vip`
- 指标映射：
  - `VipIntraffic` → `lb_traffic_rx_bps` (自动转换 Mbps -> bit/s)
  - `VipOuttraffic` → `lb_traffic_tx_bps` (自动转换 Mbps -> bit/s)
  - `VipInpkg` → `lb_packet_rx`
  - `VipOutpkg` → `lb_packet_tx`
  - `Vipindroppkts` → `lb_drop_packet_rx`
  - `Vipoutdroppkts` → `lb_drop_packet_tx`
  - `IntrafficVipRatio` → `lb_traffic_rx_utilization_pct`
  - `OuttrafficVipRatio` → `lb_traffic_tx_utilization_pct`

## Prometheus 暴露示例

```
lb_traffic_rx_bps{
  cloud_provider="aliyun",
  account_id="aliyun-prod",
  region="cn-hangzhou",
  resource_type="lb",
  resource_id="lb-bp1...",
  namespace="acs_slb_dashboard",
  metric_name="TrafficRXNew",
  code_name="my-slb",
  port="80",
  protocol="http"
} 102400
```
