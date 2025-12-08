# 自动发现指标配置系统

## 概述

- 按 `accounts.yaml` 中列出的云产品自动扫描可用指标，生成 `products` 配置。
- 事件驱动：监听 `accounts.yaml` 的 `resources` 集合变化，有变化时触发刷新；不再使用周期刷新。
- 持久化与通知：写入本地文件并通过 REST/SSE 暴露与推送。

## 组成

- 模块：`internal/discovery`
  - `manager.go`：管理发现、刷新、存储与通知
  - `aliyun.go`：阿里云命名空间指标发现（`acs_ecs_dashboard`、`acs_bandwidth_package`、`acs_slb_dashboard`）
  - `tencent.go`：腾讯云命名空间指标发现（`QCE/BWP`、`QCE/CLB`）

## 运行时行为

- 启动：创建并启动 `Manager`，立即执行一次刷新。
- 监听：定期检查 `ACCOUNTS_PATH` 文件修改时间；当解析后资源集合签名变化时触发刷新。
- TTL：资源发现缓存按 `server.discovery_ttl` 控制（默认 `1h`）。
- 持久化：默认写入 `configs/products.auto.yaml`；可通过 `server.no_savepoint: true` 禁用。
- 认证：管理接口可选 BasicAuth；建议在生产环境下通过 TLS 暴露。

## 配置来源

- `accounts.yaml`：按 `provider` 与 `resources` 选择命名空间；负载均衡统一资源名为 `lb`，阿里云仅发现具备 `InstanceId` 维度的指标，剔除分组级指标。
- `credential`：优先使用全局凭证访问云产品接口，缺省时回退账号凭证。

## REST API

- `GET /api/discovery/config`
  - 返回示例：
  ```json
  {
    "version": 1,
    "updated_at": 1733395200,
    "products": {
      "aliyun": [ {"namespace": "acs_ecs_dashboard", "auto_discover": true, "metric_info": [{"metric_list": ["CPUUtilization", "DiskReadBPS" ]}] } ],
      "tencent": [ {"namespace": "QCE/BWP", "auto_discover": true, "metric_info": [{"metric_list": ["InTraffic", "OutTraffic"]}]} ]
    }
  }
  ```
- `GET /api/discovery/stream`
  - `text/event-stream`，推送 `update` 事件：`{"version": <int>}`。

## 与采集器集成

- 采用自动发现结果填充并采集；不再支持手工 `products.yaml` 配置。

## 测试与性能

- 单元测试：`internal/discovery/manager_test.go` 覆盖刷新与通知。
- 基准测试：`BenchmarkManagerRefresh` 验证大规模指标列表下的性能。
- 监听测试：`internal/discovery/watch_test.go` 验证文件更改触发刷新与广播通知。

## 使用建议

- 建议在测试环境验证指标集合后再推广到生产。
- 首次启动可能需要较长时间以获取指标列表，后续刷新成本较低。
