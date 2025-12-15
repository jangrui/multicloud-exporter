# 对象存储（S3/OSS/COS）指标规范与映射

> 更新记录：2025-12-15；修改者：@jangrui；内容：对齐 `configs/mappings/s3.metrics.yaml` 的统一命名与描述。

## 统一命名

- 前缀：`s3_`
- 配置文件：`configs/mappings/s3.metrics.yaml`
- 核心指标集合（与 YAML `canonical` 对齐）：
  - `s3_storage_usage_bytes`：存储空间使用量（Bytes）
  - `s3_traffic_internet_rx_bytes`：公网入流量（上传，Bytes）
  - `s3_traffic_internet_tx_bytes`：公网出流量（下载，Bytes）
  - `s3_traffic_intranet_rx_bytes`：内网入流量（上传，Bytes）
  - `s3_traffic_intranet_tx_bytes`：内网出流量（下载，Bytes）
  - `s3_traffic_cdn_rx_bytes`：CDN 回源入流量（Bytes）
  - `s3_traffic_cdn_tx_bytes`：CDN 回源出流量（Bytes）
  - `s3_requests_total`：总请求数（count）
  - `s3_availability_pct`：可用性/成功率（percent）
  - `s3_response_server_error_count`：服务端错误数（count）
  - `s3_latency_e2e_get_ms`：GET 端到端延迟（ms）
  - `s3_latency_server_get_ms`：GET 服务端延迟（ms）
  - `s3_latency_e2e_put_ms`：PUT 端到端延迟（ms）
  - `s3_latency_server_put_ms`：PUT 服务端延迟（ms）

标签规范：
- `cloud_provider`、`account_id`、`region`、`resource_type`（统一为 `s3`）、`resource_id`（Bucket名称）

## 阿里云映射 (OSS)

- 命名空间：`acs_oss_dashboard`
- 维度键：`BucketName`
- 指标映射：
  - `UserStorage` → `s3_storage_usage_bytes`
  - `InternetRecv` → `s3_traffic_internet_rx_bytes`
  - `InternetSend` → `s3_traffic_internet_tx_bytes`
  - `IntranetRecv` → `s3_traffic_intranet_rx_bytes`
  - `IntranetSend` → `s3_traffic_intranet_tx_bytes`
  - `UserCdnRecv` → `s3_traffic_cdn_rx_bytes`
  - `UserCdnSend` → `s3_traffic_cdn_tx_bytes`
  - `UserAvailability` → `s3_availability_pct`
  - `ServerErrorCount` → `s3_response_server_error_count`
  - `GetObjectE2eLatency` → `s3_latency_e2e_get_ms`
  - `GetObjectServerLatency` → `s3_latency_server_get_ms`
  - `PutObjectE2eLatency` → `s3_latency_e2e_put_ms`
  - `PutObjectServerLatency` → `s3_latency_server_put_ms`

## 腾讯云映射 (COS)

- 命名空间：`QCE/COS`
- 维度键：`bucket`
- 指标映射：
  - `StdStorage` → `s3_storage_usage_bytes`
  - `InternetTrafficUp` → `s3_traffic_internet_rx_bytes`
  - `InternetTrafficDown` → `s3_traffic_internet_tx_bytes`
  - `InternalTrafficUp` → `s3_traffic_intranet_rx_bytes`
  - `InternalTrafficDown` → `s3_traffic_intranet_tx_bytes`
  - `CdnOriginTraffic` → `s3_traffic_cdn_rx_bytes`
  - `CdnTrafficDown` → `s3_traffic_cdn_tx_bytes`
  - `RequestsSuccessRate` → `s3_availability_pct`
  - `5xxResponse` → `s3_response_server_error_count`
  - `CrossRegionReplicationTraffic` → `s3_traffic_replication_bytes`

## Prometheus 暴露示例

```
s3_storage_usage_bytes{
  cloud_provider="aliyun",
  account_id="aliyun-prod",
  region="cn-hangzhou",
  resource_type="s3",
  resource_id="my-bucket",
  namespace="acs_oss_dashboard",
  metric_name="UserStorage",
  code_name=""
} 104857600
```
