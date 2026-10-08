# ⚠️ 这个目录已废弃，请勿直接使用

这些裸 YAML 已经被 **Helm** 接管（release 名：`taskqueue`，namespace：`default`）。

## 现在的正确做法

```bash
# 部署（唯一入口）
helm upgrade taskqueue ../helm/taskqueue --set image.tag=<TAG> --wait

# 查看状态
helm status taskqueue
helm list
```

## 为什么不能用 kubectl apply

集群里的资源现在都带 Helm 注解（`meta.helm.sh/release-name: taskqueue`）。

如果你执行 `kubectl apply -f deploy/k8s/`，会直接改集群里的资源，而 **Helm 并不知道**。后果是：

1. 集群的实际状态和 `helm get values` 记录的不一致
2. 下一次 CI 跑 `helm upgrade` 时，**Helm 会把这些改动静默改回去**（你以为改了的东西凭空消失）
3. 更糟的是：这些文件里的 image 字段是旧 SHA，会把正在运行的镜像打回旧版

这正是引入 Helm 要消灭的问题 —— **配置漂移**。所以这些文件现在只作为「改造前的原始版本」留档参考。

## 迁移对照

| 原来 | 现在 |
|---|---|
| `deploy/k8s/*.yaml`（8 个文件，696 行）| `deploy/helm/taskqueue/`（1 份模板 + 1 份配置）|
| 改镜像要编辑文件 + commit | `helm upgrade --set image.tag=<SHA>` |
| 多环境要复制文件 | `--set` 覆盖，或 `-f values-prod.yaml` |
| 不能回滚 | `helm rollback taskqueue N` |
