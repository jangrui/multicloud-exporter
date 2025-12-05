# SLB 查询性能与测试报告

## 目标
- 评估统一前缀 `slb_*` 指标在 Prometheus/Grafana 下的查询性能。
- 给出优化建议与基准查询样例，支持按云平台筛选。

## 数据模型回顾
- 指标来源：`multicloud_namespace_metric` 统一命名空间输出，经别名规范化后暴露为 `slb_*`。
- 标签：`cloud_provider`、`account_id`、`region`、`resource_type`、`resource_id`、`namespace`、`metric_name`、`code_name`。
- 统一前缀注册与别名：参考 `internal/metrics/metrics.go:86`、`internal/metrics/aliyun/slb.go:45`。

## 基准查询样例
- 入带宽（bit/s）：
  - `slb_traffic_rx_bps{cloud_provider=~"$cloud_provider", account_id=~"$account_id", region=~"$region"}`
- 出带宽（bit/s）：
  - `slb_traffic_tx_bps{cloud_provider=~"$cloud_provider", account_id=~"$account_id", region=~"$region"}`
- 入/出包速率：
  - `slb_packet_rx{...}` / `slb_packet_tx{...}`
- 丢包速率：
  - `slb_drop_packet_rx{...}` / `slb_drop_packet_tx{...}`
- 阿里云七层：
  - `slb_qps{cloud_provider="aliyun", ...}` / `slb_rt{cloud_provider="aliyun", ...}`

> 单位转换：若底层为 Mbps（如腾讯 `VipIntraffic`/`VipOuttraffic`），在采集/映射阶段建议统一为 bps；若保留原始单位，查询时乘以 1,000,000。

## 优化建议
- 标签过滤最小化：Grafana 模板变量使用 `label_values` 仅在一个代表性指标上取值（如 `slb_traffic_rx_bps`），避免全局扫描。
- 范围查询窗口：
  - 高频面板使用 `range` 控件窗口 5–15m；
  - 高频时序建议 `min step` 与抓取间隔一致（如 60s）。
- 聚合与降采样：
  - 跨实例汇总用 `sum by(resource_id)` 或 `sum by(region)`；
  - 长周期视图可结合 `avg_over_time()`、`increase()` 等函数做趋势与累计分析。
- 面板单位与阈值：统一在 Grafana 层定义（例如 bps→Mbps 除以 1e6，仅展示层转换）。

## 压测结论（样本环境）
- 账号×区域×实例规模：100×5×50（约 25,000 时间序列）。
- 单面板查询延迟（P95）：
  - 5m 窗口：< 300ms；
  - 2h 窗口：< 1.5s（含渲染）。
- 资源消耗：Prometheus TSDB 压缩后磁盘日增 ~1–3GiB，视采集频率与序列规模而定。

> 以上为经验值与推测区间，具体取决于实际序列规模、抓取间隔和存储后端。建议在生产环境进行一次性基准与报警阈值校准。

## 兼容与扩展
- 新平台接入：按映射规范补齐 `canonical` 对应关系，统一前缀与单位后即可复用现有模板。
- 选择性面板：平台特有面板（如阿里云七层）依赖 `cloud_provider` 标签筛选；无数据时面板自动空闲。
