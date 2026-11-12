# 管理服务调用与授权

分层遵循 [共享微服务规范](../service/backend/index.md)，领域归属见 [架构](architecture.md)。

- 通过被管理服务公开契约（如 IAM `iam.user.v1.UserService` gRPC）编排调用，不跨服务访问内部 usecase 或数据表；状态转换和身份生命周期留在领域所有者。
- 浏览器侧通过 `security/oidc.RelyingParty`（Authorization Code + PKCE S256 + `nonce` + HKDF 短期加密握手 Cookie）接入 IAM 登录，并在 Admin 自身 PostgreSQL SCS Store（嵌入 `infra/entgo/mixin.HTTPSessionMixin`、由 `security/session.NewPostgres` 构造）维护独立本地会话；退出 Admin 仅销毁本地会话，不联动注销上游 IAM 会话。
- 两层授权各自独立，不可互相替代：
  - 对人：Admin HTTP 入口通过 `security/authz/openfga` 在 `admin:global` 检查操作者的 `access`、`manage_users`、`expunge_users` 权限，并在禁用/删除/永久删除前执行防自操作（`SELF_ACTION_FORBIDDEN`）与同级管理员保护（检查目标用户的 `admin` 关系，命中返回 `TARGET_IS_ADMIN`）。
  - 对服务间调用：通过 `servora/transport/client/grpc.Dialer` 挂载 `security/oidc.NewServiceTokenMiddleware`（共享 `client_credentials` 令牌源且立即响应单次请求取消），由接收服务独立验证服务身份与操作权限（见 [授权](../iam/authorization.md) 与 [服务令牌](../iam/oidc.md)）。下游服务凭据、服务权限或网络故障在 Admin 对外统一转换为 `503 DOWNSTREAM_UNAVAILABLE`，绝不清除有效 Admin 会话或向浏览器误报 `401/403`。
- 冷启动管理员资格通过 `admin.yaml` 的 `bootstrap_admin_email` 以 Ensure-Seed（只增不删）方式幂等写入 `user:<id> admin admin:global`，启动期非阻塞尝试并在 OIDC 登录回调惰性重试。静态配置拥有的资源遵循所属服务的 [启动生命周期](../iam/startup.md)。
