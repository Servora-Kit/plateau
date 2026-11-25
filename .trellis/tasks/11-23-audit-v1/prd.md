# audit-v1

## 目标

将审计注解、代码生成插件、CloudEvents 审计运行时及 Kafka 客户端封装从母框架 Servora 迁归 Plateau，按现行 IAM / Admin 工程标准彻底重建平台级审计服务 `app/audit/service`，打通平台控制面（IAM 与 Admin）审计事件生产、ClickHouse 持久化、服务间鉴权及 Admin Web 查询视图。

## 平台定位与职责划分

平台审计遵循与 `app/admin` 完全对称的平台定位（见 `.trellis/spec/admin/architecture.md`）：

1. **共享审计与流式传输工具包（`security/audit` + `cmd/protoc-gen-plateau-audit` + `infra/kafka` + `transport/server/kafka`）**：无特定业务倾向的规则生成、CloudEvents 事件封装、发送器与非泛型 Kafka 协议消费 Server 实现。平台服务和未来任意业务微服务均可复用该工具包；业务微服务若建设自有业务审计或消息消费，配置其独立的业务 Topic 或存储目标即可。
2. **平台控制面审计服务（`app/audit/service` + `app/admin` 审计入口）**：仅消费平台控制面 Topic（默认 `plateau.audit.events`），**只负责平台核心控制面（IAM 全局身份/认证/令牌生命周期、Admin 平台管理编排、核心服务间鉴权拒绝）的不可抵赖问责留痕**，面向平台管理员提供只读检索。
3. **多服务调用链路的记录原则（问责事实 vs 链路追踪）**：
   - 执行拓扑、内部读查询耗时、SQL 与常规缓存/鉴权放行过程由 OTel Trace（Jaeger）与日志（Loki）负责，`app/audit` 不重复建设调用树或访问流水。
   - 每次跨越受审计的服务信任面时，接收端按自身验证通过的真实 `security.Actor` 独立落一条扁平问责事实，不跨服务透传浏览器凭据或伪造人类操作者（与 `10-04-admin-v1/prd.md:102` 一致）：例如管理员经浏览器调用 Admin 再由 Admin 调 IAM 时，Admin 记录 `human:<admin_uid>`（含外部来源 IP/UA），IAM 记录 `service:<client_id>`（防范绕过 Admin 直连 IAM gRPC 无据可查），两跳通过共享 OTel `trace_id` 在查询时按需串联。
4. **单向依赖不变量**：`app/audit` 是纯事后留痕与只读分析终点——平台其他服务（IAM / Admin）不得反向查询 `app/audit` 作为实时风控或业务状态判据；`app/audit` 入库与查询时也不反向 RPC 调用 IAM 反查可变用户资料（避免循环依赖，且保证目标用户在 `ExpungeUser` 永久删除后历史审计记录依然独立完整）。

## 产品与架构约束

- **母框架净身与平台归位**：
  - 从 Servora 移除 `api/protos/servora/audit/v1`、`api/protos/servora/obs/audit/v1`、`api/protos/servora/cloudevents/v1`（两仓零运行时调用，见 `.trellis/tasks/archive/2026-09/00-bootstrap-guidelines/research/openspec-crosswalk.md:644-645`）、`api/protos/servora/contrib/kafka/v1`、`cmd/protoc-gen-servora-audit`、`cmd/internal/{ruleplan,optionmerge}`、`obs/audit`、`contrib/kafka` 及关联示例与 TS 生成物，同步清理 `servora/go.mod:14,39-40` 中的 `cloudevents/sdk-go/v2` 与 `franz-go` 依赖；其余通用 `servora/contrib` 模块（`db/entgo`、`db/gorm`、`db/redis`、`cache`、`k8s`）保留在母框架。
  - Plateau 对应承接为：`infra/kafka/` + `api/protos/plateau/infra/kafka/v1/config.proto`、`transport/server/kafka/`（与 `servora/transport/server/{http,grpc}` 同构的非泛型入站消费 Server）、`security/audit/` + `api/protos/plateau/security/audit/v1/{annotations,config}.proto`（package `plateau.security.audit.v1`，保留 extension `50100`/`50101`）、`cmd/protoc-gen-plateau-audit`（复用 `internal/codegen/{ruleplan,optionmerge,plugintest}`）。
