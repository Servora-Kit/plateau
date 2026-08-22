# AGENTS.md - security/

<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-08-22 -->

## 目录定位

`security/` 承载平台共享安全生态：执行主体类型、AuthN/AuthZ 运行时与生成规则、能力模型和凭据工具。业务服务经自己的 `internal/authn` / `internal/authz` 适配层接入这里的实现。

## 目录结构

- `actor.go`：共享执行主体 `Actor{Type, ID}`，类型为 `human` / `service` / `anonymous`；作为 AuthN 结果与 AuthZ 输入的通用载体，经 `WithActor` / `ActorFrom` 在进程内传递
- `authn/`：认证实现族与生成规则
  - `rules.go`：生成插件的 `AuthnRule` 聚合入口（`WithRulesFuncs` / `NewRules`）
  - `jwt/`、`session/`：具体认证实现
- `authz/`：授权实现族与生成规则
  - `rules.go`：生成插件的 `AuthzRule` 聚合入口（`WithRulesFuncs` / `NewRules`）
  - `openfga/`：OpenFGA 授权实现
- `cap/`：能力模型与运行时（见 [cap/AGENTS.md](cap/AGENTS.md)）
- `password/`：密码哈希与校验
- `jwt/`：JWT 编解码工具
- `errors/`：共享安全错误

## 约定

- `authn/` 与 `authz/` 下各实现以独立顶层 package 定义自己的 provider 合同（如 `session.Authenticator`、`openfga.Authorizer`），接入方按需选用实现。
- 安全 Proto 定义在 `api/protos/plateau/security/**`，生成代码位于 `api/gen/go/plateau/security/`。
- AuthN/AuthZ 代码生成插件源码在根 `cmd/protoc-gen-plateau-authn/` 与 `cmd/protoc-gen-plateau-authz/`，从当前 checkout 本地安装后由 `just gen` 驱动生成规则代码。
