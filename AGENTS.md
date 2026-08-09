# servora-platform

Servora 平台服务与主要参考应用；当前包含 Audit 服务和 Example CRUD 服务。

## 约定

- 根 `go.work` 连接生成模块与各微服务。
- Proto 统一由根 `just gen` 刷新 Go、TypeScript、OpenAPI、Wire 与 Ent。
- `api/gen/go/`、`api/gen/ts/`、服务 Web generated client 与 `wire_gen.go` 只由生成命令维护。
- gRPC 与 HTTP 使用同一个领域 Proto service。
- 业务分层：`service -> biz <- data`；service 只适配接口，biz 定义 Use Case/Repo，data 实现存储。
- 修改 OpenFGA model 后运行 `just openfga-model-apply`。

## 命令

```bash
just init
just gen
just wire
just lint
just api-ts-check
just openfga-model-validate
just openfga-model-test
just openfga-model-apply
```

`api/gen` 与 `app/*/web` 共用根 pnpm workspace 和 lockfile。新增平台服务参考 `app/example`。
