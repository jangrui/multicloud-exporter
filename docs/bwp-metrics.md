# 共享带宽（BWP）指标规范与映射

## 统一命名

- 前缀：`bwp_`
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

## 腾讯云映射（接入计划）

- 命名空间：共享带宽包（BWP）命名空间以官方文档为准（如 `QCE/BWP`）。
- 指标与维度：通过 `DescribeBaseMetrics` 获取维度与周期，并映射到统一的 8 项指标名。
- 统一策略：
  - 若腾讯云提供利用率指标，直接映射到 `bwp_*_utilization_pct`；若仅提供速率与配额，将在采集侧计算利用率。
  - 速率单位对齐：如云侧单位不一致，优先转换为 bit/s 与 pps。
  - 丢弃包速率：若云侧有不同含义（限速/错误丢弃），增加标签 `drop_reason` 进行区分（未来可选）。
- API 参考：`DescribeProductList`、`DescribeBaseMetrics`、`DescribeStatisticData`、`GetMonitorData`。

## Prometheus 暴露示例

```
bwp_in_utilization_pct{
  cloud_provider="aliyun",
  account_id="aliyun-test",
  region="cn-hangzhou",
  resource_type="bwp",
  resource_id="cbwp-xxxxxxxx"
} 12.5
```

## 设计原则

- 指标可读：名称直观、语义明确，减少云厂商差异对使用方的影响。
- 单位统一：尽量将速率统一到 bit/s 与 pps；过渡期可保留云侧原单位并在文档标注。
- 渐进接入：先完成阿里云对齐，腾讯云按命名空间与指标文档逐步映射，保持相同行为与标签规范。

