# 自动发现指标配置系统

> 更新记录：2025-12-08；修改者：@jangrui；内容：补充需求分析、技术调研与可行性评估；明确 Period 自动适配与来源优先级；同步代码位置引用。

## 概述

- 按 `accounts.yaml` 中列出的云产品自动扫描可用指标，生成 `products` 配置。
- 事件驱动：监听 `accounts.yaml` 的 `resources` 集合变化，有变化时触发刷新；不再使用周期刷新。
- 通知：通过 REST/SSE 暴露与推送。

## 需求分析

- 来源唯一性：运行时以自动发现产出的内存产品集为唯一来源（source of truth），手工 `products.yaml` 不参与加载；仅持久化快照用于排查对比。
- 配置一致性：代码默认值、配置文件与 Chart 默认保持一致；Period 不得硬编码，需自动适配云侧最小可用周期。
- 可观测性：发现刷新、API 统计、限流计数与采集耗时需统一暴露指标，以便容量与可靠性评估。

## 技术调研

- 文件监听方案：定期轮询 `ACCOUNTS_PATH` 的 `mtime`，变更触发刷新；已实现于 `internal/discovery/manager.go:131-156`。
- 账户/资源签名：解析 `accounts.yaml` 后生成资源集合签名，避免冗余刷新；对比逻辑优化为 `reflect.DeepEqual` 替代 `json.Marshal` 以提升性能；对比见 `internal/discovery/manager.go:139-150`。
- 周期策略：Aliyun/Tencent 元数据接口均可返回指标支持的 `Periods`/`Period`；最小值用作默认请求参数（Tencent：`DescribeBaseMetrics`；Aliyun：`DescribeMetricMetaList`）。

## 可行性评估

- 可靠性：监听文件变更足以覆盖静态配置更新；SSE 流与 REST 接口提供外部核对能力。
- 性能：发现与采集解耦，TTL 控制枚举频率；缓存有效降低 `List/Describe` 压力。
- 一致性：运行时产品集为唯一来源，不进行本地文件持久化。

## 行为与实现摘录

- 启动与一次性刷新：创建 `Manager` 并立即刷新；引用 `internal/discovery/manager.go:56-84`。
- 文件监听触发：`watchAccounts` 检查 `mtime`，签名变化时调用 `Refresh`；引用 `internal/discovery/manager.go:131-156`。
- TTL 与缓存：资源枚举结果按 `server.discovery_ttl` 控制；腾讯采集器缓存接口见 `internal/providers/tencent/tencent.go:223-240,250-253`。
- Period 自动适配（腾讯）：包级缓存最小周期选择，应用于 CLB/BWP；引用 `internal/providers/tencent/tencent.go:136-197`，CLB/BWP 调用点 `internal/providers/tencent/clb.go:79-83`、`internal/providers/tencent/bwp.go:75-79`。

## 配置来源

## 组成

- 模块：`internal/discovery`
  - `manager.go`：管理发现、刷新、存储与通知
  - `aliyun.go`：阿里云命名空间指标发现（`acs_bandwidth_package`、`acs_slb_dashboard`）
  - `tencent.go`：腾讯云命名空间指标发现（`QCE/BWP`、`QCE/LB`）

## 运行时行为

- 启动：创建并启动 `Manager`，立即执行一次刷新。
- 监听：定期检查 `ACCOUNTS_PATH` 文件修改时间；当解析后资源集合签名变化时触发刷新。
- TTL：资源发现缓存按 `server.discovery_ttl` 控制（默认 `1h`）。
- 认证：管理接口可选 BasicAuth；建议在生产环境下通过 TLS 暴露。

## 配置来源

- `accounts.yaml`：按 `provider` 与 `resources` 选择命名空间；产品标识仅使用 canonical 名称（`clb/bwp/s3`）。阿里云仅发现具备 `InstanceId` 维度的指标，剔除分组级指标。
- `credential`：优先使用全局凭证访问云产品接口，缺省时回退账号凭证。

### 来源优先级

- 运行时产品源：自动发现产出的内存集合。
- 不持久化快照：仅通过 REST/SSE 提供对比与排查能力。
- 手工目录：`config/products/*` 不参与加载。

## REST API

- `GET /api/discovery/config`
  - 返回示例：
  ```json
  {
    "version": 1,
    "updated_at": 1733395200,
    "products": {
      "aliyun": [ {"namespace": "acs_slb_dashboard", "auto_discover": true, "metric_info": [{"metric_list": ["ActiveConnection", "TrafficRX" ]}] } ],
      "tencent": [ {"namespace": "QCE/BWP", "auto_discover": true, "metric_info": [{"metric_list": ["InTraffic", "OutTraffic"]}]} ]
    }
  }
  ```
- `GET /api/discovery/stream`
  - `text/event-stream`，推送 `update` 事件：`{"version": <int>}`。
  - 初始化事件 `init` 携带当前版本：`{"version": <int>}`。
  - 基于 SSE，客户端需保持长连接并自行处理断线重连。

- `GET /api/discovery/status`
  - 返回发现状态（示例）：
  ```json
  {
    "version": 3,
    "updated_at": 1733395200,
    "accounts_path": "/app/configs/accounts.yaml",
    "accounts_signature": "aliyun#alb,clb|tencent#bwp,clb",
    "subscribers": 2,
    "providers": ["aliyun", "tencent"],
    "products_count": {"aliyun":2, "tencent":1},
    "api_stats": [
      {
        "provider": "tencent",
        "api": "GetMonitorData",
        "total": 1280,
        "status_count": {"success": 1270, "limit_error": 8, "auth_error": 2},
        "qps_1m": 5.4,
        "qps_5m": 3.2
      },
      {
        "provider": "aliyun",
        "api": "DescribeMetricLast",
        "total": 3420,
        "status_count": {"success": 3400, "auth_error": 10, "region_skip": 10},
        "qps_1m": 12.1,
        "qps_5m": 9.8
      }
    ]
  }
  ```

## 与采集器集成

- 采用自动发现结果填充并采集；不再支持手工 `products.yaml` 配置。

## 测试与性能

- 单元测试：`internal/discovery/manager_test.go` 覆盖刷新与通知。
- 基准测试：`BenchmarkManagerRefresh` 验证大规模指标列表下的性能。
- 监听测试：`internal/discovery/watch_test.go` 验证文件更改触发刷新与广播通知。

## 使用建议

- 建议在测试环境验证指标集合后再推广到生产。
- 首次启动可能需要较长时间以获取指标列表，后续刷新成本较低。

## 版本历史

- 2025-12-08：补充需求/调研/评估；新增 Period 自动适配说明与代码引用；修改者：@jangrui。
