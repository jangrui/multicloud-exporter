# LB 查询性能与测试报告

- 评估统一前缀 `lb_*` 指标在 Prometheus/Grafana 下的查询性能。

- 指标来源：`multicloud_namespace_metric` 统一命名空间输出，经别名规范化后暴露为 `lb_*`。

- 入带宽（bit/s）：
  - `lb_traffic_rx_bps{cloud_provider=~"$cloud_provider", account_id=~"$account_id", region=~"$region"}`

- 出带宽（bit/s）：
  - `lb_traffic_tx_bps{cloud_provider=~"$cloud_provider", account_id=~"$account_id", region=~"$region"}`

- 入/出包速率：
  - `lb_packet_rx{...}` / `lb_packet_tx{...}`

- 丢包速率：
  - `lb_drop_packet_rx{...}` / `lb_drop_packet_tx{...}`

- 阿里云七层：
  - `lb_qps{cloud_provider="aliyun", ...}` / `lb_rt{cloud_provider="aliyun", ...}`

- 标签过滤最小化：Grafana 模板变量使用 `label_values` 仅在一个代表性指标上取值（如 `lb_traffic_rx_bps`），避免全局扫描。
