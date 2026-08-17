# plateau

Servora 平台服务、主要参考应用与产品安全生态；当前包含安全 runtime/codegen、Audit 服务和 Example CRUD 服务。

## 约定

- 根 `go.work` 连接生成模块与各微服务。
- Proto 统一由根 `just gen` 刷新 Go、TypeScript、OpenAPI、Wire 与 Ent。
- `api/gen/go/`、`api/gen/ts/`、服务 Web generated client 与 `wire_gen.go` 只由生成命令维护。
- gRPC 与 HTTP 使用同一个领域 Proto service。
- 业务分层：`service -> biz <- data`；service 只适配接口，biz 定义 Use Case/Repo，data 实现存储。
- 修改 OpenFGA model 后运行 `just openfga-model-apply`。
- 公共包归属：`security` 共享 Actor、`security/authn/<implementation>`、`security/authz/<engine>`、`security/jwt`、`infra/openfga`；`security/authn` 与 `security/authz` 不定义公共父级 provider 合同。
- 安全 Proto 位于 `api/protos/plateau/**`；AuthN/AuthZ 插件从当前 checkout 的 `cmd/` 本地安装。
- Audit 服务只消费 generic CloudEvents，不拥有 AuthN/AuthZ typed payload、事件 namespace 或 emit 路径。
- OpenFGA 管理使用 `manifests/scripts/openfga.sh` / `openfga.ps1` 与 `fga` CLI，不依赖 `svr`。

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
