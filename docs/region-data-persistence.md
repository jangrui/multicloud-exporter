# 区域数据持久化指南

本文档详细说明智能区域发现功能的数据持久化选项和配置方法。

## 概述

智能区域发现功能通过持久化区域状态（`/app/data/region_status.json`）来避免重复探测无资源的区域，从而显著提升采集性能。

**核心价值**：
- ✅ 减少云厂商 API 调用次数（降低 50%-90%）
- ✅ 缩短采集周期（从 60 秒降低到约 15 秒）
- ✅ 节省云配额成本
- ✅ 提升监控响应速度

## 持久化选项对比

| 特性 | emptyDir（默认） | PVC（可选） |
|------|------------------|-------------|
| **存储类型** | 临时存储（节点本地） | 持久化存储（网络存储） |
| **Pod 重启** | ✅ 状态保留 | ✅ 状态保留 |
| **Pod 删除** | ❌ 状态丢失 | ✅ 状态保留 |
| **配置复杂度** | 简单（无需配置） | 中等（需配置 PVC） |
| **适用场景** | 开发/测试环境 | 生产环境 |
| **存储成本** | 免费 | 依赖 StorageClass |

## 配置方式

### 1. emptyDir（推荐用于开发/测试）

**特点**：
- 无需额外配置，开箱即用
- 适合短期运行或频繁重建 Pod 的场景
- 数据存储在节点本地，性能好

**配置示例**：

```yaml
# values.yaml
server:
  regionDiscovery:
    enabled: true

regionData:
  persistence:
    enabled: false  # 默认值，使用 emptyDir
```

**安装命令**：

```bash
helm install multicloud-exporter ./chart
```

**验证**：

```bash
# 检查 Pod 内的数据目录
kubectl exec deployment/multicloud-exporter -- ls -la /app/data

# 查看区域状态文件
kubectl exec deployment/multicloud-exporter -- cat /app/data/region_status.json
```

---

### 2. PVC（推荐用于生产环境）

**特点**：
- 跨 Pod 生命周期保留数据
- 支持 Pod 重新调度到不同节点
- 可选使用已存在的 PVC

**配置示例**：

#### 2.1 使用动态 PVC（创建新的 PVC）

```yaml
# values.yaml
server:
  regionDiscovery:
    enabled: true

regionData:
  persistence:
    enabled: true
    storageClass: standard       # StorageClass 名称（留空使用集群默认）
    size: 1Gi                  # PVC 大小（默认 1Gi，区域状态文件很小）
    accessMode: ReadWriteOnce    # 访问模式（默认 ReadWriteOnce）
```

**安装命令**：

```bash
helm install multicloud-exporter ./chart \
  --set regionData.persistence.enabled=true \
  --set regionData.persistence.storageClass=standard \
  --set regionData.persistence.size=2Gi
```

#### 2.2 使用静态 PVC（使用已存在的 PVC）

**场景**：当已经有一个 PVC 时，可以直接挂载使用。

```yaml
# values.yaml
server:
  regionDiscovery:
    enabled: true

regionData:
  persistence:
    enabled: true
    existingClaim: my-region-data-pvc  # 已存在的 PVC 名称
```

**安装命令**：

```bash
helm install multicloud-exporter ./chart \
  --set regionData.persistence.enabled=true \
  --set regionData.persistence.existingClaim=my-region-data-pvc
```

---

## 详细配置说明

### regionData.persistence 参数

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `enabled` | bool | `false` | 是否启用 PVC 持久化。`false` 时使用 emptyDir |
| `storageClass` | string | `""` | StorageClass 名称。留空时使用集群默认 StorageClass |
| `size` | string | `1Gi` | PVC 请求的存储大小。区域状态文件通常很小（<100KB） |
| `accessMode` | string | `ReadWriteOnce` | PVC 访问模式。支持 `ReadWriteOnce`, `ReadWriteMany`, `ReadOnlyMany` |
| `existingClaim` | string | `""` | 已存在的 PVC 名称。设置时不会创建新 PVC |

### server.region_discovery 参数

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `enabled` | bool | `true` | 是否启用智能区域发现 |
| `discovery_interval` | string | `24h` | 重新发现周期。支持 `s`, `m`, `h`, `d` |
| `empty_threshold` | int | `3` | 连续为空次数阈值。超过此值后跳过该区域 |
| `data_dir` | string | `/app/data` | 数据目录路径 |
| `persist_file` | string | `region_status.json` | 持久化文件名（相对于 `data_dir`） |

---

## 部署示例

### 场景 1：开发环境（emptyDir）

```yaml
# values.yaml
replicaCount: 1

server:
  regionDiscovery:
    enabled: true
    discovery_interval: "1h"
    empty_threshold: 2

regionData:
  persistence:
    enabled: false  # 使用默认的 emptyDir
```

```bash
helm install multicloud-dev ./chart -f values.yaml
```

---

### 场景 2：生产环境（PVC + 高可用）

```yaml
# values.yaml
replicaCount: 3

server:
  regionDiscovery:
    enabled: true
    discovery_interval: "24h"
    empty_threshold: 3

regionData:
  persistence:
    enabled: true
    storageClass: fast-ssd
    size: 2Gi
    accessMode: ReadWriteOnce

# 高可用配置
affinity:
  podAntiAffinity:
    preferredDuringSchedulingIgnoredDuringExecution:
      - weight: 100
        podAffinityTerm:
          labelSelector:
            matchExpressions:
              - key: app.kubernetes.io/name
                operator: In
                values:
                  - multicloud-exporter
          topologyKey: kubernetes.io/hostname
```

