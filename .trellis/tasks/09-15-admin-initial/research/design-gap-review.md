# Admin 实现与结构设计复核

日期：2026-09-16。范围为本任务 Admin/IAM/Servora 接入设计；仅静态源码审查，未运行产品构建或测试。区分已规划但未实施的能力与尚未闭合的合同；以下建议不代表产品行为已经获批，也不写入 spec。

## G1：管理创建用户的部分完成结果

- 当前事实：[AccountUsecase.CreatePendingUser](../../../../app/iam/service/internal/biz/account.go):112-130 先提交用户与凭据，再创建验证 token、同步发送验证邮件；后两步失败时用户已经存在。[UserService 的错误转换](../../../../app/iam/service/internal/service/user.go):177-190 将这类错误归为普通内部错误。
- 设计缺口：PRD R6/R16、AC5/AC15 要求准确结果和保留验证邮件流程，但未明确“账号已创建、验证邮件失败”的界面与返回合同；重试创建会变为邮箱已占用。不能把这类错误显示成账号未创建，也不能以新建另一个账号补偿。
- 处置：用户/凭据/登录标识/验证 token 在 IAM 内使用同一创建事务；发送邮件发生在提交后。发送失败保留 pending 用户，返回可识别的部分完成 reason 与已创建资源引用，Admin 刷新并明确提示；本人沿用 IAM 已有重发入口。不引入密码交付、邀请、额外通知或新邮件平台。
- 2026-09-16 用户已回复“按照你说的处理”，确认上述用户可见行为；已纳入 PRD R16/AC15、design 5.4 和 implement S1/S3/S5。该确认关闭产品问题，不表示已批准整套规划实施或功能已验收。

## G2：已删除用户查询与物理删除的隔离

- 当前事实：Servora [SkipSoftDelete](../../../../../servora/contrib/db/entgo/mixin/soft_delete.go):25-28 同时跳过默认查询过滤；同文件 72-78 表明它也绕过删除改写、允许执行原始删除。
- 设计缺口：[design](../design.md) 的 5.1 要求 Get/List 支持 `show_deleted`，5.2 的 GetBootstrapUser 允许返回 tombstone，但 6.1 又把 bypass 限定在恢复/purge 查询中，合同不一致。
- 建议：由 IAM data 层对授权管理查询局部创建 bypass context，限制在确切查询调用，不把它写回整个请求上下文；恢复的特定 tombstone 查询同样局部使用，物理删除仅由内部 purge 命令显式启用。公共 `show_deleted` 参数不能控制写入路径是否物理删除。此项属于技术设计澄清，无需新增用户功能。
- 验证：Get/List/GetBootstrapUser 的已删除视图正确；带查询可见性选项不能使 DeleteUser 物理删除；普通账号、登录与认证查询始终排除 tombstone。

## G3：服务令牌的活动状态不能写成现有校验

- 当前事实：[共享 JWT AuthN](../../../../security/authn/jwt/authn.go):91-124 验证签名、类型、issuer、audience 与时间；[IAM claims](../../../../app/iam/service/internal/authn/jwt.go):26-32 验证 service 身份字段。没有查询 OAuth token/client 的当前数据库活动状态。[现有测试](../../../../app/iam/service/internal/oidc/service_token_test.go):115-125 明确覆盖删除 client 后旧 token 仍能通过离线校验。
- 设计缺口：design 3.3 的“仍验证 active 状态”超出现有事实，且未说明要新增在线状态校验。它会让实施者误以为删除 client 等于立即吊销全部服务 JWT。
- 建议：明确保留当前签名/有效期校验与接收端独立 OpenFGA 授权；删除 client 阻止继续申请令牌，不宣称旧令牌立即失效。真正的即时服务 token 吊销需单独设计在线校验或撤销记录；不是往 Servora gRPC client 塞入 IAM 领域状态查询。

## G4：下游 RPC 错误归因与客户端生命周期

- 设计缺口：design 5.3 仅给出 HTTP 状态分类，尚未按失败来源区分浏览器认证、Admin 人类资格、IAM 服务凭据/权限，以及 IAM 领域拒绝。将 IAM `Unauthenticated` 原样转为浏览器 401，会错误清理仍有效的人的 Admin 会话；将服务权限失败转为人的 403 也会误导排障。
- 当前事实：Servora [Dialer](../../../../../servora/transport/client/grpc/dialer.go):86-94、128-134 每次 Dial 创建连接并返回建连错误；[连接测试](../../../../../servora/transport/client/grpc/conn_test.go):115-133 表明连接由调用方关闭。现有装配不替应用定义错误含义或连接所有权。
- 建议：Admin 的 IAM adapter 在 Wire provider 中创建应用生命周期的连接/生成客户端并返回 cleanup，避免逐请求 Dial。adapter 保留领域 reason，对下游服务认证/权限故障转换为可识别的依赖错误；只有 Admin 自身用户认证/资格失败返回相应 401/403。超时、取消、连接失败保留来源，不重放结果未知的变更。

## G5：Admin 仅作 IAM client（旧复制树历史快照）

- 删除前证据：Admin 副本曾含 `app/admin/service/internal/oidc/provider.go:34-81`、`internal/oidc/initializer.go:19-59` 和 `configs/local/oidc.yaml:1-20`。这些路径属于已删除目录，不再作为当前源码链接或实施输入。
- 当时设计缺口：工程布局没有完整区分通用启动骨架与 IAM 领域/发行方依赖，仅重命名 package 无法完成角色转换。
- 保留的设计结论：Admin 的 OIDC 模块只承担 RP/client；Admin public origin 与远端 IAM issuer、token endpoint、两个 OAuth client 和 IAM gRPC endpoint 分开配置，IAM adapter 与本地会话/bootstrap 存储分别实现 biz ports。Admin 不拥有 IAM 身份、凭据或 seed 创建流程。
- 删除前次序证据：`app/admin/service/api/protos/iam/account/v1/account.proto:9` 曾引用 `admin/user/v1/user.proto`，但复制树路径仍在 `iam/**`；`api/buf.openapi.gen.yaml:15-21` 也引用不存在的 `admin/**`。这两个文件已删除，原“先清理复制树再生成”的步骤已由新建工程取代。
- 2026-09-16 后续决定：用户已删除整个 Admin 后端，本轮核实目录不存在。实施按 IAM、Example 与 Trellis 规范重新建立 `app/admin/service`，S1 建立真实 Admin 源合同与生成入口，S4 完成接线；不得恢复这份过时复制树。

## 本轮处置

G2–G5 的技术澄清已写入 design 的工程布局、服务调用、HTTP 错误与软删除边界，并在 implement S1/S2/S4 增加对应前置步骤和检查。G1 的用户可见结果也已由用户确认并同步需求、设计和验收步骤，五项均已在规划中关闭。任务保持 planning，未修改 spec 或产品代码；未以静态审查代替功能验证。
