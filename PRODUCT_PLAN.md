# 多云监控产品功能规划

## 基础框架

- [x] 多云平台支持
- [x] 按账号采集
- [x] Prometheus 指标注册与暴露 /metrics
- [x] API 请求计数与时延指标
- [x] 环境变量控制端口/采集间隔
- [x] Helm 基本模板
- [ ] 统一指标名称
- [ ] 多云平台自动发现
  - [x] bwp - 共享带宽
- [ ] 缓存刷新策略（DiscoveryRefresh）生效
- [ ] 统一限速与重试策略（跨云一致）
- [ ] 测试覆盖（单元/集成）
- [ ] 文档与示例完善（各云 products/accounts）

## 阿里云（Aliyun）

- [x] 阿里云区域自动发现
- [x] 资源ID缓存与 TTL
- [x] 命名空间指标动态注册与命名清洗
- [x] 共享带宽包命名空间指标采集（acs_bandwidth_package）
- [x] 共享带宽包资源枚举与分页
- [x] ECS 实例枚举（供命名空间维度使用）
- [ ] ECS 命名空间指标采集（acs_ecs_dashboard）默认启用
- [ ] RDS/Redis/SLB/EIP/NAT/OSS/CDN/VPC/DISK 指标采集
- [ ] 区域异常降级与默认区域策略完善（DEFAULT_REGIONS）

## 腾讯云（Tencent）

- [x] CVM 基础指标采集（CPU/内存）
- [ ] CLB 指标采集（连接数/带宽）
- [ ] CDB/Redis 基础指标采集
- [ ] EIP/NAT/VPC/COS/CDN/CBS 指标采集
- [ ] 区域自动发现与缓存
- [ ] 命名空间指标采集配置（qce_cvm/qce_clb）补齐 products.yaml

## 华为云（Huawei）

- [x] ECS 基础采集示例（状态）
- [ ] ELB 指标采集（QPS/带宽）
- [ ] RDS/Redis/EIP/NAT/OBS/CDN/VPC/EVS 指标采集
- [ ] 区域自动发现与缓存
- [ ] 命名空间指标采集配置（SYS.ECS/SYS.ELB）补齐 products.yaml

## 配置与部署

- [x] 环境变量覆盖配置路径（SERVER_PATH/PRODUCTS_PATH/ACCOUNTS_PATH）
- [ ] 完整 `products.yaml`（补齐 tencent/huawei）
- [ ] README 支持矩阵与使用示例更新
- [ ] 容器镜像与发布流程固化（版本节拍）

## 可观测性与安全

- [x] 采集阶段日志（账号/区域开始结束、枚举/拉取点数）
- [ ] 错误分类统一（鉴权/网络/参数），多云对齐
- [ ] 指标命名/标签一致性校验（跨云）
- [x] 环境占位符展开，避免明文凭证
- [ ] 最小权限账号与权限模板建议

## 里程碑

- P0：补齐产品配置驱动；腾讯 CVM/CLB、华为 ECS/ELB 基础采集；阿里云命名空间稳定化
- P1：区域自动发现与缓存统一；命名空间映射与限速/重试策略；API 统计统一
- P2：数据库/缓存/网络/存储覆盖；远端 Prom 推送与鉴权；部署与文档完善

最后更新日期：2025-12-03
