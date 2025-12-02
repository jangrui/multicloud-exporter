# Multicloud Exporter Helm Chart

用于在 Kubernetes 中部署 `multicloud-exporter` 的 Helm Chart，暴露 Metrics 指标。

## 快速开始

```bash
kubectl -n monitoring create secret generic aliyun-accounts \
  --from-literal=account_id=xxx \
  --from-literal=access_key_id=xxx \
  --from-literal=access_key_secret=xxxx

helm repo add jangrui https://jangrui.com/chart --force-update

helm -n monitoring upgrade -i multicloud-exporter jangrui/multicloud-exporter --version v0.0.1

# 检查
kubectl -n monitoring get po,svc -l app.kubernetes.io/name=multicloud-exporter
```

默认监听 `9101` 端口并创建 `ClusterIP` Service，采集间隔为 `60` 秒。

## 参数

- 镜像
  - `image.registry`：镜像注册中心（默认 `ghcr.io/jangrui`）
  - `image.repository`：镜像仓库（默认 `multicloud-exporter`）
  - `image.tag`：镜像标签（默认 `Chart.AppVersion`）
  - `image.pullPolicy`：镜像拉取策略（默认 `IfNotPresent`）

- 服务
  - `service.port`：容器与服务端口（默认 `9101`）
  - `service.type`：Service 类型（默认 `ClusterIP`）

- 环境变量
  - `env.scrapeInterval`：采集间隔秒数（字符串，默认 `"60"`）

- 配置文件
  - `server.yaml`：由 `values.server` 渲染为 ConfigMap 并挂载到容器固定路径
  - `products.yaml`：由 Chart 内置为 ConfigMap 并挂载到容器固定路径
  - `accounts.yaml`：由用户预创建 Secret 提供并挂载到容器固定路径

-- 账号 Secret 引用
  - `accounts.secrets`：分散的每账号 Secret 列表，Chart 会生成 `accounts.yaml` 占位符并注入对应环境变量

- 调度与资源
  - `resources`：容器资源限制与请求
  - `nodeSelector`、`tolerations`、`affinity`：节点选择与亲和/容忍

## 升级与卸载

- 升级：
  ```bash
  helm -n monitoring upgrade multicloud-exporter jangrui/multicloud-exporter
  ```

- 卸载：
  ```bash
  helm -n monitoring uninstall multicloud-exporter
  ```

## 备注

- 建议将敏感配置通过 Secret 管理，避免直接提交到版本库。
