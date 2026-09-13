# Research: OpenSpec 核对 findings

- Query：复读当前 Trellis 正文、Servora/Plateau 源码与测试入口，逐项判断 69 个历史 spec 的有效规则、当前差异、正文缺口和验收边界。
- Scope：mixed（当前规范、源码/测试入口、历史 OpenSpec）；旧 OpenSpec 只作对照。
- Date：2026-09-13

## Findings

### 覆盖和状态

crosswalk 已逐行覆盖 69 个 spec、467 个 Requirement、974 个 Scenario；每个 Requirement 列出全部 Scenario，历史 Requirement/Scenario 引用共 1441 个。当前状态分布如下：

| 状态 | Requirement | Scenario | 解释 |
|---|---:|---:|---|
| 代码或配置存在但未验收 | 453 | 940 | 有具体源码、配置或测试入口，但本轮未执行该 Requirement 全部 Scenario；不由包级测试外推 |
| 待确认 | 0 | 0 | 原六项已由主代理核对 schema/源码归属并关闭，见下文 |
| 已过时 | 6 | 13 | Actor/SystemActor 与 ClickHouse 的历史 Servora owner/位置已迁入 Plateau；保留现行语义，旧路径过时 |
| 已过时（范围外） | 8 | 21 | integration/business-repo Makefile 历史；当前任务不启动迁移 |
| 已实现并验证 | 0 | 0 | 本轮没有把包级 `go test ./security/... ./infra/... ./cmd/...` 提升成全部 Scenario 验收 |
| 旧规范未实现 | 0 | 0 | 未找到足以把现行 Requirement 直接判成“旧规范未实现”的证据；未知保持“待确认” |

### 当前正文已经覆盖的规则，不再作为遗漏建议

- [servora/framework/crud.md](../../../../../spec/servora/framework/crud.md) 已覆盖 `ResourcePlan`、`ListPreparer`、`ResourceNameMatcher`、`ListQuery`、mask/lifecycle、mapper、Ent `ListFields`/`ClearHelper`、scope fingerprint 与框架错误；[service/backend/crud.md](../../../../../spec/service/backend/crud.md) 已覆盖消费方构造和 `ToResponse`、`Columns`、`Bind`、`ToDTO` 等检查清单。后续只补具体历史仍有效但正文没有的边界，不能再笼统建议“补 CRUD 基础合同”。
- [servora/proto/annotations.md](../../../../../spec/servora/proto/annotations.md) 已明确 `SectionKey`、`SectionOptional`、`ApplyDefaults`、`CheckRequired`、`ApplyConf` 及 default 子树的 allocate 规则；历史 `ValidateConf` 名称不应回写。`bootstrap.md` 已说明 scan 顺序、optional 缺失和错误停止。
- [servora/web/proto-utils.md](../../../../../spec/servora/web/proto-utils.md) 已明确当前 TypeScript `moduleResolution: bundler`、ESM `.js` type-only import、ProtoJSON 64 位整数和验证入口；历史 Node16/NodeNext 是兼容目标或旧约束，不能当作当前配置。
- [plateau/security/capabilities.md](../../../../../spec/plateau/security/capabilities.md) 已覆盖 v2 前缀、签名 challenge、Redis Lua/GETDEL 一次性消费、TTL、scope、未知字段拒绝、失败关闭和多实例边界；当前明确不读取旧有状态 token，旧 token 兼容 Scenario 不应作为现行规则补写。
- [iam-service/identity.md](../../../../../spec/iam-service/identity.md)、[authorization.md](../../../../../spec/iam-service/authorization.md)、[oidc.md](../../../../../spec/iam-service/oidc.md)、[sessions.md](../../../../../spec/iam-service/sessions.md) 已覆盖账号归一化、CAP 先于副作用、状态门槛、AuthN/AuthZ 分界、OIDC code/PKCE、OAuth 事务与服务令牌边界；缺口应针对未写出的具体业务不变量提出。
- [audit-service/ingestion.md](../../../../../spec/audit-service/ingestion.md) 已写明 Consumer→DecodeRecord→校验→BatchWriter→ClickHouse→提交链、structured/binary、未知 type、坏记录、flush/commit 失败和 DDL 维护事实；[status.md](../../../../../spec/audit-service/status.md) 已写明停止维护、不作为新服务模板和 Wire/依赖限制。Audit scaffold 不能因停止维护而整体判过时。
- [service/backend/data.md](../../../../../spec/service/backend/data.md) 已覆盖 schema owner、软删除、scope fingerprint、事务责任、错误保留和真实数据库验证边界；[api/proto/generation.md](../../../../../spec/api/proto/generation.md) 已覆盖根/leaf 生成顺序、输出 owner、TS sidecar、插件来源和验证命令；[web/client/http.md](../../../../../spec/web/client/http.md) 已覆盖同源、JSON/ProtoJSON 分流、取消/超时/GET 重试和写请求不重试。

