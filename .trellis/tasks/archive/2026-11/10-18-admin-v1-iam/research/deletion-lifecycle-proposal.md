# IAM 软删除、恢复与显式永久删除：设计依据

已确认需求以 [父 PRD](../../10-04-admin-v1/prd.md) 和 [设计第 4–6 节](../design.md) 为准：软删除无限保留、管理员显式 Expunge，purge_time 为空；删除／恢复仅传 name，不加 etag 或 allow_missing，重复删除返回 NOT_FOUND；失效 SCS 存储行按现有 TTL 清理。本期不设用户清理期限或扫描器，未来调度选型不纳入交付。尚未落实的领域事务和错误转换保留为实施设计材料。

## 1. AIP 依据与接口范围

- [AIP-164](https://google.aip.dev/164) 允许无限期保留，不要求自动清理。软删除的 Delete 应返回资源，Undelete 必须直接返回资源；Expunge 返回 google.protobuf.Empty 或 LRO，并要求独立于普通删除的永久删除权限。
- [AIP-135](https://google.aip.dev/135) 允许可选 etag 和 allow_missing；本期选择不提供，删除／恢复仅依据事务内当前记录执行，不校验页面版本。
- [AIP-148](https://google.aip.dev/148) 定义 delete_time/purge_time。未安排永久清理时间时，本期 purge_time 保持空，不返回虚构期限。
- 本期仅已软删除目标可 Expunge，未删除目标拒绝；用户自助入口不提供恢复或永久删除。
- UserService 保持服务身份保护的 gRPC 接入；普通管理使用 manage_users，Expunge 使用独立 expunge_users，均作用于 iam:global。模型及部署关系见 design.md 第 6 节。

资源式接口：

```proto
rpc DeleteUser(DeleteUserRequest) returns (User);
rpc UndeleteUser(UndeleteUserRequest) returns (User);
rpc ExpungeUser(ExpungeUserRequest) returns (google.protobuf.Empty);
```

三个管理请求均只含 `name`（users/{user}），不增加 etag、allow_missing、用户指定状态、期限或级联模式。源码参考：`app/iam/service/api/protos/iam/user/v1/user.proto:18-57,89-115`；Example 已有 Delete/Undelete 直接返回 User 的契约，见 `app/example/service/api/protos/example/service/v1/user.proto:43-53,194-214`。

## 2. 已确认的状态与查询语义

- 不增加额外的删除前状态字段或 DELETED 枚举；保留原 status，用 delete_time 是否存在表达删除。
- 软删除保留资料、稳定 ID、邮箱验证事实、密码哈希、认证器状态及首次改密要求；不借用 DisableUser/EnableUser 改写状态。
- 恢复清空删除标记，原 status 自然保留；旧会话和令牌不复活。不再检查恢复期限。
- 默认 List 排除已删除用户；show_deleted=true 包含正常与已删除用户。仅看已删除记录时按 delete_time 筛选，表达式须与既有过滤解析能力对齐。
- 有管理权限的 GetUser 可读软删除记录；认证、会话解析与普通修改操作继续拒绝已删除身份。

当前 `UserRepo.GetUser` 委托内部 `Get`（`data/user.go:167-180`），实现需区分管理读取与认证读取，不能全局开启 SkipSoftDelete。mixin 不过滤 Update，`updateProfileFields` 当前只按 id/etag 更新（`data/user.go:259-264`），需显式加入未删除条件。

show_deleted 须传至数据层并进入分页 scope fingerprint。可参考 Example 的 `entcrud.List(..., scope.Fingerprint(includeDeleted))`；Servora `ListInput` 本身没有 ShowDeleted 字段，定义见 `../servora/core/crud/list.go:375-383`。

## 3. 共用命令与事务

管理 UserUsecase 与本人 AccountUsecase 共用 biz 包内 deleteUser 和 UserRepo.SoftDelete；自助 proof 携带四元凭据、LoginID、UserEtag。具体 Go 签名及两个锁函数见 [生命周期调用设计](lifecycle-implementation-design.md) 第 3–5 节。沿用 UserUsecase → AccountUsecase 依赖，不反向注入。

- 软删除事务：锁定用户，复验未删除及自助入口凭据条件；写 delete_time、etag/update_time，purge_time 保持空；复用 revokeUserSessions/revokeOAuthSessions，确保旧一次性凭据不会在恢复后重新可用；提交后返回 User。不检查请求 etag。
- 恢复事务：显式包含删除记录并锁定用户，确认仍已删除；清除删除标记并更新 etag/update_time；不改变原 status、认证器状态或首次改密要求，不撤销旧会话的失效标记。
- Expunge 事务：在管理授权后锁定用户，确认仍已软删除，清理该用户关联数据并硬删除身份；整个事务提交后才释放邮箱。
- 恢复与 Expunge 使用同一目标行的串行化检查，防止同时成功；与设密及其他身份变更的并发也须纳入验证。
- 自助入口在 biz 校验密码，明文不入 Repo；如何在提交前复验已验证凭据，与管理设密的登录并发问题一起审查，不引入全局 token 版本系统。

既有 `inTx` 使用 ReadCommitted 并保护回滚（`data/transaction.go:13-33`），`lockUser` 使用 FOR UPDATE（`data/authn.go:37-40`）。共享 SoftDeleteMixin 提供字段、过滤和硬删除绕过（`infra/entgo/mixin/soft_delete.go:25-100`）；它不负责领域关联数据清理。

## 4. Expunge 的领域数据范围

IAM 当前以标量 ID 关联数据，没有可依赖的数据库级联删除。推荐单用户一个事务，先取得认证器、IAM 登录、OAuth token session 的关联 ID，再显式处理：

1. 用户 refresh/access token、授权码及已关联身份的 OIDC 请求。
2. OAuth token session 与 IAM 登录记录。
3. 邮箱验证、密码恢复令牌。
4. 密码凭据及认证器。
5. 登录标识与用户行；同一事务提交后邮箱才释放。

只处理 human 令牌及目标关联记录，不能仅按 subject 字符串相等删除独立 service token。共享 OAuth client、签名密钥和其他服务数据不受影响。

SCS sessions 只有 token/data/expiry，没有 user_id（`data/schema/http_session.go:15-25`）。已选择由现有 Store 按 TTL 清理失效容器；IAMLoginSession 撤销或删除后，旧 cookie 立即无法解析身份。PRD 已明确容器延后物理清理的例外；不扫描全表、不添加用户索引或替换 Store adapter。此决策不保留 Expunge 范围内的 IAM 身份和 OAuth 用户数据。

## 5. 错误契约与源码依据

| 情形 | 结果 |
| --- | --- |
| 删除不存在/已删除用户 | NOT_FOUND，不提供 allow_missing。 |
| 恢复不存在或已清理用户 | NOT_FOUND。 |
| 恢复未删除用户 | ALREADY_EXISTS。 |
| Expunge 不存在/已清理用户 | NOT_FOUND。 |
| Expunge 未软删除用户 | FAILED_PRECONDITION，本期不允许直接永久删除正常账号。 |
| 权限不足 | 先拒绝调用，不泄露资源存在性。 |

Kratos v3.0.0 的 `errors.Error.GRPCStatus` 通过 `transport/http/status.ToGRPCCode` 从 HTTP code 得到 gRPC code；HTTP 409 对应 Aborted。ALREADY_EXISTS 和 ETAG_MISMATCH 共用 HTTP code 时需在 IAM gRPC 出口按领域 reason 选择准确状态，保留 ErrorInfo.reason；客户端不得将原生状态先转回 HTTP code 再编码。

## 6. 未来自动清理选型，仅供参考

### K8s 耦合发生在哪里

[Kubernetes CronJob](https://kubernetes.io/docs/concepts/workloads/controllers/cron-jobs/) 让任务部署和自动触发依赖 Kubernetes，但它可以只运行一个普通程序或容器。若清理命令和领域事务不调用 Kubernetes API，离开 K8s 时替换调度配置即可，业务清理代码可以保留；若直接在业务代码中管理 CronJob 对象，耦合才会进入应用代码。

CronJob 官方说明可能重复或遗漏调度；concurrencyPolicy=Forbid 只约束同一 CronJob 的执行，不能替代领域事务检查。本期不引入此部署方式或独立运维调用者。

### Asynq 与这类任务的关系

[Asynq Periodic Tasks](https://github.com/hibiken/asynq/wiki/Periodic-Tasks) 明确提供 Scheduler：按 cron/间隔把任务放进 Redis，由 worker Server 执行。它既可以承担周期清理，也可以承担其他异步任务。它负责调度、排队及任务执行机制，不负责 IAM 的清理规则。

同一 schedule 应只有一个 Scheduler，否则可能重复入队；worker 可以分布式运行。动态变更周期任务可使用 [PeriodicTaskManager](https://github.com/hibiken/asynq/wiki/Dynamic-Periodic-Task)。即使项目已经使用 Redis，仍需维护 Scheduler、Worker 和任务执行语义；当前不为单一清理需求引入这些组件。

### 技术选项对比

| 方式 | 运行依赖与职责 | 适用条件 |
| --- | --- | --- |
| Go time.Ticker | 随应用运行，无新增库或服务；只提供时间触发。 | 少量简单周期工作，接受进程生命周期约束。 |
| [robfig/cron](https://github.com/robfig/cron) / [gocron v2](https://github.com/go-co-op/gocron/tree/v2) | Go 进程内调度；日历表达式、时区或更丰富的作业控制。 | 需要复杂时间表但不需要持久化任务队列。多副本协调须明确；gocron 的分布式 elector/locker 需配置相应实现。 |
| [systemd.timer](https://www.freedesktop.org/software/systemd/man/latest/systemd.timer.html) / cron | 操作系统触发一次性程序，不要求应用常驻内置循环。 | VM、裸机和已有主机管理环境；systemd.timer 同一目标单元尚活跃时不会再启动一个实例。 |
| Kubernetes CronJob | 控制面创建 Job/Pod，部署配置依赖 K8s。 | 已使用 Kubernetes，倾向独立任务资源与运维管理。 |
| [Asynq](https://github.com/hibiken/asynq) | Scheduler + Redis 队列 + Worker，提供异步与周期执行能力。 | 已有多类异步任务，需要统一任务队列、并发消费和任务管理。 |
| [pg_cron](https://github.com/citusdata/pg_cron) | PostgreSQL 扩展，周期执行 SQL 或存储过程。 | 数据库内部维护或纯 SQL 保留策略；IAM 使用它需将相应事务规则纳入数据库端管理，不能绕过领域规则直接删表。 |

推荐按需求选：简单 Go 内部周期工作优先标准库或轻量调度库；VM 独立任务用系统定时器；已有 K8s 用 CronJob；已有异步任务生态时考虑 Asynq。没有必要现在选定或实现一套全平台调度框架。

### 保留可复用的领域清理能力

共享 mixin、API 约定及小型持久化辅助；各服务拥有自己的 Expunge 事务。以后如增加自动保留期策略，应在锁定目标后复验到期条件，再复用事务内的数据清理核心。不能仅按旧列表结果调用无期限条件的 Expunge，否则恢复后再次删除的记录可能被旧任务提前清除。

上述栈只做文档与官方资料对比，未安装、运行或测评其性能与部署。未来采用自动清理必须另行确认产品策略和实施，不改变本期手动 Expunge 决定。
