# audit-v1：底座迁移与共享审计运行时

## 目标

完成 Servora 母框架旧审计与 Kafka 模块净身发版，并在 Plateau 落地无业务耦合的共享基础设施 `infra/kafka`、通用入站消费传输层 `transport/server/kafka`、规则生成器 `cmd/protoc-gen-plateau-audit` 与 CloudEvents 审计运行时 `security/audit`，为平台审计与后续服务接入提供统一支撑。

## 需求来源与范围

来源为 [audit-v1 主 PRD](../11-23-audit-v1/prd.md) §1、§2。涉及 `../servora` 仓库与本仓 `api/protos/plateau/{infra/kafka,security/audit}`、`cmd/protoc-gen-plateau-audit`、`infra/kafka`、`transport/server/kafka`、`security/audit`、`buf.go.gen.yaml`、`go.mod` 及 `.trellis/spec/` 对应规范。`app/audit/service` 业务重建、IAM/Admin 接入与前端页面由后续子任务承担。

| ID | 父需求 | 本任务交付 |
| --- | --- | --- |
| R1 | §1 | 从 `../servora` 移除 `audit/v1`、`obs/audit/v1`、`cloudevents/v1`、`contrib/kafka/v1` Proto、生成物、`protoc-gen-servora-audit`、`cmd/internal/{ruleplan,optionmerge}`、`obs/audit`、`contrib/kafka` 及无引用依赖（`cloudevents/sdk-go/v2`、`franz-go`），保留其余通用 `contrib` 模块并发布新版本。 |
| R2 | §2 | 新增 `api/protos/plateau/infra/kafka/v1/config.proto`、`infra/kafka`（可选连接、SASL、TLS、压缩与 `slog` 适配）及非泛型 `transport/server/kafka`（实现 Kratos `transport.Server`，承载多 Topic 注册、按批与定时触发、错误退避、累积位点安全提交与优雅停机 flush）。 |
| R3 | §2 | 新增 `api/protos/plateau/security/audit/v1/{annotations,config}.proto` 与 `cmd/protoc-gen-plateau-audit`，复用 `internal/codegen/ruleplan` 生成不可变规则表与克隆访问函数，替换 `buf.go.gen.yaml` 中的旧插件。 |
| R4 | §2 | 新增核心运行时 `security/audit`（`New` + `WithBackend`、内置 `Noop`/`NewStdout`/`NewLog`/`Multi`、`Server` 中间件、`NewEvent`）与独立外部后端包 `security/audit/kafka`（`Backend`/`New` 及 CloudEvents Kafka 编解码），打通 `security.ActorFrom(ctx)`、Kratos 错误 `reason`、OTel `trace_id`、客户端元数据及标准 AIP 资源名提取。 |
| R5 | §1 | 在 `GOWORK=off` 下完成 Servora 与 Plateau 两仓独立门禁校验（保留 `app/audit/service` 仅做最小编译过渡或留待子任务 2 重建前不阻塞根构建，具体衔接策略见 `design.md`）。 |

## 验收标准

- [x] AC-R1（R1）：`../servora` 在 `GOWORK=off` 下通过 `just gen-fresh`、`just gen-ts`、`just lint-proto`、`just test`、`just web-typecheck`、`just tidy`，`go.mod` 不再包含 `cloudevents/sdk-go/v2` 与 `twmb/franz-go`。
- [x] AC-R2（R2）：`infra/kafka` 明确区分未配置（返回 `nil, nil`）、配置或 Ping 失败（返回 `nil, err`）与成功连接；`transport/server/kafka` 单测覆盖非泛型多 Topic `BatchHandler` 调度、`batch_size` / `flush_interval` 触发、处理失败保留未提交位点、优雅 `Stop` 刷空尾批及禁用自动提交强制约束。
- [x] AC-R3（R3）：`cmd/protoc-gen-plateau-audit` 单测覆盖 `service_default` 继承、方法显式覆盖、确定性排序与返回 map 防御性克隆，`just gen-check` 无漂移。
- [x] AC-R4（R4）：`security/audit` 单测覆盖 `human` / `service` / `anonymous` 三类 `Actor` 注入、HTTP/gRPC 双协议上下文提取、Kratos 结构化错误 `reason` 提取、AIP 资源目标提取、敏感字段脱敏保护、发送失败不阻断业务返回，以及 Kafka JSON/Binary 编解码往返。
- [x] AC-R5（R5）：Plateau 升级新版本 Servora 后，在 `GOWORK=off` 下 `just gen-check`、`just test`、`just lint`、`just api-ts-check` 全部通过。

## 依赖与交付协作

- 本任务是其余三个子任务的前置依赖，优先独立实施、校验与归档。
- 接口与扩展属性契约见 [技术设计](design.md)；按整体规划顺序，待全部四个子任务的 `prd.md` 与 `design.md` 审阅定稿后，再生成本任务的 `implement.md` 并启动实施。
