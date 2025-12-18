# 暂存区：未统一映射的厂商原始指标（Unmapped Metrics）

本目录用于收纳“**暂时无法/不适合**纳入统一指标映射（`configs/mappings/*.metrics.yaml`）”的厂商原始指标清单。

## 目标（Option 1）

- `configs/mappings/*.metrics.yaml` **只包含跨云语义最稳、口径最容易对齐**的指标（canonical）。
- 其余原始指标进入暂存区，便于：
  - 评估是否应纳入 canonical
  - 记录口径差异、前置条件（例如 AWS S3 Request Metrics 的 `FilterId` 依赖）
  - 避免“为了覆盖更多指标而强行统一口径”导致的误导

## 数据来源

- 离线元数据快照：`local/configs/products/<provider>/<product>.yaml`
  - 注意：离线快照可能包含资源名称/实例ID等敏感信息；暂存区文件只记录**维度名称**，不记录具体维度值。

## 生成方式

使用工具 `cmd/mappings-unmapped` 从离线快照与 mapping 自动生成“未覆盖清单”：

```bash
go run ./cmd/mappings-unmapped \
  -provider aws \
  -prefix s3 \
  -products-root local/configs/products \
  -mapping configs/mappings/s3.metrics.yaml \
  -out configs/unmapped/s3.aws.yaml
```

## 分类规则（rules.yaml）

- 规则文件：`configs/unmapped/rules.yaml`
- 用途：为未映射指标自动填充 `stability` 与 `reason`
- 匹配按顺序命中第一条规则

## 文件命名规范

- `<prefix>.<provider>.yaml`
  - 例如：`s3.aws.yaml`、`clb.tencent.yaml`

## 字段说明（stability）

- `stability: stable`：无需额外前置条件，口径清晰，建议优先纳入统一映射
- `stability: conditional`：指标本身稳定，但依赖前置条件/默认策略（例如 AWS S3 的 `FilterId` 依赖 Request Metrics 开关），建议在明确策略后再纳入
- `stability: experimental`：口径/维度/单位差异较大或缺少等价项，建议保持暂存


