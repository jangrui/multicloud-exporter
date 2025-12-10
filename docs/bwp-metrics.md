# 共享带宽（BWP）指标规范与映射

> 更新记录：2025-12-08；修改者：@jangrui；内容：确认腾讯云映射与 Period 自动适配；补充统一单位与实现位置引用。

## 统一命名

- 前缀：`bwp_`
- 配置文件：`configs/mappings/bwp.metrics.yaml`
- 指标集合（8项）：
  - `bwp_in_utilization_pct`：入方向带宽利用率（百分比）
  - `bwp_out_utilization_pct`：出方向带宽利用率（百分比）
  - `bwp_in_bps`：入方向带宽速率（单位以云侧口径为准，推荐统一为 bit/s）
  - `bwp_out_bps`：出方向带宽速率（同上）
  - `bwp_in_pps`：入方向包速率（包/秒）
  - `bwp_out_pps`：出方向包速率（包/秒）
  - `bwp_in_drop_pps`：入方向因限速丢弃的包速率（包/秒）
  - `bwp_out_drop_pps`：出方向因限速丢弃的包速率（包/秒）

标签规范：
- `cloud_provider`、`account_id`、`region`、`resource_type`（统一为 `bwp`）、`resource_id`

## 阿里云映射

- 命名空间：`acs_bandwidth_package`
- 维度键：`sharebandwidthpackages` 或 `bandwidthPackageId`
- 指标映射：
  - `in_bandwidth_utilization` → `bwp_in_utilization_pct`
  - `out_bandwidth_utilization` → `bwp_out_utilization_pct`
  - `net_rx.rate` → `bwp_in_bps`
  - `net_tx.rate` → `bwp_out_bps`
  - `net_rx.Pkgs` → `bwp_in_pps`
  - `net_tx.Pkgs` → `bwp_out_pps`
  - `in_ratelimit_drop_pps` → `bwp_in_drop_pps`
  - `out_ratelimit_drop_pps` → `bwp_out_drop_pps`

说明：
- 维度中的 `userId` 无需传入，由云监控自动注入。
- `Period` 建议从指标元数据的 `Min Periods` 读取；如未指定，使用最小可用周期。
- API 参考：云产品监控指标索引、服务接入点、`DescribeMetricLast`。

## 腾讯云映射

- 命名空间：`QCE/BWP`
- 指标别名注册：`internal/metrics/tencent/bwp.go:9-18`
- Period 自动适配：未显式配置时按云侧元数据选择指标最小可用周期；实现见 `internal/providers/tencent/tencent.go:136-197`，调用于 `internal/providers/tencent/bwp.go:75-79`
- 统一策略：
  - 若云侧提供利用率指标（如 `IntrafficBwpRatio/OuttrafficBwpRatio`），直接映射到 `bwp_*_utilization_pct`。
  - 速率单位统一为 bit/s 与 pps；缩放逻辑测试覆盖见 `internal/providers/tencent/scale_test.go:11-15`。
  - 丢弃包速率（如提供）遵循 `*_drop_pps` 命名；如语义差异，后续可通过标签区分（如 `drop_reason`）。
 - API 参考：`DescribeBaseMetrics`、`GetMonitorData`。

## Prometheus 暴露示例

```
bwp_in_utilization_pct{
  cloud_provider="aliyun",
  account_id="aliyun-test",
  region="cn-hangzhou",
  resource_type="bwp",
  resource_id="cbwp-xxxxxxxx",
  namespace="acs_bandwidth_package",
  metric_name="in_bandwidth_utilization",
  code_name=""
} 12.5
```

## 设计原则

- 指标可读：名称直观、语义明确，减少云厂商差异对使用方的影响。
- 单位统一：尽量将速率统一到 bit/s 与 pps；过渡期可保留云侧原单位并在文档标注。
- 渐进接入：先完成阿里云对齐，腾讯云按命名空间与指标文档逐步映射，保持相同行为与标签规范。