- **`app/audit/service` 推倒重建**：
  - 彻底废弃旧实现中无鉴权裸露 Traefik 路由（`app/audit/service/internal/server/http.go:17-37`、`configs/local/bootstrap.yaml:39-43`）、旧 `audit.service.v1` 包结构、未绑定过滤条件的明文 base64 游标（`internal/data/audit.go:31-52`）及混放于 `internal/data` 的消费生命周期（`internal/data/consumer.go:34-86`）。
  - 登记 `10020–10029` 端口段（`10020` HTTP 仅用于内部运维/健康探针，关闭公开网关路由；`10021` gRPC 面向内部授权服务开放）。
  - ClickHouse 库名对齐 `docker-compose.yaml` 现行默认值 `audit`（表名 `events`）；单宽表存储平台审计事件，支持基于 `event_id` 的幂等去重（容忍 Kafka at-least-once 重试），默认 TTL 保留 **180 天**（支持配置覆盖与动态同步）。
- **投递不阻断主业务**：审计发送失败仅记录结构化错误日志与指标，不改写业务 RPC 返回结果；敏感字段（密码、令牌、密钥）受 `servora.redact.v3` 保护，绝不落入审计载荷。

## 任务组织

本任务作为父任务维护完整需求、定位约束、子任务映射及跨服务整体验收，不直接承担代码实现；各子任务独立维护自身 `prd.md`、`design.md`、`implement.md` 与上下文清单。

| 子任务 | 父需求归属 | 交付职责 | 前置顺序 |
| --- | --- | --- | --- |
| [底座迁移与共享审计运行时](../11-23-audit-v1-runtime/prd.md) | §1、§2；配合 §7 | Servora 旧审计/Kafka 净身与发版；Plateau `infra/kafka`、`transport/server/kafka`、`cmd/protoc-gen-plateau-audit`、`security/audit` 运行时与平台 Proto 契约 | 第 1 步（其余子任务前置） |
| [Audit 微服务与存储重建](../11-23-audit-v1-service/prd.md) | §3、§4；配合 §6、§7 | `app/audit/service` 推倒重建：`audit.event.v1` AIP 契约、ClickHouse `audit.events` 表与 180 天 TTL、Kafka 批量消费、Service JWT + OpenFGA `audit.fga` 鉴权 | 依赖 `audit-v1-runtime` |
| [IAM 与 Admin 审计接入及查询编排](../11-23-audit-v1-producers-bff/prd.md) | §5、§6；配合 §7 | IAM 与 Admin 平台控制面事件生产（规则自动审计 + 免鉴权认证与 403 拒绝显式审计）；Admin BFF 接入 `audit` gRPC 并向管理员暴露查询接口 | 依赖 `audit-v1-runtime` 与 `audit-v1-service` 契约 |
| [Admin Web 审计视图与联调](../11-23-audit-v1-web/prd.md) | §7、§8 | `app/admin/web` 新增「审计日志」一级菜单、分类快捷筛选、详情与同 `trace_id` 关联链路抽屉，完成全链路联调 | 依赖 `audit-v1-producers-bff` |

## 需求与验收

### 1. Servora 清理与跨仓契约独立门禁

- [x] Servora 已完整移除旧审计注解、审计配置、`cloudevents.proto`、`contrib/kafka`、`protoc-gen-servora-audit`、`cmd/internal/{ruleplan,optionmerge}`、`obs/audit` 及关联示例与 TS 生成物，`GOWORK=off` 下 Servora 全量构建、测试、Proto lint 与前端类型检查通过并完成版本更新。
- [x] Plateau 更新依赖至新版本 Servora，在不依赖本地 `go.work`（`GOWORK=off`）下通过 `just gen-check`、`just test`、`just lint` 与 `just api-ts-check`。

### 2. Plateau 共享流式传输与审计运行时（`infra/kafka` + `transport/server/kafka` + `security/audit`）

- [x] `infra/kafka` 基于 `plateau.infra.kafka.v1.Kafka` 提供区分“未配置 / 配置或连接失败 / 连接成功”的客户端构造能力，复用 Servora TLS 与 `slog` 适配；`transport/server/kafka` 提供非泛型 `*kafka.Server`（实现 Kratos `transport.Server`），统一托管拉包、按批/定时聚合、仅在批次处理成功时提交累积位点及停机刷盘生命周期。
- [x] `cmd/protoc-gen-plateau-audit` 基于 `plateau.security.audit.v1`（extension `50100`/`50101`）与 `internal/codegen/ruleplan` 生成不可变规则表与克隆访问函数，纳入根 `go.mod` `tool` 与 `buf.go.gen.yaml`。
- [x] `security/audit` 中间件与事件构造器自动提取 `security.ActorFrom(ctx)`（`human` / `service` / `anonymous`）、服务名、RPC `operation`、OTel `trace_id`、客户端 IP / User-Agent / `request_id`、Kratos 错误 `reason`（`error_code`），并支持从请求/响应的标准 AIP 资源注解提取目标资源名且遵守脱敏规则。