```bash
helm install multicloud-prod ./chart -f values.yaml
```

---

### 场景 3：多租户环境（共享 PVC）

```yaml
# values.yaml
replicaCount: 1

server:
  regionDiscovery:
    enabled: true

regionData:
  persistence:
    enabled: true
    existingClaim: shared-region-data  # 多个 Exporter 共享同一个 PVC
```

```bash
# 安装第一个 Exporter
helm install multicloud-tenant1 ./chart -f values.yaml

# 安装第二个 Exporter（使用同一个 PVC）
helm install multicloud-tenant2 ./chart -f values.yaml
```

---

## 验证和故障排查

### 验证 PVC 创建

```bash
# 列出所有 PVC
kubectl get pvc -n monitoring | grep region-data

# 查看详细信息
kubectl describe pvc multicloud-exporter-region-data -n monitoring
```

### 验证数据持久化

```bash
# 1. 检查区域状态文件
kubectl exec deployment/multicloud-exporter -- cat /app/data/region_status.json

# 2. 验证文件格式（应该包含 region_map 和 updated_at）
kubectl exec deployment/multicloud-exporter -- \
  jq '.' /app/data/region_status.json

# 3. 删除 Pod，测试状态是否保留
kubectl delete pod -l app.kubernetes.io/name=multicloud-exporter

# 4. 等待新 Pod 启动后，检查状态文件
kubectl exec deployment/multicloud-exporter -- \
  cat /app/data/region_status.json
```

### 常见问题排查

#### 问题 1：PVC 处于 Pending 状态

**原因**：StorageClass 不存在或存储资源不足。

**解决**：
```bash
# 检查可用的 StorageClass
kubectl get storageclass

# 查看详细信息
kubectl describe pvc multicloud-exporter-region-data

# 修改 values.yaml，使用正确的 StorageClass
regionData:
  persistence:
    storageClass: fast-ssd  # 确保此 StorageClass 存在
```

#### 问题 2：权限错误

**原因**：Docker 镜像问题或权限配置错误。

**解决**：
```bash
# 检查日志
kubectl logs deployment/multicloud-exporter | grep -i permission

# 验证数据目录权限
kubectl exec deployment/multicloud-exporter -- ls -la /app/data

# 应该看到 65532 用户拥有该目录
# drwxr-xr-x 2 65532 65532 ...
```

#### 问题 3：状态文件损坏

**原因**：Pod 异常终止导致数据不一致。

**解决**：
```bash
# 删除状态文件，让 Exporter 重新创建
kubectl exec deployment/multicloud-exporter -- rm /app/data/region_status.json

# 重启 Pod
kubectl delete pod -l app.kubernetes.io/name=multicloud-exporter
```

#### 问题 4：区域状态不更新

**原因**：区域发现被禁用或调度器未启动。

**解决**：
```bash
# 检查日志中是否有调度器启动消息
kubectl logs deployment/multicloud-exporter | grep "启动区域重新发现调度器"

# 验证配置
kubectl exec deployment/multicloud-exporter -- \
  env | grep REGION_DISCOVERY_ENABLED
```

---

## 性能优化建议

### 调整重新发现周期

```yaml
server:
  regionDiscovery:
    discovery_interval: "12h"  # 资源变化频繁时，缩短周期
```

### 调整空区域阈值

```yaml
server:
  regionDiscovery:
    empty_threshold: 5  # 如果资源稀少，增加阈值以减少跳过风险
```

### 监控指标

区域发现功能提供以下指标：

```promql
# 智能区域选择统计
multicloud_region_discovery_stats{account_id="aliyun-prod"} > 0

# 区域状态分布
multicloud_region_status_count{account_id="aliyun-prod", status="active"}
```

---

## 安全最佳实践

### 1. 限制 PVC 访问权限

```yaml
regionData:
  persistence:
    accessMode: ReadWriteOnce  # 单 Pod 读写，更安全
```

### 2. 使用加密存储（如需要）

```yaml
# 创建带加密的 StorageClass
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: encrypted-standard
provisioner: kubernetes.io/aws-ebs
parameters:
  encrypted: "true"
  type: gp2
```

```yaml
# values.yaml
regionData:
  persistence:
    storageClass: encrypted-standard
```

### 3. 定期备份（可选）

```bash
# 定期备份区域状态文件
kubectl exec deployment/multicloud-exporter -- \
  cat /app/data/region_status.json > region_status_backup_$(date +%Y%m%d).json
```

---

## 升级和迁移

### 从 emptyDir 迁移到 PVC

```bash
# 1. 备份当前状态
kubectl exec deployment/multicloud-exporter -- \
  cat /app/data/region_status.json > region_status_backup.json

# 2. 升级到启用 PVC
helm upgrade multicloud-exporter ./chart \
  --set regionData.persistence.enabled=true \
  --set regionData.persistence.size=1Gi

# 3. 恢复状态（如果需要）
kubectl exec deployment/multicloud-exporter -- \
  cat > /app/data/region_status.json < region_status_backup.json
```

---

## 参考资料

- [Helm Chart 文档](../chart/README.md)
- [智能区域发现架构](../docs/architecture.md)
- [配置参考](../README.md#智能区域发现region-discovery)

