# 服务后端规范基线

本记录服务于 0 号任务的 `service`、`iam-service`、`example-service` 和 `audit-service` 规范建设。调查发生在规范实施阶段；只描述当前静态源码、测试和已确认约定，不把未运行的外部依赖写成已验收能力。

## 共同服务结构

- 确认约定：根 [AGENTS](../../../../AGENTS.md) 将 `app/` 定义为平台微服务目录；[app/AGENTS.md](../../../../app/AGENTS.md) 明确服务布局、`service -> biz <- data`、各层职责、服务 leaf 命令和 local/docker 配置边界。
- 当前实现：Example、IAM、Audit 均有 `cmd/server` 与 `internal/server`、`service`、`biz`、`data`。Example 的 [server.go](../../../../app/example/service/internal/server/server.go) 只含 ProviderSet，HTTP/gRPC 在对应文件；IAM 额外有 `authn`、`authz`、`oidc`、`mail`、`startup`，证明专有模块实际与四层并列。
- 确认约定与差异：biz 采用 `XxxRepo`/`XxxUsecase`，data 推荐私有 `xxxRepo`/`NewXxxRepo`。Example [userRepo](../../../../app/example/service/internal/data/user.go) 符合；IAM [data/user.go](../../../../app/iam/service/internal/data/user.go) 当前为 `userRepository`/`NewUserRepository`。本轮只记录，不改代码。

## Example CRUD

- 当前实现：`NewUserService` 在 [service/user.go](../../../../app/example/service/internal/service/user.go) 建立 `ResourcePlan`、`ListPreparer` 和 `ResourceNameMatcher`；服务方法解析资源名、准备 CRUD 输入并用 `plan.ToResponse`/`ToResponses` 输出。
- 当前实现：`NewUserRepo` 在 [data/user.go](../../../../app/example/service/internal/data/user.go) 用 `entcrud.NewListFields`、`Columns`、`Bind`、`ResourceMapper` 和 `ClearHelper` 建立 immutable 持久化合同；`ListUsers` 通过 `entcrud.List` 和 `mapper.ToDTOs` 返回 `corecrud.ListResult`。
- 当前实现：biz 的 [user.go](../../../../app/example/service/internal/biz/user.go) 负责 tenant scope fingerprint、allow-missing、etag、软删除/恢复和敏感临时密码哈希；数据层不推断这些业务语义。
- 当前实现：[User schema](../../../../app/example/service/internal/data/ent/schema/user.go) 的 canonical identity 唯一索引对活动与 tombstone 行均生效；`SoftDeleteMixin` 默认过滤 tombstone，Example 仅在包含已删除行时显式绕过。跨实体/并发不变量的事务责任以 IAM [inTx](../../../../app/iam/service/internal/data/transaction.go) 为现有例子，不从单实体 User Repo 推导为通用事务要求。
- 当前检查入口：`internal/biz/user_test.go`、`internal/service/user_integration_test.go`，以及独立 Servora 仓库的 `contrib/db/entgo/crud/*_test.go`。本轮未运行数据库或 transport 测试。

## IAM

- 当前实现：身份归一化与 UUIDv7 生成在 [biz/identity.go](../../../../app/iam/service/internal/biz/identity.go)；[biz/session.go](../../../../app/iam/service/internal/biz/session.go) 解析可撤销登录会话并检查 user 当前状态。
- 当前实现：`internal/authn` 将已装载 SCS session 转为共享 `security.Actor`，见 [authn/session.go](../../../../app/iam/service/internal/authn/session.go)；OIDC provider 的请求限制、discovery 与浏览器授权回调在 [oidc/provider.go](../../../../app/iam/service/internal/oidc/provider.go)。
- 当前实现：`internal/authz/openfga.go` 只将 authenticated Actor 映射到 OpenFGA subject；它不是 receiving service 的业务 PEP。服务 token 也只证明服务身份，见 [authn/jwt.go](../../../../app/iam/service/internal/authn/jwt.go)。
- 当前实现：[AccountUsecase](../../../../app/iam/service/internal/biz/account.go) 以 canonical email 查询、保留 display email，且在注册、重发验证和重置请求的账号查询/副作用前验证 CAP；重发/重置对 CAP 成功后的未知或不合资格账号返回空成功。密码重置在 [data/account.go](../../../../app/iam/service/internal/data/account.go) 的事务中消费令牌、更新密码并撤销全部用户 login/OAuth session。
- 当前实现：[OAuthRepository.Issue](../../../../app/iam/service/internal/data/oauth.go) 在事务中锁定用户，并将 code 消费、refresh 轮换和重放后的 OAuth session 撤销放在同一并发边界；服务 token 仅用于允许 client_credentials 的已注册 client/audience，见 [service_token.go](../../../../app/iam/service/internal/oidc/service_token.go)。
- 当前检查入口：`internal/authn/session_test.go`、`internal/authz/openfga_test.go`、`internal/oidc/provider_test.go` 和若干 server/data 集成测试。本轮未连接 PostgreSQL、OpenFGA 或公开 OIDC origin。

## Audit

- 确认状态：[README](../../../../README.md) 与 [app/AGENTS.md](../../../../app/AGENTS.md) 均将 Audit 标为停止维护、待重构，因此不作为新服务范例。
- 当前实现：[Consumer](../../../../app/audit/service/internal/data/consumer.go) 读取 Kafka CloudEvent，校验后交由 [BatchWriter](../../../../app/audit/service/internal/data/batch_writer.go) 批量写 ClickHouse；成功发送才提交有效记录，解码/校验失败记录会被警告并提交。
- 当前实现：消费支持 structured JSON 和 binary CloudEvents；writer 默认 100 条/一秒 flush，ClickHouse 成功发送后提交，写入失败不在该分支提交。DDL 采用日分区和 retention TTL，见 [clickhouse.go](../../../../app/audit/service/internal/data/clickhouse.go)。这些仅是停止维护服务的现状，不是新服务模板。
- 当前检查入口：`internal/data/consumer_test.go`、`internal/data/batch_writer_test.go`。本轮未运行 Kafka/ClickHouse 集成验证。

## 本轮范围

已据此建立服务和三个业务后端规范。没有修改业务源码、生成物、服务配置、Wire 生成物或任务状态；历史 OpenSpec 核对由主代理在本基线之后进行。