### 3. `app/audit/service` 消费与 ClickHouse 持久化

- [x] 兼容 CloudEvents structured JSON 与 Kafka binary 绑定，校验必填属性（`id`、`type`、`source`、`time`）；坏消息按明确策略记录诊断信息后推进位点，不死循环阻塞分区；仅在 ClickHouse 批次写入成功后提交 Kafka 位点，进程退出时使用独立超时 context 完成尾批 flush。
- [x] ClickHouse `audit.events` 表按事件发生时间 `occurred_at` 按月分区，默认保留期 180 天（可通过 `audit.conf.v1` 配置调整），对相同 `event_id` 重复投递具备去重保障；未知扩展事件保留原始 JSON / base64 对象信封于 `detail_json` 列。

### 4. `app/audit/service` 查询契约与服务间安全

- [x] `audit.event.v1.EventService` 仅提供只读 RPC（`GetEvent`、`ListEvents`，无创建/修改/删除接口），遵循 AIP 资源规范；`ListEvents` 支持时间区间、事件类别、生产服务、`actor_type`、`actor_id`、目标资源、`success`、`trace_id` 过滤，且不透明 `page_token` 绑定查询过滤指纹（篡改过滤条件时拒绝旧游标）。
- [x] `app/audit/service` gRPC 强制挂载 `jwt.ServerFor[jwt.ServiceClaims]` 与 `openfga.Server`（基于 `manifests/openfga/audit.fga` 校验服务读取关系），拒绝匿名、人类令牌、错误 audience 或未授权服务访问；移除公开 Traefik 路由暴露。

### 5. IAM 与 Admin 控制面审计生产

- [x] Admin 管理写操作（创建/更新/设密/撤销会话/封禁/解封/软删/恢复/永久删除用户）与 IAM 对应写操作（含用户管理、本人资料修改、已登录改密、本人自助删除、登出）开启规则表自动审计；跨服务调用各自记录真实 `Actor` 并共享同一 `trace_id`。
- [x] 免鉴权或认证前置阶段的关键安全动作（IAM 登录成败、首次改密完成/失败、邮箱验证、密码重置申请与确认、Admin OIDC 登录回调成败与退出、以及 403 鉴权拒绝）均发出携带目标标识（或可识别摘要）与结构化错误 `reason` 的审计事件，且绝不包含明文密码、令牌或重置凭证。

### 6. Admin 平台审计查询编排

- [x] Admin 后端使用自身 OAuth Service Token 调用 `audit.event.v1.EventService` gRPC，面向浏览器暴露受本地管理员会话与 OpenFGA 平台管理权限保护的 HTTP 查询接口；下游故障与人的登录态失效明确区分。

### 7. Admin Web 审计日志视图

- [ ] `app/admin/web` 在「总览 / 用户管理」同级新增单个一级菜单「审计日志」，不堆叠多余二级菜单。
- [ ] 支持按类别快捷视图（全部 / 认证与会话 / 资源变更 / 授权拒绝）、时间范围、`actor_type`（人类 / 服务 / 匿名）、服务、执行结果、`actor_id` / 目标资源 / `trace_id` 筛选分页浏览。
- [ ] 点击记录打开详情抽屉，展示完整标准维度、格式化 `detail` JSON，并支持按该记录的 `trace_id` 展示同一调用链上的上下游关联审计记录。

### 8. 全链路真实联调验收

- [ ] 在本地基础设施（PostgreSQL、Redis、OpenFGA、Kafka、ClickHouse）启动下，完成浏览器 → Admin Web → Admin Service → IAM Service → Kafka → Audit Service（ClickHouse）→ Admin Web 查询展示的端到端实测，覆盖人类直连认证成败、Admin 跨服务管理变更（同 `trace_id` 双记录关联）及无权限拒绝场景。

## 范围外事项

- **业务应用与租户级审计**：不接收、不存储、不展示未来业务子系统或租户工作台内部的领域业务操作日志，不引入 `tenant_id` 多租户字段。
- **高频运行观测流水**：不审计常规只读查询（`Get*` / `List*`）、健康检查、SQL 语句执行（不做 DB Driver 级全量拦截）及常规放行的 OpenFGA `Check` 判定。
- **反向业务耦合**：不将 `app/audit` 用作登录限流、账号锁定等实时业务风控状态源，不在 `app/audit` 内反向调用 IAM 拼装用户实时资料。
- **伪防篡改与本地归档脚本**：不引入无链式校验闭环的单行数字签名，不编写导出容器本地磁盘 JSONL 文件的定时删表任务。
