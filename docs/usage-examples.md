# 使用示例

> 版本：v1.0.0 | 创建日期：2026-01-22

## 基础部署

### 1. Docker 部署

```bash
# 拉取镜像
docker pull multicloud-exporter:latest

# 运行容器
docker run -d \
  --name multicloud-exporter \
  -p 9101:9101 \
  -v $(pwd)/configs:/app/configs \
  -v $(pwd)/.local/configs/accounts.local.yaml:/app/configs/accounts.local.yaml \
  -e LOG_LEVEL=info \
  multicloud-exporter:latest

# 查看指标
curl http://localhost:9101/metrics
```

### 2. Kubernetes 部署（Helm）

```bash
# 添加 Helm 仓库
helm repo add multicloud https://charts.jangrui.com/multicloud
helm repo update

# 安装
helm install multicloud-exporter multicloud/multicloud-exporter \
  --namespace monitoring \
  --create-namespace

# 查看状态
kubectl -n monitoring get pods -l app.kubernetes.io/name=multicloud-exporter

# 查看 Pod 日志
kubectl -n monitoring logs -f deployment/multicloud-exporter

# 查看 ServiceMonitor
kubectl -n monitoring get servicemonitor multicloud-exporter
```

### 3. Kubernetes ConfigMap 挂载

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: multicloud-exporter-config
  namespace: monitoring
data:
  server.yaml: |
    server:
      port: 9101
      scrape_interval: 60s
      discovery_ttl: 1h
      log_level: info
---
apiVersion: v1
kind: Secret
metadata:
  name: multicloud-exporter-accounts
  namespace: monitoring
type: Opaque
stringData:
  accounts.local.yaml: |
    accounts:
      aliyun:
        - account_id: "your-account-id"
          access_key_id: "your-access-key-id"
          access_key_secret: "your-access-key-secret"
          regions:
            - cn-hangzhou
            - cn-beijing
          resources:
            - slb
            - cbwp
            - oss
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: multicloud-exporter
  namespace: monitoring
spec:
  replicas: 1
  selector:
    matchLabels:
      app: multicloud-exporter
  template:
    metadata:
      labels:
        app: multicloud-exporter
    spec:
      containers:
      - name: exporter
        image: multicloud-exporter:latest
        ports:
        - containerPort: 9101
          name: http
        env:
        - name: LOG_LEVEL
          value: "info"
        volumeMounts:
        - name: config
          mountPath: /app/configs
        - name: accounts
          mountPath: /app/.local/configs/accounts.local.yaml
          subPath: accounts.local.yaml
        livenessProbe:
          httpGet:
            path: /healthz
            port: http
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /ready
            port: http
          initialDelaySeconds: 10
          periodSeconds: 5
      volumes:
      - name: config
        configMap:
          name: multicloud-exporter-config
      - name: accounts
        secret:
          secretName: multicloud-exporter-accounts
---
apiVersion: v1
kind: Service
metadata:
  name: multicloud-exporter
  namespace: monitoring
spec:
  selector:
    app: multicloud-exporter
  ports:
  - port: 9101
    targetPort: http
    name: http
---
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: multicloud-exporter
  namespace: monitoring
  labels:
    app: multicloud-exporter
spec:
  selector:
    matchLabels:
      app: multicloud-exporter
  endpoints:
  - port: http
    path: /metrics
    interval: 60s
```

---

## 高级配置

### 1. 集群模式部署（多副本）

```yaml
# values.yaml
replicaCount: 3

server:
  cluster_enabled: true
  first_run:
    strategy: auto
    max_delay: 180
```

```bash
# 部署
helm install multicloud-exporter ./chart \
  -f custom-values.yaml \
  --namespace monitoring
```

### 2. 自定义并发控制

```yaml
# server.yaml
server:
  region_concurrency: 4
  product_concurrency: 2
  metric_concurrency: 2

  region_discovery:
    enabled: true
    discovery_interval: 1h
    empty_threshold: 30
