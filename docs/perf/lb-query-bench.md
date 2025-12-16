# CLB 查询性能与测试报告

- 评估统一前缀 `clb_*` 指标在 Prometheus/Grafana 下的查询性能。

- 指标来源：`multicloud_namespace_metric` 统一命名空间输出，经别名规范化后暴露为 `clb_*`。

- 入带宽（bit/s）：
  - `clb_traffic_rx_bps{cloud_provider=~"$cloud_provider", account_id=~"$account_id", region=~"$region"}`

- 出带宽（bit/s）：
  - `clb_traffic_tx_bps{cloud_provider=~"$cloud_provider", account_id=~"$account_id", region=~"$region"}`

- 入/出包速率：
  - `clb_packet_rx{...}` / `clb_packet_tx{...}`

- 丢包速率：
  - `clb_drop_packet_rx{...}` / `clb_drop_packet_tx{...}`

- 阿里云七层：
  - `clb_qps{cloud_provider="aliyun", ...}` / `clb_rt{cloud_provider="aliyun", ...}`

- 标签过滤最小化：Grafana 模板变量使用 `label_values` 仅在一个代表性指标上取值（如 `clb_traffic_rx_bps`），避免全局扫描。
