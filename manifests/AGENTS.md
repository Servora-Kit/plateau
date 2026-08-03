# AGENTS.md - manifests/

<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-03-15 | Updated: 2026-03-22 -->

## 目录定位

`manifests/` 保存仓库内可运行或可复用的部署资产，当前重点是 `k8s/` Kustomize 清单、`openfga/` 模型与 `scripts/` 自动化脚本。
这里描述的是 **当前仓库真实存在的目录和命令**，不要沿用过时目录名。

## 当前结构

```text
manifests/
├── k8s/
│   ├── base/                 # 命名空间、RBAC 与跨服务聚合入口
│   ├── iam/                  # IAM 服务 Kustomize 清单
│   └── sayhello/             # SayHello 服务 Kustomize 清单
├── openfga/
│   ├── model/                # OpenFGA model（如 servora.fga）
│   └── tests/                # OpenFGA model 测试（如存在）
├── scripts/
│   ├── k6/                   # 压测脚本
│   └── postgres-init/        # Compose 用 DB 初始化 SQL
└── ...
```

## WHERE TO LOOK

| 任务 | 位置 | 说明 |
|------|------|------|
| 本地基础设施 | `../docker-compose.yaml` | root `just compose-up` |
| 本地开发环境 | `../docker-compose.yaml` + 服务 leaf 本机 Air | root `just compose-up` 后执行 `just service::<name>::dev` |
| K8s 基础设施聚合 | `k8s/base/` | Namespace / RBAC / 跨目录资源聚合 |
| IAM 服务部署 | `k8s/iam/` | `deployment.yaml`、`service.yaml`、`configmap.yaml`、依赖资源 |
| SayHello 服务部署 | `k8s/sayhello/` | `deployment.yaml`、`service.yaml`、`configmap.yaml` |
| OpenFGA model | `openfga/model/` | 修改后需执行 `just openfga-model-apply` |
| OpenFGA model 测试 | `openfga/tests/` | 使用 `just openfga-model-test` |
| Compose 初始化脚本 | `scripts/postgres-init/` | 根 `docker-compose.yaml` 挂载 |
| 压测脚本 | `scripts/k6/` | k6 压测 |

## 约定

### 与 templates/ 的区别
- `templates/`：框架级示例模板，给使用框架的人参考
- `manifests/`：当前仓库实际使用/维护的部署资产
- 两者可以相互参考，但不要假设目录结构完全同步

### K8s 清单组织
- 统一使用 **Kustomize**，不是 Helm
- `k8s/base/` 负责命名空间、RBAC 与跨目录聚合；服务自身资源定义放在 `k8s/<service>/`
- 服务目录通常包含 `deployment.yaml`、`service.yaml`、`configmap.yaml`；如果有额外依赖（如 postgres、redis），也与服务清单放在同级目录
- 修改 `kustomization.yaml` 时先核对引用路径是否仍与当前仓库结构一致，尤其不要保留已经迁移/删除的旧路径

### 常用命令
```bash
just compose-up
just compose-stop
just compose-down
just compose-reset
just openfga-init
just openfga-model-validate
just openfga-model-test
just openfga-model-apply
```

## 维护提醒

- SQL 初始化脚本如果位于服务目录，应在对应服务目录内维护；`manifests/scripts/postgres-init/` 仅负责 Compose 场景
- 变更 OpenFGA model 后，除了提交文件本身，还要提醒执行 `just openfga-model-apply`
- 如果清单引用了 `app/`、`templates/` 或外部目录，更新前先验证这些路径今天是否真实存在，避免把历史遗留路径继续写入文档