```

### 3. 启用华为云缓存优化

```yaml
# server.yaml
server:
  huawei_cache:
    enabled: true
    resource_ttl: 10m
    tag_ttl: 30m
```

### 4. 调整标签缓存 TTL

```yaml
# server.yaml
server:
  tag_cache_ttl: 30
```

---

## 监控集成

### 1. Prometheus 配置

```yaml
# prometheus.yml
scrape_configs:
  - job_name: 'multicloud-exporter'
    static_configs:
      - targets: ['multicloud-exporter:9101']
    scrape_interval: 60s
    scrape_timeout: 30s
```

### 2. Grafana Dashboard 导入

1. 从 `.local/grafana/dashboards/` 导入 JSON 文件
2. 或访问 Grafana Dashboard Marketplace:
   - 搜索 "Multicloud Exporter"
   - 导入 ID: 1xxxxx

### 3. 告警规则示例

```yaml
# alerts.yaml
groups:
  - name: multicloud_exporter
    rules:
      - alert: MulticloudExporterDown
        expr: up{job="multicloud-exporter"} == 0
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "Multicloud Exporter is down"
          description: "Exporter {{ $labels.instance }} has been down for more than 5 minutes."

      - alert: MulticloudHighRateLimit
        expr: rate(multicloud_rate_limit_total[5m]) > 10
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Multicloud Exporter rate limiting"
          description: "Exporter is experiencing high rate limiting ({{ $value }} req/s)."

      - alert: MulticloudScrapeFailed
        expr: rate(multicloud_scrape_errors_total[5m]) > 0.1
        for: 5m
        labels:
          severity: error
        annotations:
          summary: "Multicloud Exporter scrape errors"
          description: "Exporter scrape error rate is {{ $value }}."
```

---

## 故障排查

### 1. 检查 exporter 状态

```bash
# 健康检查
curl http://localhost:9101/healthz

# 就绪检查
curl http://localhost:9101/ready

# 查看指标
curl http://localhost:9101/metrics | grep multicloud_
```

### 2. 查看 Kubernetes Pod 日志

```bash
# 实时日志
kubectl -n monitoring logs -f deployment/multicloud-exporter

# 查看最后 100 行
kubectl -n monitoring logs deployment/multicloud-exporter --tail=100

# 查看特定标签的 Pod
kubectl -n monitoring logs -l app=multicloud-exporter
```

### 3. 调试模式

```bash
# 启用调试日志
export LOG_LEVEL=debug

# 或在配置文件中
server:
  log_level: debug
```

### 4. 内存泄漏诊断

```bash
# 获取堆内存快照
curl http://localhost:9101/debug/pprof/heap > heap.prof

# 使用 pprof 分析
go tool pprof heap.prof

# 或在浏览器中查看
go tool pprof -http=:8081 heap.prof
```

---

## 性能优化

### 1. 调整并发度

```yaml
server:
  region_concurrency: 8      # 区域级并发（1-8）
  product_concurrency: 2      # 产品级并发（1-2）
  metric_concurrency: 2       # 指标级并发（1-3）
```

### 2. 启用缓存

```yaml
server:
  discovery_ttl: 1h          # 资源发现缓存
  tag_cache_ttl: 30           # 标签缓存（分钟）
  huawei_cache:
    enabled: true
    resource_ttl: 10m         # 华为云资源缓存
    tag_ttl: 30m              # 华为云标签缓存
```

### 3. 智能区域发现

```yaml
server:
  region_discovery:
    enabled: true
    discovery_interval: 1h      # 重新发现周期
    empty_threshold: 30         # 空区域跳过阈值
