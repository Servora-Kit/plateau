# AGENTS.md - security/

<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-08-22 -->

## 目录定位

`security/` 承载平台共享安全生态：执行主体类型、AuthN/AuthZ 运行时与生成规则、OIDC 客户端协议、能力模型和凭据工具。常规服务直接装配 `security/authn/jwt` 与 `security/authn/session` 的开箱即用实现或泛型扩展；仅具备领域专属状态解析需求的服务（如 IAM 服务端可撤销登录会话查表）保留自有 `internal/authn`。

## 目录结构

- `actor.go`：共享执行主体 `Actor{Type, ID}`，类型为 `human` / `service` / `anonymous`；作为 AuthN 结果与 AuthZ 输入的通用载体，经 `WithActor` / `ActorFrom` 在进程内传递
- `authn/`：认证实现族与生成规则
  - `rules.go`：生成插件的 `AuthnRule` 聚合入口（`WithRulesFuncs` / `NewRules`）
  - `jwt/`：Bearer JWT 认证器、泛型指针约束 `ActorClaims[T]`（`AuthenticateFor` / `ServerFor`）及平台通用 `AccessClaims` / `ServiceClaims` / `HumanClaims`
  - `session/`：泛型 SCS 会话认证器（`New[T]` / `Server[T]`）以及标准 `*userpb.User` 浏览器会话接入（`SaveUser` / `NewUserAuthenticator` / `UserFrom`）
- `authz/`：授权实现族与生成规则
  - `rules.go`：生成插件的 `AuthzRule` 聚合入口（`WithRulesFuncs` / `NewRules`）
  - `openfga/`：OpenFGA 授权实现
- `oidc/`：纯 OIDC 客户端协议工具（`RelyingParty` 登录回调直接产出 `*userpb.User`、`NewServiceTokenMiddleware`），不反向依赖 `scs/v2` 或 `security/authn/*`
- `session/`：SCS 会话管理器、PostgreSQL Store 绑定与浏览器同源/JSON 过滤器
- `cap/`：能力模型与运行时（见 [cap/AGENTS.md](cap/AGENTS.md)）
- `password/`：密码哈希与校验
- `jwt/`：JWT 编解码与 JWKS 验签工具
- `errors/`：共享安全错误

## 约定

- `authn/` 与 `authz/` 下各实现以独立顶层 package 定义自己的 provider 合同（如 `session.Authenticator`、`openfga.Authorizer`），接入方按需选用实现。
- 安全 Proto 定义在 `api/protos/plateau/security/**`，生成代码位于 `api/gen/go/plateau/security/`。
- AuthN/AuthZ 代码生成插件源码在根 `cmd/protoc-gen-plateau-authn/` 与 `cmd/protoc-gen-plateau-authz/`，从当前 checkout 本地安装后由 `just gen` 驱动生成规则代码。
