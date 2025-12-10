# 对象存储（S3/OSS/COS）指标规范与映射

> 更新记录：2025-12-09；修改者：@jangrui；内容：统一对象存储指标命名与映射。

## 统一命名

- 前缀：`s3_`
- 配置文件：`configs/mappings/s3.metrics.yaml`
- 核心指标集合：
  - `s3_bucket_size_bytes`：存储桶容量（Bytes）
  - `s3_internet_tx_bytes`：公网出流量（Bytes）
  - `s3_intranet_tx_bytes`：内网出流量（Bytes，部分云厂商支持）
  - `s3_requests_2xx`：2xx 请求数
  - `s3_requests_3xx`：3xx 请求数
  - `s3_requests_4xx`：4xx 请求数
  - `s3_requests_5xx`：5xx 请求数

标签规范：
- `cloud_provider`、`account_id`、`region`、`resource_type`（统一为 `oss` 或 `cos`，后续建议统一为 `s3`）、`resource_id`（Bucket名称）

## 阿里云映射 (OSS)

- 命名空间：`acs_oss_dashboard`
- 维度键：`BucketName`
- 指标映射：
  - `UserStorage` → `s3_bucket_size_bytes`
  - `InternetTraffic` → `s3_internet_tx_bytes`
  - `IntranetTraffic` → `s3_intranet_tx_bytes`
  - `CacheRecv` → `s3_cache_recv_bytes`
  - `CacheSend` → `s3_cache_send_bytes`
  - ...

## 腾讯云映射 (COS)

- 命名空间：`QCE/COS`
- 维度键：`bucket`
- 指标映射：
  - `StdStorage` → `s3_bucket_size_bytes`
  - `InternetTraffic` → `s3_internet_tx_bytes`
  - `2xxResponse` → `s3_requests_2xx`
  - `3xxResponse` → `s3_requests_3xx`
  - `4xxResponse` → `s3_requests_4xx`
  - `5xxResponse` → `s3_requests_5xx`
  - `CrossRegionReplicationTraffic` → `s3_replication_traffic_bytes`

## Prometheus 暴露示例

```
s3_bucket_size_bytes{
  cloud_provider="aliyun",
  account_id="aliyun-prod",
  region="cn-hangzhou",
  resource_type="oss",
  resource_id="my-bucket",
  namespace="acs_oss_dashboard",
  metric_name="UserStorage",
  code_name=""
} 104857600
```