```

---

## 多租户配置

### 1. 多账号配置

```yaml
# accounts.local.yaml
accounts:
  aliyun:
    - account_id: "account-1"
      access_key_id: "${ALIYUN_ACCOUNT1_ACCESS_KEY_ID}"
      access_key_secret: "${ALIYUN_ACCOUNT1_ACCESS_KEY_SECRET}"
      regions:
        - cn-hangzhou
        - cn-beijing
      resources:
        - slb
        - cbwp
    - account_id: "account-2"
      access_key_id: "${ALIYUN_ACCOUNT2_ACCESS_KEY_ID}"
      access_key_secret: "${ALIYUN_ACCOUNT2_ACCESS_KEY_SECRET}"
      regions:
        - cn-shanghai
        - cn-guangzhou
      resources:
        - oss
        - alb

  tencent:
    - account_id: "tencent-account-1"
      secret_id: "${TENCENT_ACCOUNT1_SECRET_ID}"
      secret_key: "${TENCENT_ACCOUNT1_SECRET_KEY}"
      regions:
        - ap-guangzhou
        - ap-beijing
      resources:
        - clb
        - nlb
        - bwp
```

### 2. 多云厂商配置

```yaml
# accounts.local.yaml
accounts:
  aliyun:
    - account_id: "aliyun-account"
      access_key_id: "${ALIYUN_ACCESS_KEY_ID}"
      access_key_secret: "${ALIYUN_ACCESS_KEY_SECRET}"
      regions:
        - cn-hangzhou
      resources:
        - slb

  tencent:
    - account_id: "tencent-account"
      secret_id: "${TENCENT_SECRET_ID}"
      secret_key: "${TENCENT_SECRET_KEY}"
      regions:
        - ap-guangzhou
      resources:
        - clb

  huawei:
    - account_id: "huawei-account"
      access_key: "${HUAWEI_ACCESS_KEY}"
      secret_key: "${HUAWEI_SECRET_KEY}"
      regions:
        - cn-north-4
      resources:
        - elb
        - obs

  aws:
    - account_id: "aws-account"
      access_key_id: "${AWS_ACCESS_KEY_ID}"
      secret_access_key: "${AWS_SECRET_ACCESS_KEY}"
      regions:
        - us-east-1
      resources:
        - elb
        - s3
```

---

## 安全配置

### 1. 使用 Secrets 管理凭证

```bash
# Kubernetes Secrets
kubectl create secret generic multicloud-accounts \
  --from-literal=aliyun_access_key_id="${ALIYUN_ACCESS_KEY_ID}" \
  --from-literal=aliyun_access_key_secret="${ALIYUN_ACCESS_KEY_SECRET}" \
  --namespace=monitoring

# 挂载到 Pod
env:
- name: ALIYUN_ACCESS_KEY_ID
  valueFrom:
    secretKeyRef:
      name: multicloud-accounts
      key: aliyun_access_key_id
```

### 2. 网络策略

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: multicloud-exporter-network-policy
  namespace: monitoring
spec:
  podSelector:
    matchLabels:
      app: multicloud-exporter
  policyTypes:
  - Ingress
  - Egress
  ingress:
  - from:
    - namespaceSelector:
        matchLabels:
          name: monitoring
    ports:
    - protocol: TCP
      port: 9101
  egress:
  - to:
    - namespaceSelector:
        matchLabels:
          name: kube-system
      ports:
      - protocol: TCP
        port: 53
  - to:
    - ipBlock:
        cidr: 100.64.0.0/10   # 阿里云公网
    ports:
    - protocol: TCP
        port: 443
  - to:
    - ipBlock:
        cidr: 169.254.0.0/16   # 腾讯云公网
    ports:
    - protocol: TCP
        port: 443
  - to:
    - ipBlock:
        cidr: 119.8.0.0/16     # 华为云公网
    ports:
    - protocol: TCP
        port: 443
  - to:
    - ipBlock:
        cidr: 54.240.0.0/14     # AWS 公网
    ports:
    - protocol: TCP
        port: 443
```

---

## 相关文档

- [README.md](../../README.md) - 项目概览
- [架构文档](../../docs/architecture.md)
- [指标映射规范](../../.opencode/rules/04-metrics.md)
- [部署规范](../../.opencode/rules/06-deployment.md)
- [质量保证](../../.opencode/rules/07-quality.md)
