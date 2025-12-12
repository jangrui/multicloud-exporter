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
  resource_id="bwp-xxxxxxxx",
  namespace="acs_bandwidth_package",
  metric_name="in_bandwidth_utilization",
  code_name=""
} 12.5
```

## 设计原则

- 指标可读：名称直观、语义明确，减少云厂商差异对使用方的影响。
- 单位统一：尽量将速率统一到 bit/s 与 pps；过渡期可保留云侧原单位并在文档标注。
- 渐进接入：先完成阿里云对齐，腾讯云按命名空间与指标文档逐步映射，保持相同行为与标签规范。

## 规划设计
- 统一前缀：所有共享带宽暴露指标统一前缀为 `bwp_*`，通过别名实现跨云一致。
- 映射驱动：使用 `configs/mappings/bwp.metrics.yaml:1-43` 维护 canonical→provider 的映射与单位/缩放建议：
  - Aliyun：`DownstreamBandwidth/UpstreamBandwidth`、`DownstreamPacket/UpstreamPacket`
  - Tencent：`InTraffic/OutTraffic`（单位 Mbps，统一乘以 `1000000`）；`InPkg/OutPkg`
- 标签设计：固定标签集合由指标注册统一维护（`cloud_provider, account_id, region, resource_type, resource_id, namespace, metric_name, code_name`），实现见 `internal/metrics/metrics.go:143-162`。
- 别名注册：
  - Aliyun：`internal/metrics/aliyun/cbwp.go:10-25`
  - Tencent：`internal/metrics/tencent/bwp.go:8-18`
- 周期策略：查询周期优先使用云侧最小支持周期；阿里云通过元数据最小周期 `DescribeMetricMetaList` 获取，逻辑在 `internal/providers/aliyun/aliyun.go:392-470`；腾讯云在 `internal/providers/tencent/tencent.go:136-197`。
- Grafana 约定：
  - 趋势图：`bwp_in_bps{...} / 1000000` 展示 Mbps（见 `grafana/dashboards/bwp.json:971`）
  - 模板变量：`cloud_provider` 基于 `label_values(bwp_in_bps,cloud_provider)`（见 `grafana/dashboards/bwp.json:1471`）

## 当前现状
- 采集实现：
  - Aliyun：批次拉取 `DescribeMetricLast` 并按维度键记录，核心在 `internal/providers/aliyun/aliyun.go:743-893`；维度键选择为 `BandwidthPackageId/bandwidthPackageId/sharebandwidthpackages`（见 `internal/providers/aliyun/aliyun.go:549-553`），默认维度映射更新在 `internal/config/config.go:64`。
  - Tencent：按指标分组调用 `GetMonitorData`，缩放逻辑在 `internal/providers/tencent/bwp.go:126-133`；记录指标通过 `metrics.NamespaceGauge`（见 `internal/providers/tencent/bwp.go:114-121`）。
- 别名与统一：
  - Aliyun：为新旧指标名都注册别名，确保暴露为 `bwp_in_bps/bwp_out_bps/bwp_in_pps/bwp_out_pps`（`internal/metrics/aliyun/cbwp.go:10-25`）。
  - Tencent：统一别名与缩放，保持与 Aliyun 一致的命名与单位（`internal/metrics/tencent/bwp.go:8-18`）。
- 大屏引用：
  - Timeseries 与变量均依赖历史名 `bwp_in_bps`（`grafana/dashboards/bwp.json:971, 1471`）。
- 历史问题复盘：
  - 问题：阿里云 BWP 统一映射后仅暴露了 `bwp_downstreambandwidth` 等新名，未注册 `in_bps/out_bps` 别名；同时维度键误用 `InstanceId`，导致采集跳过。
  - 修复：补充 Aliyun BWP 别名、纠正维度键选择；参考实现位置 `internal/metrics/aliyun/cbwp.go:10-25` 与 `internal/providers/aliyun/aliyun.go:549-553`、`internal/config/config.go:64`。
- 验证：
  - 本地：`curl -s localhost:9101/metrics | grep '^bwp_in_bps'`
  - 单测：缩放与注册已在腾讯云侧覆盖（`internal/providers/tencent/scale_test.go:11-15`、`internal/metrics/metrics_test.go:1-98`）。

## 预期收益
- 一致性提升：跨云相同语义指标以统一命名暴露，减少面向查询的适配成本。
- 运维友好：Grafana 模板变量与图表查询保持稳定，不受云侧指标名变更影响。
- 性能与正确性：周期自动适配避免不合法的 `Period`；维度键纠正确保不漏采。
- 迭代效率：映射集中在配置与别名注册，后续新增云厂商和指标的改动面更小。

## 改进建议
- canonical 完整覆盖：在 `configs/mappings/bwp.metrics.yaml:1-43` 中补充利用率与丢包指标的 canonical（例如 `utilization_pct/drop_pps`），并在 Aliyun/Tencent 注册对应别名与单位/缩放。
- 缩放统一：将 Aliyun 侧单位差异也纳入 `RegisterNamespaceMetricScale`，减少在 providers 中的特殊判断。
- 标签增强：为腾讯云 BWP 增加 `code_name` 注入能力（通过标签 API 或资源名称映射），与 Aliyun 对齐，便于业务筛选。
- 测试完善：补充 Aliyun BWP 的维度键选择与别名注册的单元测试，确保回归稳定。
- Grafana 变量容错：变量查询改为并集以提升鲁棒性：
  - `label_values({__name__=~"bwp_in_bps|bwp_traffic_rx_bps"},cloud_provider)`
- 文档持续同步：当映射或标签有变更时，遵循项目规则更新 `docs/bwp-metrics.md` 与 `chart/README.md`，保持“代码-配置-文档”一致。

## 参考实现位置
- 别名注册（Aliyun）：`internal/metrics/aliyun/cbwp.go:10-25`
- 别名注册（Tencent）：`internal/metrics/tencent/bwp.go:8-18`
- 维度键默认映射：`internal/config/config.go:64`
- 维度键选择（Aliyun）：`internal/providers/aliyun/aliyun.go:549-553`
- 指标记录统一接口：`internal/metrics/metrics.go:123-191`
- 缩放逻辑（Tencent）：`internal/providers/tencent/bwp.go:126-133`
- Grafana 查询引用：`grafana/dashboards/bwp.json:971, 1471`