### 最终集成与处置

以下发现已由主代理结合当前源码处理，未留下要求本任务额外实施业务功能的事项：

1. Example 的 `user-crud.md` 补充 biz 单元测试与 `TestUserReferenceIntegration` 的职责：后者实际装配 service/biz/data/Ent/SQLite，需要 integration tag 和显式 DSN，未配置即失败。它不等同 HTTP/gRPC 或浏览器验收。
2. Servora framework/crud.md 补入共享 `string_matches.json`、`resource_names.json` 及 Go core、Go plugin、TS helper 的实际读取者；向量或语义变更必须同时验证相应消费者。
3. Audit ingestion.md 增加查询责任、过滤、排序、分页及错误/空依赖维护事实。现有 cursor token 没有通用 CRUD 的 query fingerprint，不能将其写成新服务推荐。
4. IAM authorization/oidc 建立双向引用，模型与 tuple owner 的清单引用 Plateau infra/openfga 的权威正文，避免两处重复维护。
5. Mail 原三项“待确认”属于误判：原 Requirement 明确只要求共享配置，不要求平台发送器。当前 Mail/SMTP/MailFrom 字段、optional mail section 均存在；具体 Mailer/Sender/SMTP 实现位于 IAM internal/mail。已纳入 API annotations 与 infra 索引，归为代码或配置存在但未验收。
6. ClickHouse 原三项“待确认”已定位到 Plateau infra/clickhouse 与对应 Proto/测试；旧 Servora 路径过时。现行行为保留在 Plateau 主题，Servora providers 增加归属引用，不恢复旧 adapter。

初稿中按整组 Go 包测试推断历史 Scenario 全通过的口径已撤销；最终表没有把源码入口和未运行场景写成已验收。初稿的 fallback 目标、空证据和表格排版已修正，全部历史文件/行号引用在集成后重新核对。

### 过时、保留与已确认差异

- **Actor/SystemActor：** `servora/package/despecialization` 的语义仍有效，但 owner 已迁到 Plateau 的 `security/actor.go`；crosswalk 将 3 个 Requirement 标为“已过时”只表示历史 Servora 包位置和迁移叙事过时，不表示 Actor 规则消失。
- **CAP v2：** `plateau/security/cap` 的 JSON、scope、一次性消费和失败语义保留；“迁移前生成的旧有状态 token 继续读取”这一 Scenario 过时，当前实现以 `cap:v2` 和签名/Redis v2 边界为准。
- **Proto-utils bundler：** 当前是 `moduleResolution: bundler`；Node16/NodeNext 条件只能作为兼容目标或历史差异记录。`.js` type-only import 与 ESM sidecar 仍是当前生成/消费合同。
- **Bootstrap config：** 当前名称是 `CheckRequired`/`ApplyConf`；default 只对传递性含 default 的子树 allocate，未配置的 oneof/source 和无 default 子树继续保持 nil。不能把历史 `ValidateConf` 或无条件递归 allocate 搬入正文。
- **Transport advertise：** server 读取 `server.http.advertise`/`server.grpc.advertise` 解决注册地址覆盖；Registry/Discovery 是另一条配置与职责链。crosswalk 按 `server/http`、`server/grpc` 的 advertise 代码和测试核对，不能映射到 Registry。
- **Audit scaffold：** 当前目录、go.mod、Proto、Wire 与运行代码仍在；状态正文说停止维护、不作为新模板。因此目录结构、模块和 Wire Requirement 分别保留为“代码或配置存在但未验收”，不整体判为过时。
- **Makefile integration：** 8 个 Requirement/21 个 Scenario 属于业务仓库迁移历史；当前任务明确不启动额外迁移，标“已过时（范围外）”。

### 事实边界

- 本轮未运行 Servora 自身全量测试、Proto lint/生成、前端构建或 Node 测试；未连接 PostgreSQL、Kafka、ClickHouse、Redis、OpenFGA，也未做真实 OIDC discovery/JWKS、Cookie、CAP 或浏览器 smoke。crosswalk 的源码/测试链接是证据入口，不是执行记录。
- 主代理已运行 `go test ./security/... ./infra/... ./cmd/...` 并通过；该事实不能把 Plateau 安全/基础设施主题的所有 Scenario 标为“已实现并验证”。
- Audit 的 `consumer_test.go`、`batch_writer_test.go` 和 Servora audit helper tests 只证明有针对性测试入口；本轮未运行，且不覆盖 Kafka/ClickHouse/部署链路。
- 六个原归属疑点已按源码和原 Requirement 的明确职责关闭；没有将 Mail 缺少平台发送器误报成缺陷，也没有为匹配旧 ClickHouse 目录新增实现。历史表的“代码或配置存在但未验收”只说明相关入口和合同已有依据，不保证该条所有 Scenario 已满足。
