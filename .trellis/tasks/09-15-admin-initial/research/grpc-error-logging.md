# Kratos 错误传递与 Admin 日志接线

日期：2026-09-16。核对本机依赖的 Kratos v3.0.0 和当前 Servora/Plateau 源码；Context7 的官方示例用于交叉参考，精确版本行为以以下源码为准。本轮没有运行真实 Admin → IAM 调用或修改运行代码。

## 已有合同

- [Kratos errors](../../../../../../../../go/pkg/mod/github.com/go-kratos/kratos/v3@v3.0.0/errors/errors.go):22-33、57-65、126-151：本地 cause 通过 Unwrap 保留；gRPC 传输映射后的 status/message 与 ErrorInfo 的 reason/metadata。FromError 支持识别本地包装；远端恢复不会还原内部 cause，也不保证保留任意 gRPC details。不能把错误字符串当成公开合同。
- [IAM userError](../../../../app/iam/service/internal/service/user.go):177-191 已把内部错误变成稳定、安全的 UserError reason/message，并用 WithCause 保留本地诊断原因。Admin adapter 应复用这套结构化错误，不新造一套字符串协议。
- [CreateUser Redact](../../../../api/gen/go/iam/user/v1/user.pb.redact.go):61-77 和 [接收端测试](../../../../app/iam/service/internal/server/redact_test.go):18-45 已提供请求密码脱敏证据；不等于所有错误 cause 或自定义 metadata 自动安全。
- [Servora client chain](../../../../../servora/transport/client/middleware/chain.go):50-60 与 [server chain](../../../../../servora/transport/server/middleware/chain.go):101-114 各装配一次 logging。两端观测是有意义的，adapter/usecase 不应再无条件重复打印同一请求失败。
- [OTEL logger 测试](../../../../../servora/obs/logger/otel_test.go):30-64 体现启用对应 handler 时的关联能力；普通 stdout/file 输出不保证自动出现 trace_id/span_id，需按实际配置验收。Kratos logging 也会记录 error/stack，因此构造错误 cause 时不能拼接凭据。

## 本任务接线与验收

- 保留本地 `%w`/WithCause 链，通过 errors.FromError/code/reason 做来源分类；下游服务认证/权限问题不误判为人的 Admin 登录或资格错误。
- HTTP 只返回选定的 Admin/IAM 公开 reason、安全 message 和允许的 metadata，不无差别透传原始 Error/cause。
- 诊断日志包含调用方向、下游服务、RPC operation、code/reason、耗时与 trace/span 关联；新增设密、自助删除和首次改密请求必须纳入 Redact 验证。普通请求日志不输出 password/token/secret，不改变已确认的 seed 一次性受控交付日志。
- 用真实 gRPC 验证领域 reason、服务凭据拒绝、服务权限拒绝、超时与未知错误的 Admin 映射，并捕获实际日志验证字段、脱敏和重复记录边界。此处是待实施验收，不能用 Context7 示例或静态源码代替。

官方参考：[Kratos gRPC server/client 错误示例](https://github.com/go-kratos/kratos/blob/main/transport/grpc/server_test.go)、[日志过滤示例](https://github.com/go-kratos/kratos/blob/main/log/README.md)。示例来自 main，不将其作为 v3.0.0 全部字段行为的证据。
