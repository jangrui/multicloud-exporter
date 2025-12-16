# 负载均衡（CLB）指标规范与映射

> 更新记录：2025-12-17；修改者：@jangrui；内容：修复腾讯云指标映射问题，统一使用实际采集到的指标名称（VIntraffic/VOuttraffic 等）。

## 统一命名

- 前缀：`clb_`
- 配置文件：`configs/mappings/clb.metrics.yaml`
- 核心指标集合：
  - `clb_traffic_rx_bps`：入方向流量（bit/s）
  - `clb_traffic_tx_bps`：出方向流量（bit/s）
  - `clb_packet_rx`：入方向包速率（pps）
  - `clb_packet_tx`：出方向包速率（pps）
  - `clb_drop_packet_rx`：入方向丢包速率（pps）
  - `clb_drop_packet_tx`：出方向丢包速率（pps）
  - `clb_drop_traffic_rx_bps`：入方向丢失流量（bit/s）
  - `clb_drop_traffic_tx_bps`：出方向丢失流量（bit/s）
  - `clb_traffic_rx_utilization_pct`：入方向带宽利用率（%）
  - `clb_traffic_tx_utilization_pct`：出方向带宽利用率（%）

标签规范：
- `cloud_provider`、`account_id`、`region`、`resource_type`（统一为 `clb`）、`resource_id`
- 阿里云特有：`code_name`（注入实例名称）、`port`（监听端口）、`protocol`（协议）
- 腾讯云：仅提供实例级维度，无监听端口维度。

## 阿里云映射 (SLB)

- 命名空间：`acs_slb_dashboard`
- 维度键：`instanceId`, `port`, `protocol`
- 指标映射：
  - `InstanceTrafficRX` → `clb_traffic_rx_bps` (实例级入流量)
  - `InstanceTrafficTX` → `clb_traffic_tx_bps` (实例级出流量)
  - `PacketRX` → `clb_packet_rx`
  - `PacketTX` → `clb_packet_tx`
  - `DropPacketRX` → `clb_drop_packet_rx`
  - `DropPacketTX` → `clb_drop_packet_tx`
  - `DropTrafficRX` → `clb_drop_traffic_rx_bps`
  - `DropTrafficTX` → `clb_drop_traffic_tx_bps`
  - `Qps` → `clb_qps` (Aliyun Only)
  - `Rt` → `clb_rt` (Aliyun Only)
  - `StatusCode2xx` → `clb_status_code_2xx` (Aliyun Only)
  - ...

## 腾讯云映射 (CLB)

- 命名空间：`QCE/LB`
- 维度键：`vip`
- 指标映射：
  - `VIntraffic` → `clb_traffic_rx_bps` (自动转换 Mbps -> bit/s，实际采集到的指标名)
  - `VOuttraffic` → `clb_traffic_tx_bps` (自动转换 Mbps -> bit/s，实际采集到的指标名)
  - `VInpkg` → `clb_packet_rx` (实际采集到的指标名)
  - `VOutpkg` → `clb_packet_tx` (实际采集到的指标名)
  - `Vipindroppkts` → `clb_drop_packet_rx`
  - `Vipoutdroppkts` → `clb_drop_packet_tx`
  - `InDropPkts` → `clb_drop_packet_rx`
  - `OutDropPkts` → `clb_drop_packet_tx`
  - `IntrafficVipRatio` → `clb_traffic_rx_utilization_pct`
  - `OuttrafficVipRatio` → `clb_traffic_tx_utilization_pct`
  - `VConnum` → `clb_active_connection` (实际采集到的指标名)
  - `VNewConn` → `clb_vip_new_connection` / `clb_new_connection` (实际采集到的指标名)
- **注意**：腾讯云 API 实际返回的指标名称可能因实例类型而异，代码中已兼容多种指标名称（`VipIntraffic`/`ClientIntraffic`/`VIntraffic` 等），确保都能正确映射到统一指标名。

## Prometheus 暴露示例

### 阿里云示例
```
clb_traffic_rx_bps{
  cloud_provider="aliyun",
  account_id="aliyun-prod",
  region="cn-hangzhou",
  resource_type="clb",
  resource_id="lb-bp1...",
  namespace="acs_slb_dashboard",
  metric_name="InstanceTrafficRX",
  code_name="my-clb",
  port="",
  protocol=""
} 102400
```

### 腾讯云示例
```
clb_traffic_rx_bps{
  cloud_provider="tencent",
  account_id="tencent-prod",
  region="ap-guangzhou",
  resource_type="clb",
  resource_id="43.153.253.128",
  namespace="QCE/LB",
  metric_name="VIntraffic",
  code_name="",
  port="",
  protocol=""
} 4998912.8
```
