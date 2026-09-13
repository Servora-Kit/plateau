# Servora 规范实施基线

本笔记记录 `servora` 分组在本轮规范建设中的实现依据与静态检查。调查时间为 2026-09-13；`../servora` 只读，未修改其源码、文档、生成物或 Git 状态。

## 调查范围与事实

| 主题 | 当前实现依据 | 已确认事实 |
| --- | --- | --- |
| 框架边界 | [`../servora/core/AGENTS.md`](../../../../../../../servora/core/AGENTS.md)、[`transport/AGENTS.md`](../../../../../../../servora/transport/AGENTS.md)、[`obs/AGENTS.md`](../../../../../../../servora/obs/AGENTS.md)、[`contrib/AGENTS.md`](../../../../../../../servora/contrib/AGENTS.md) | `core` 只收横切、跨 capability、无业务语义的协议/平台能力；transport、obs、contrib 和 TLS 有各自职责边界。 |
| 启动 | [`core/bootstrap/bootstrap.go`](../../../../../../../servora/core/bootstrap/bootstrap.go)、[`scan.go`](../../../../../../../servora/core/bootstrap/scan.go)、[`provider.go`](../../../../../../../servora/core/bootstrap/provider.go) | 当前入口为 `NewRuntime`、`Scan`、`Runtime.Run`/`Close` 和 `ProviderSet`；cleanup 和可选 section 的行为有源码与同目录测试。 |
| CRUD | [`core/crud/plan.go`](../../../../../../../servora/core/crud/plan.go)、[`list.go`](../../../../../../../servora/core/crud/list.go)、[`response.go`](../../../../../../../servora/core/crud/response.go)、[`contrib/db/entgo/crud/AGENTS.md`](../../../../../../../servora/contrib/db/entgo/crud/AGENTS.md) | 框架维护 ResourcePlan、ListPreparer、资源名、读映射与 Ent List/Clear adapter；Ent adapter 不管理事务、授权或业务 scope。 |
| transport/TLS | [`transport/server/AGENTS.md`](../../../../../../../servora/transport/server/AGENTS.md)、[`transport/client/AGENTS.md`](../../../../../../../servora/transport/client/AGENTS.md)、[`security/tls/tls.go`](../../../../../../../servora/security/tls/tls.go) | client/server middleware 链、endpoint 索引和 TLS 复用已有明确契约；TLS 默认最低 TLS 1.2。 |
| 可观测性与审计 | [`obs/audit/audit_middleware.go`](../../../../../../../servora/obs/audit/audit_middleware.go)、[`obs/logger/logger.go`](../../../../../../../servora/obs/logger/logger.go)、[`obs/metrics/metrics.go`](../../../../../../../servora/obs/metrics/metrics.go) | 审计失败不阻断原 handler；logger/OTel cleanup 由调用方负责；metrics 使用私有 registry 与 OTel provider。 |
| Proto 与生成 | [`api/protos/AGENTS.md`](../../../../../../../servora/api/protos/AGENTS.md)、[`buf.go.gen.yaml`](../../../../../../../servora/buf.go.gen.yaml)、[`buf.typescript.gen.yaml`](../../../../../../../servora/buf.typescript.gen.yaml) | 框架公共 Proto 是唯一来源；Go 和 TypeScript 生成物分别写入 `api/gen/go`、`web/packages/proto-utils/src/gen`，均禁止手改。 |
| 命令与 web | [`cmd/AGENTS.md`](../../../../../../../servora/cmd/AGENTS.md)、[`cmd/protoc-gen-servora-crud/main.go`](../../../../../../../servora/cmd/protoc-gen-servora-crud/main.go)、[`web/AGENTS.md`](../../../../../../../servora/web/AGENTS.md)、[`web/packages/proto-utils/test/crud.test.mjs`](../../../../../../../servora/web/packages/proto-utils/test/crud.test.mjs) | `svr` 与各 plugin 具有不同输出边界；`proto-utils` 是 CRUD/Proto/error 合同，不承载业务 HTTP client 或 UI 状态。 |

## 本组规范边界

- 新建 `servora/framework`、`servora/proto`、`servora/cmd`、`servora/web` 四层及索引；主题来自设计文档指定清单。
- `servora/framework/crud.md` 只说明框架的内部契约。业务服务使用流程由 `service/backend/crud.md` 维护，本组以相对链接引用，避免把 Example 的组合写成框架实现要求。
- 本轮没有读取 `../openspec`；历史 Requirement/Scenario 的逐项核对留给主代理后续执行，不能把当前静态源码调查表述为历史覆盖结论或端到端验收。

## 实际与既有文档差异

原 `.trellis/spec/servora/backend/` 是初始化模板：正文标明 “To fill”，描述通用 database/error/logging/quality 项目，没有 Servora 目录、符号或验证证据，也要求英文正文。它不适合作为 Plateau 的现行框架规范，已按本任务设计移除并以四个实际主题层替换。

`../servora/AGENTS.md` 中明确 `api/gen/go` 和 `web/packages/proto-utils/src/gen` 只由生成命令维护；当前规范保留该约束。框架 README/AGENTS 中列出的生成、发布和测试命令是可用检查入口；本组没有运行生成、发布、网络或完整产品测试，因此没有把任何能力写为已运行验收。

## 历史 findings 的现行判别

- findings 1：当前源码支持 ResourcePlan descriptor 准入、field behavior/mask、CRUD reason、token context fingerprint、read-only mapper 与 Ent Clear 边界；已补入框架 CRUD 正文。业务资源的 schema、scope、最终排序和事务仍由消费方决定，不迁入框架规则。
- findings 3：当前 error sidecar 确实使用 source-relative `./index.js` type-only import，且 64 位 ProtoJSON 字段生成为 `string`；已补入插件和 web 正文。历史 NodeNext 不是当前配置：`proto-utils` 使用 `moduleResolution: bundler`，不能写成现行 NodeNext 合同。
- findings 8：已补 section/field 的 owner、nil receiver、普通嵌套 allocate-default 与 oneof default-if-set。`Scan` 拒绝 nil target，与生成方法对 nil receiver 的无操作语义不同，已交叉链接。
- findings 9：当前 Servora 没有 `pkg/` 目录，也未发现本仓 `github.com/Servora-Kit/servora/pkg/` import；保留 core/transport/obs/contrib 的现行边界。Actor/SystemActor 不在当前 Servora 源码范围，未据历史记录写入本组规范。
- findings 10：已补当前 HTTP `json`/`protojson` codec 注册与 64 位整数测试证据。没有发现 Servora 自定义 Accept/Content-Type 协商实现，历史协商细节不作为现行 transport 规则；中间件顺序以当前 chain 实现和测试为准。

## 已执行的静态检查

- 已读取任务 PRD、设计、实施清单、规划上下文和 spec-writing 指引。迁移前 `get_context.py --mode packages` 发现旧 `servora/backend`；移除其空目录后，最终输出只发现 `cmd`、`framework`、`proto`、`web` 四层。
- 已读取 `../servora/AGENTS.md` 与 core/bootstrap/crud、transport、obs/audit、contrib、TLS、api/protos、cmd、web 的下级 AGENTS，并对关键符号执行定向 `rg`。
- 已检查 19 个本组规范/研究 Markdown 文件：相对链接可解析、均有末尾换行且无尾随空白；`To be filled`、`TODO: fill`、`placeholder` 无匹配。`git diff --check` 无输出。未运行生成、发布、网络或完整产品测试；任务上下文的最终注入验收由主代理在汇总全部分组和 JSONL 后完成。
