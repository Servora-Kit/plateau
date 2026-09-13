# 0 号任务设计：Trellis 规范结构与迁移

状态：结构与职责设计已落实，规范正文、历史核对和文档验收已完成；结果见 `research/verification.md`。

本文承接 `prd.md` 的 R1–R6，确定规范归属、目录组织、任务上下文及历史核对方式。首批规范已按下述结构建设，实际入口见 [规范总索引](../../../../spec/index.md)。执行顺序与验证命令另写入 `implement.md`，实施证据保存在 `research/`。

本任务同时承担项目开发规范的重新梳理与确立。开发者明确的约定作为规范依据，当前源码用于提供实例和核对执行现状；两者有偏差时记录差异，不把旧写法自动当作推荐标准。代码整改仍按 PRD 的范围边界另行处理。

## 1. 当前基线与设计边界

- `.trellis/config.yaml` 已包含五个共享／框架 package 和六个业务端 package，默认 package 为 `plateau`；源码路径及 Trellis 解析已经检查。
- 规划时 `.trellis/spec/` 为初始化结构；实施中已归并旧 `client`、`gen` 等目录并建立业务端正文。旧文件去向见 [迁移映射](research/migration-map.md)，启动时内容已保存逐文件副本。
- 当前根 `cmd/` 只有 `protoc-gen-plateau-authn` 和 `protoc-gen-plateau-authz`，均调用 `internal/codegen/ruleplan`，测试共同使用 `internal/codegen/plugintest`。
- `app/AGENTS.md` 将 IAM 定位为主要业务实践、内部 Example 定位为参考应用，将 Audit 标记为不再更新和参考、等待重构。
- `app/admin/web` 当前为空目录，暂不登记为已实现的业务端 package。
- 本设计沿用现有工作区联调方式。工作围绕规范、任务资料和必要导航展开；按开发者收尾前的目录修订，补充本地 package 根索引发现及其回归检查，不修改业务代码、生成产物或历史 OpenSpec。

现状证据入口：`.trellis/config.yaml`、`app/AGENTS.md`、`cmd/protoc-gen-plateau-authn/main.go`、`cmd/protoc-gen-plateau-authz/main.go`、各插件的 `main_test.go`。

## 2. 规范寻址与目录约定

业务规范采用 `.trellis/spec/<package>/<topic>.md`；共享规范按需要采用 `.trellis/spec/<package>/<layer>/<topic>.md`：

- `package` 使用已配置的职责名称或 `<应用>-service` / `<应用>-web`。
- `layer` 是可选的、具有实际意义的规范分组，可以叫 `project`、`security`、`codegen`、`client`、`framework` 等；业务包已用 `-service` / `-web` 表达职责，不再套 backend/frontend。
- `topic` 按具体问题命名，例如 `authN.md`、`authZ.md`、`cmd.md`、`audit.md`。不要求复制模板中的文件名称或数量。
- 每个正文分组提供 `index.md`，列出适用范围、开发前检查、质量检查和实际主题索引。
- 业务 package 根 `index.md` 直接索引同目录的主题。含子分组的 package 根索引承担跨分组导航，各实际分组仍保留自己的入口。
- 每份重要规范应能独立说明适用范围、规则、来源与验证方法；关联主题通过链接和任务清单组合，不依赖读者猜测隐含前提。

本地 package 发现同时报告包根 `index.md` 与实际子目录索引：JSON 的 `specIndex` 表示存在的包根索引或 null，`specLayers` 只列真实子层；文本和会话摘要也提供包根入口。六个业务包没有子层是正常结果，不能误报为未配置。任务 JSONL 继续显式列出需要读取的正文，索引链接不会自动展开。

机制依据：`.trellis/scripts/common/packages_context.py` 的 `_scan_spec_layers`、`get_packages_info`、`get_context_packages_text`，以及 `.trellis/workflow.md` 的 Spec System 与 Planning Artifacts。

## 3. Package 与主题设计

下表列出首批规范分组及主题。主题清单用于指导源码调查和编写；发现主题重复时合并，发现独立契约时补充，不为满足表格数量建立空文档。

| Package / layer | 首批主题文件 | 主要来源与职责 |
| --- | --- | --- |
| `plateau/project` | `structure.md`、`boundaries.md`、`development.md` | 根 `AGENTS.md` 的“目录结构”直接作为结构正文基础；结合 README、Just 入口和部署配置整理平台职责、端口与开发方式 |
| `plateau/security` | `authN.md`、`authZ.md`、`actor.md`、`capabilities.md`、`credentials.md` | `security/`；认证、授权、执行主体、能力及凭据／会话工具 |
| `plateau/infra` | `openfga.md`、`clickhouse.md` | 当前 `infra/openfga/`、`infra/clickhouse/`；平台基础设施适配与生命周期 |
| `plateau/codegen` | `cmd.md`、`ruleplan.md` | 根 `cmd/` 与 `internal/codegen/`；插件实现和共享生成逻辑 |
| `api/proto` | `contracts.md`、`annotations.md`、`generation.md` | `api/protos/`、`api/AGENTS.md`、各应用源 Proto、Buf 配置；资源、查询、字段更新、错误与兼容性契约，以及生成物归属 |
| `web/client` | `architecture.md`、`http.md`、`sse.md`、`websocket.md` | `web/packages/client/src/` 与 `test/`；共享通信设计、协议与消费边界 |
| `service/backend` | `layout.md`、`layers.md`、`coding.md`、`crud.md`、`testing.md` 及各层编码规范（见 3.1、3.4、3.5） | `app/AGENTS.md` 的“服务结构”与开发者确认的写法，结合 Example 和 IAM；目录布局、职责分层、推荐编码方式、Servora CRUD 使用流程、测试与装配 |
| `servora/framework` | `architecture.md`、`bootstrap.md`、`crud.md`、`transport.md`、`audit.md`、`observability.md`、`providers.md`、`tls.md` | `../servora/core/`、`../servora/transport/`、`../servora/obs/`、`../servora/contrib/`、`../servora/security/`；框架运行时及扩展约束 |
| `servora/proto` | `annotations.md`、`generation.md` | `../servora/api/`；母框架的公共 Proto、注解与生成模块契约 |
| `servora/cmd` | `cli.md`、`plugins.md` | `../servora/cmd/`；开发 CLI、生成器及各自的实现与测试约定 |
| `servora/web` | `proto-utils.md` | `../servora/web/`；框架前端共享包与生成代码之间的契约 |
| `iam-service` | `identity.md`、`sessions.md`、`oidc.md`、`authorization.md`、`startup.md` | `app/iam/service/` 与 IAM ADR；领域模型、业务流程和特有装配 |
| `iam-web` | `architecture.md`、`auth-flows.md`、`bff.md` | `app/iam/web/`、`docs/adr/0004-separate-iam-op-ui-from-oidc-rp-bff.md`；应用基本组织、请求与状态管理，以及操作界面／RP BFF 边界 |
| `example-service` | `user-crud.md` | `app/example/service/`；参考资源与 CRUD 实践，引用共通分层规范 |
| `example-web` | `architecture.md`、`requests.md`、`forms.md` | `app/example/web/` 及其 AGENTS；应用基本组织、生成客户端消费、请求控制台与交互 |
| `audit-service` | `status.md`、`ingestion.md` | `app/audit/service/` 与 `app/AGENTS.md`；维护状态、消费和存储实现的适用限制 |
| `test-web` | `purpose.md` | `app/test/web/`；现有用途、基本组织和构建验证入口，按实际规模合并说明 |

`guides/` 继续存放通用思考指南，不承担某个应用或框架模块的权威规则。

这些分组不新增 config package。`servora/cmd` 直接对应 `../servora/cmd/`，其中 `cli.md` 描述开发 CLI，`plugins.md` 描述生成插件。`plateau/codegen` 当前集中于两个平台安全生成插件及共享生成实现。后续可以在各自 package 内继续按能力调整分组。

### 3.1 service/backend：目录布局、职责分层与编码规范

`layout.md` 和 `layers.md` 分别保留，明确区分文件应放在哪里、各层应承担什么职责。各层的实际写法另用主题文档说明，避免只给出目录树或依赖箭头就认为开发规范已经完整。

| 文件 | 负责回答的问题 | 应覆盖的内容 |
| --- | --- | --- |
| `index.md` | 开发或审查当前改动应读取哪些规范 | 适用范围、按修改位置选择正文、开发前检查与质量检查 |
| `layout.md` | 项目中的文件和目录放在哪里 | `api/`、`cmd/`、`configs/`、`internal/`、模块与命令文件的布局；手写与生成内容的位置；目录扩展方式 |
| `layers.md` | 每个内部层承担什么职责、允许依赖谁 | `server`、`service`、`biz`、`data` 的责任、调用与依赖方向，业务／协议／存储边界；业务专有子包的归属原则 |
| `coding.md` | 各层共同遵循哪些编码约定 | 当前项目的命名、构造与依赖注入、context 传递、错误和日志处理、注释及测试写法；仅记录跨层共通规则 |
| `crud.md` | 如何在微服务中使用 Servora CRUD 生态 | 以 Example 为参考，串联各层协作、组件声明与构造、字段配置与绑定、映射和响应输出的推荐流程；具体重点见 3.4 |
| `testing.md` | 各层如何测试、何时需要集成验证 | 单元测试边界、依赖替代、数据层与跨层集成验证条件、当前检查命令；业务和框架特有要求由各自正文补充 |
| `server.md` | HTTP/gRPC 等服务端如何编写 | 服务与路由注册、中间件装配、ProviderSet 和服务端构造方式，以及与业务适配层的边界 |
| `service.md` | 接口适配层如何编写 | 生成接口的实现与嵌入、请求规范化、Usecase 调用、响应与错误适配，以及避免业务和存储逻辑进入该层的具体写法 |
| `biz.md` | 业务层如何编写 | Usecase、Repo 接口、业务流程组织和领域约束；对 data 实现的隔离，以及业务测试方式 |
| `data.md` | 数据访问层如何编写 | Repo 实现、数据映射、存储错误处理、事务与资源生命周期；`NewData`、schema 和生成代码的边界，以及不同存储实现的适用差异 |
| `bootstrap.md` | 一个微服务如何完成启动装配 | `cmd/server`、Wire、配置加载、ProviderSet 的组合、初始化顺序与清理；应用特有启动逻辑引用业务 package 的规范 |

这里的 `service` package 代表整套微服务共通规范，`service.md` 正文专门描述源码中的 `internal/service/` 层。

原先拟定的 `composition.md` 内容按职责进入 `layout.md` 和 `bootstrap.md`，`persistence.md` 内容进入 `data.md`，不再保留重复主题。`layers.md` 负责边界，具体编码文档负责边界内的推荐实现；跨层共有规则只在 `coding.md` 定义。

每份编码规范都应包含适用场景、推荐写法、真实源码或测试引用、常见反模式和检查要求。优先从 Example 与 IAM 中提炼共通模式；应用差异留在对应业务 package，并说明原因。新增业务专有子包时，根据职责寻找规范归属，不机械地为每个源码目录新增一份共享规范。

编写与审查任务按实际涉及的层组合读取 `coding.md` 和对应层正文。使用或调整 Servora CRUD 时加入 `crud.md`；改动目录结构时再加入 `layout.md`，改变职责或依赖关系时加入 `layers.md`。读取索引不能代替这些正文。

### 3.2 指定 AGENTS 章节的整理方式

开发者已明确以下正文来源，整理时保留原有层次和有效约定，不退回到通用模板重新描述项目：

| 正文来源 | 目标规范 | 整理要求 |
| --- | --- | --- |
| 根 `AGENTS.md` 的“目录结构” | `plateau/project/structure.md` | 以整段目录说明为基础，包括 `api/` 的源 Proto、生成目录与包配置子项，以及其他顶层目录和配置文件的职责 |
| `app/AGENTS.md` 的“服务结构”中的目录树和位置说明 | `service/backend/layout.md` | 保留服务根目录、模块、API、配置、命令入口、internal 子目录以及手写／生成文件的位置关系 |
| 同一章节中的职责和依赖说明 | `service/backend/layers.md` | 保留 server/service/biz/data 的职责及依赖边界，包括 `service -> biz <- data` 和应用专有模块的归属 |
| 同一章节中各层文件的编码约定 | `service/backend/server.md`、`service.md`、`biz.md`、`data.md` | 保留 ProviderSet、接口实现、Usecase、Repo、NewData 等有效约定，再补入本任务中明确的写法 |

索引和正文中的链接改为目标文件位置可解析的引用。目录事实逐项核对；原文若列出已经不存在的路径，记录文档与源码差异，不为匹配旧说明补建源码目录。实施仅在根、app、api、app/example/web 四份 AGENTS 中补充相关规范导航，并同步直接相关的目录及生成产物事实；保留有效约定与 Trellis 托管块。

### 3.3 已确认的微服务编码与模块约定

以下内容来自开发者在本任务中的明确要求，作为相关规范正文的约束；源码示例用于说明其适用情况，不替代这些要求。

#### server 文件边界

`internal/server/` 的生产代码通常限定为 `server.go`、`grpc.go`、`http.go`，以及根据实际需要增加的 `sse.go`、`asynq.go` 等服务端文件。判断依据是服务端职责，文件名示例不是封闭的协议名单。业务流程、存储实现和应用专有模块应放入各自所属位置。

`server.go` 保留原 AGENTS 中以 ProviderSet 为主的约定。相关测试按测试职责组织，不将上述生产文件清单理解为删除同包测试的要求。

#### biz 文件声明顺序

在 Go 的 package 声明之后，业务文件按以下顺序组织：

1. import。
2. 该业务需要的常量和变量；无对应内容时不创建占位声明。
3. `XxxRepo` 接口；允许按业务依赖定义一组或多组 Repo 接口，不限定每个文件只能有一个接口。
4. 当前业务的 `XxxUsecase` 结构体。
5. `NewXxxUsecase` 构造函数。
6. Usecase 的方法。

此顺序写入 `service/backend/biz.md`。`biz.go` 的 ProviderSet 约定与具体业务文件的声明顺序分别说明。

#### data 文件声明顺序

在 Go 的 package 声明和 import 之后，仓储实现按以下顺序组织：

1. 私有的 `xxxRepo` 结构体。
2. `NewXxxRepo`，负责构造该私有结构体实例。
3. 该结构体实现对应 `biz.XxxRepo` 接口的方法。

此顺序与命名写入 `service/backend/data.md`。构造函数的具体签名按依赖和错误处理契约表达；本条明确的是私有实现、构造职责和声明顺序。`data.go` 中的 NewData／ProviderSet 约定单独说明，不与具体 Repo 文件混写。

#### 应用专有 internal 模块

沿用 IAM 的组织方式，`app/iam/service/internal/oidc/`、`authn/`、`mail/`、`startup/` 等业务特有模块直接位于当前服务的 `internal/` 下，与通用层目录并列。

`layout.md` 说明扩展位置，`layers.md` 说明各模块应具有清晰职责和依赖边界；模块内部的业务设计进入 `iam-service` 等应用规范。不能为了凑齐四层而强行将独立业务模块塞入 biz/data，也不能仅因它是独立目录就提升成平台共享包。

#### 已核对的例子与差异

- `app/iam/service/internal/biz/user.go` 的主要声明依次为 import、业务错误变量、`UserRepo`、`UserUsecase`、`NewUserUsecase` 和方法，可作为 biz 顺序的真实例子。
- `app/example/service/internal/data/user.go` 使用私有 `userRepo`、`NewUserRepo` 和对应方法，可作为 data 顺序与命名的真实例子。
- `app/iam/service/internal/data/user.go` 当前使用 `userRepository`／`NewUserRepository`，与本次确认的 `xxxRepo`／`NewXxxRepo` 命名存在差异。迁移核对时保留这一记录，不反向改写确认规范，也不在本次文档整理中重命名业务代码。
- 上述 IAM 专有模块目录确实存在；更深入的业务约束在编写应用规范时继续核对。

### 3.4 Servora CRUD 的使用流程与编码规范

开发者指定 `app/example/service` 为 Servora CRUD 使用规范的主要参考。目标是说明如何按推荐流程编写一个 CRUD 资源，以及各层组件如何配合；组件清单只是编写正文的入口。

规范按职责归属：

| 目标正文 | 负责内容 |
| --- | --- |
| `service/backend/crud.md` | 微服务消费 Servora CRUD 的共通流程、推荐声明与初始化方式、配置和调用方式；与 `service.md`、`biz.md`、`data.md` 互相引用，通用分层和文件顺序继续由各层正文定义 |
| `servora/framework/crud.md` | 母框架 CRUD 组件自身的设计、公共契约与内部开发约束；消费方正文引用需要遵循的具体契约 |
| `example-service/user-crud.md` | Example 的 User 资源及其业务特有配置，作为共享使用流程的具体实例，引用共通规范 |

开发者本轮重点指出以下 service 层字段及 `service.plan.ToResponse` 的使用：

```go
plan          *corecrud.ResourcePlan[*examplev1.User]
listPreparer  *corecrud.ListPreparer
parentMatcher *corecrud.ResourceNameMatcher
```

data 层重点包括以下字段，以及 `entcrud.NewListFields`、`entcrud.Columns`、字段绑定（本轮以 `bind` 指代）和 `repo.mapper.ToDTO` 的使用：

```go
listFields *entcrud.ListFields[*entmodel.User]
mapper     *crudmapper.ResourceMapper[*examplev1.User, entmodel.User]
clear      *entcrud.ClearHelper[*entmodel.UserMutation]
```

实施时围绕这些入口梳理声明、构造、配置与方法调用之间的完整关系，说明请求准备、资源名处理、Usecase／Repo 协作、字段处理、数据映射和响应输出分别如何编写。具体调用顺序、参数、字段绑定符号、适用条件、错误处理及检查要求，届时结合 Example 源码与必要的框架契约核对；不能把示例中的业务选择一律推广为框架要求。

实施已沿 Example 的 service/biz/data 与 Servora 组件完成定向核对，声明、构造、字段绑定、错误处理和调用示例见 [CRUD 使用规范](../../../../spec/service/backend/crud.md)，来源与差异见 [服务基线](research/services-baseline.md)。本文中的字段片段是设计重点，完整使用关系由正文说明。

### 3.5 基础工程规范的覆盖

以下内容补足首批规范的基础覆盖，沿现有 package 归属组织，不新增 package：

| 内容 | 权威正文与编写要求 |
| --- | --- |
| 测试与质量检查 | 微服务共通规则进入 `service/backend/testing.md`；框架、client 和业务应用在对应主题或索引中补充自己的测试方式与检查入口。区分已经存在的测试、推荐补充的验证和实际运行结果 |
| 错误、日志、context、配置与资源生命周期 | `coding.md` 说明跨层通用写法，`service.md`／`data.md` 说明错误转换与记录位置，`bootstrap.md` 说明配置和清理；框架内部机制引用 `servora/framework` 对应正文 |
| API 契约与演进 | `api/proto/contracts.md` 说明资源命名、分页／过滤／排序、字段选择与更新语义、错误和兼容性；`annotations.md`、`generation.md` 与消费方 CRUD 规范引用各自相关契约 |
| 业务前端基本组织 | IAM 和 Example 的 `architecture.md` 说明目录、路由、组件／状态职责、请求与生成 API 的使用，以及加载和错误状态；业务交互在原有主题展开。Test 按实际规模在 `purpose.md` 内说明，不人为增设框架和页面要求 |

主题正文按需要包含适用范围、规则与设计理由、推荐示例、常见错误、检查方式和相关来源。它们是内容检查维度，不要求每个文件机械使用全部标题。前端规范从各应用实际实践出发，共通能力有明确依据后再提炼到 `web`。

## 4. cmd、API 与安全能力的归属

### 4.1 根 cmd 归入 plateau/codegen

当前选择 `.trellis/spec/plateau/codegen/cmd.md`，不单独建立 `cmd` package。

这份规范覆盖：

- 平台命令与两个安全生成插件的职责、命名和代码位置。
- 插件从当前 checkout 安装，以及在现有生成流程中的入口关系。
- 插件专有的声明校验、输出契约和错误定位要求。
- 生成产物不可手改，以及插件测试与生成结果行为检查的边界。

`ruleplan.md` 覆盖共享生成实现：`optionmerge` 的默认与覆盖处理、`ruleplan` 的收集分组和输出规划、`plugintest` 的编译与行为验证方式。具体算法和约束从当前源码与测试提炼。

根 `cmd/` 已包含实际生成逻辑，不将其误写成只有参数解析的薄入口，也不借规范任务重构实现。

### 4.2 契约按责任归属

| 问题 | 权威规范位置 | 其他规范如何使用 |
| --- | --- | --- |
| Proto 如何声明注解、字段和服务契约 | `api/proto` | codegen、运行时和业务规范引用源定义 |
| Plateau 生成器如何解释、校验并生成规则 | `plateau/codegen` | API 规范说明入口，安全规范引用生成契约 |
| 认证、授权和能力在平台运行时如何执行 | `plateau/security` | 微服务规范说明适配和装配，IAM 规范补充业务策略 |
| 服务级 `cmd/server` 如何启动应用 | `service/backend` 与相应 `<应用>-service` | 共通启动组成与业务特有初始化分别说明 |
| 母框架自己的命令、插件和公共注解如何开发 | `servora/cmd`、`servora/proto` | Plateau 只引用所消费的具体契约 |

生成器实现与运行时共享的契约需要互相链接，验收时核对其一致性。名称相近的 Plateau 与 Servora 注解不得直接推定具有相同合并语义。

### 4.3 何时重新考虑独立命令 package

当前没有独立拆分需求。未来只有在根命令形成独立产品职责、稳定的公共接口与单独维护方式时，再评估增加 package；新增一个可执行目录本身不触发拆分。

## 5. 跨 package 组合与任务上下文

### 5.1 规范复用规则

- 共享规范保存共同契约，业务规范只补充应用特有规则，并明确引用所依赖的正文。
- `plateau/project` 保存项目总体边界，不堆入所有共享模块的实现细节。
- `service/backend` 描述微服务如何使用框架和平台能力，不复制框架的内部实现规范。
- `web/client` 只描述共享 client；各应用是否以及如何使用它，应从当前消费代码确认。不能因为同属前端就假定所有应用使用同一请求实现。
- `servora/framework/audit.md` 描述框架审计能力；`audit-service` 描述具体业务消费者及其停止维护状态。
- 同一规则出现冲突时，检查权威来源、实现证据和适用范围；不得靠复制或静默覆盖解决。

### 5.2 清单选择

`task.json.package` 表达任务的主要归属；跨端任务仍通过 JSONL 加载其他 package 的相关规范。规范来源路径允许重叠，但不产生自动继承。

以下是跨 package 的正文组合示例；每个具体任务只登记实际涉及且已存在的文件：

| 任务 | 需要组合的规范主题 |
| --- | --- |
| 修改 IAM 后端认证流程 | `iam-service` 的会话／身份主题、`plateau/security/authN.md`、`service/backend/coding.md`，以及实际涉及的业务、适配或数据层正文 |
| 修改 AuthZ 注解及生成行为 | `api/proto/annotations.md`、`plateau/codegen/cmd.md`、`plateau/codegen/ruleplan.md`、`plateau/security/authZ.md` |
| 修改 IAM 前端请求或登录交互 | `iam-web` 的对应主题；实际涉及共享 client 时加载 `web/client` 对应正文 |
| 修改 Example CRUD 全链路 | `example-service/user-crud.md`、`example-web/requests.md`、`service/backend/crud.md`、`service/backend/coding.md` 及实际涉及的 `service.md`、`biz.md`、`data.md`，再按影响加载 `api` 的具体契约或 `servora/framework/crud.md` |
| 修改 Servora 审计能力 | `servora/framework/audit.md`；涉及规则生成时再加载 `servora/proto`、`servora/cmd` 相关正文 |

JSONL 每条记录使用真实文件路径及具体加载原因。索引仅是导航和检查入口，不能用一个索引条目代替所有需要遵循的正文，也不使用整目录兜底加载全部规范。

`implement.jsonl` 与 `check.jsonl` 允许不同：前者覆盖写作和实现所需约束，后者覆盖验收、交叉契约与历史核对要求。两者均须有实际有效条目。

### 5.3 0 号任务的初始上下文

新规范尚不存在时，通过任务研究材料定位有效 AGENTS 与已确认约定，并加载适用的规范写作指导。初始清单使用 `research/planning-context.md` 和 `trellis-spec-bootstrap` 的 `references/spec-writing.md`；正文稳定后，implement/check 清单均补入跨层指南、平台边界、API 注解及 CRUD 使用规范，共六条。PRD、设计与实施文档由角色上下文读取机制直接加载，不在 JSONL 中重复登记。

源码及即将修改的文件不预登记为注入正文，实施时沿研究材料中的入口按需读取。以下是研究和任务资料应引导读取的来源：

- `.trellis/workflow.md`：规划资料与执行阶段的契约。
- `AGENTS.md`、`app/AGENTS.md`、`api/AGENTS.md`：平台、微服务与 API 边界。
- `security/AGENTS.md`：平台安全能力的调查入口。
- `../servora/AGENTS.md` 及按主题选择的下级 AGENTS：母框架规范来源。

这些文档也需要与当前源码核对，不能把旧导航中已经不存在的目录写成现行规范。规范基线产生后再替换或补充清单中的实际规范正文。

任务的 `prd.md`、`design.md`、`implement.md` 由规划资料加载机制读取，不拿 JSONL 代替它们。源码与测试留作调查证据，不批量塞入规范清单。

### 5.4 配置与运行时边界

- 维持当前 `default_package: plateau`。
- `servora.git: true` 用于收集独立仓库的 Git 上下文；规范寻址和依赖联调不由这个标记控制。
- 本次不增加 `session.spec_scope` 限制；package 发现与任务实际需要加载的正文分开处理。
- 现有 `task.json.status` 不能证明当前会话已绑定任务或原生 hooks 已触发。后续通过受支持的任务机制处理真实会话绑定，不伪造标识或重置状态。
- 实际注入需要记录触发、加载和截断情况；手工读取可作为本地工作流的替代路径，但不能记为原生 hook 验收成功。
- 此次按用户确认的扁平业务目录调整本地 `.trellis/scripts/common/packages_context.py` 并补充回归检查；不修改全局安装、配置 schema 或 hooks。包根发现是本地兼容点，后续 Trellis 更新时须保留或重新核对此改动。

机制检查入口：`.trellis/scripts/common/config.py`、`.trellis/scripts/common/task_context.py`、`.codex/hooks/inject-subagent-context.py`。

## 6. 现有模板的归并

旧目录只是内容调查起点，不能只改名字就视为迁移完成。

| 当前目录或配置来源 | 目标归属 |
| --- | --- |
| `spec/api/backend`、`spec/api/frontend` | 源契约与生成规则进入 `api/proto`；实际消费约束归相应共享／业务消费者 |
| 原 `web -> app/admin/web` 映射及 `spec/web` 模板 | 旧映射已经修正；新的 `web/client` 从真实共享 client 源码建立规范 |
| `spec/client` | 按当前共享 client 的实际内容归并到 `web/client` |
| `spec/gen` | 按内容归属 Servora 源 Proto、工具或消费方；不保留生成目录独立 package |
| `spec/service` | 共通内容进入 `service/backend`；业务特有内容归对应 `<应用>-service` |
| `spec/servora/backend` | 按主题归入 `servora/framework`、`servora/proto`、`servora/cmd` 或 `servora/web` |
| `spec/iam-web`、`spec/example-web`、`spec/test-web` | 保留 package 名，按实际应用内容重写或调整主题 |
| 尚不存在的 `plateau` 和业务后端规范 | 在已确定的 package 下建立索引与真实正文 |
| `spec/guides` | 保留通用指南，检查是否仍有过时路径或不适用内容 |

归并时区分空模板、用户已有内容和已过时规范。包含有效内容的旧文件在目标内容与引用核对后再移除；同步索引、任务上下文和必要导航，避免留下失效路径。

0 号任务继续沿用现有身份。规划补齐时已将主要 package 设为 `plateau`，更新中文标题、描述和 `relatedFiles` 至配置、规范及相关导航范围，保留与本次无关的元数据。`relatedFiles` 是变更范围记录，允许列出待建设目录；它不承担上下文注入职责。

## 7. 历史 OpenSpec 核对设计

结合开发者确认的规范、当前源码、测试和有效文档形成基线后，再核对 `../openspec/specs/`；必要时读取归档历史。核对范围以本任务的 package 和职责为准，范围外主题明确标记为未纳入，不据此启动额外迁移。

核对记录保存在任务内的 `research/openspec-crosswalk.md`；历史发现进入 `research/openspec-findings.md`，实施差异和处置结论汇总到 `research/gaps.md`，不创建空占位文件。

开发者确认规范与当前代码之间的差异同样进入差异记录，注明确认依据、目标规范、实际文件和当前状态；不要求必须存在对应的历史 OpenSpec 条目。旧规范核对和新约定落实情况分别可查。

每条核对记录包含：

| 字段 | 内容 |
| --- | --- |
| 历史来源 | 文件路径、Requirement 名称及 Scenario；多场景可分别记录 |
| 当前归属 | package、规范主题及目标规则 |
| 当前证据 | 源码符号、测试、有效 ADR 或配置 |
| 状态 | 已实现并验证／代码或配置存在但未验收／旧规范未实现／已过时／待确认 |
| 判定依据 | 实际验证结果、未运行项目、冲突或适用范围 |
| 后续处理 | 纳入现行规则、记录维护限制、另立问题或等待设计确认 |

“已实现并验证”必须说明验证层次和证据；静态代码检查不能代替运行验收。历史规则有合理设计意图但源码尚未满足时，记录差异，不能把缺陷写成推荐惯例。

最终 spec 保留当前可执行的约束、必要的设计意图和已明确的维护限制；历史逐条对照和临时结论留在任务资料。历史 OpenSpec 本身保持原位。

## 8. 验收设计与证据

| 对应需求 | 应观察到的结果 | 验证边界 |
| --- | --- | --- |
| R1、R2 | package 键唯一、路径存在、默认项有效，业务端命名正确，Servora 独立仓库标记被解析 | 配置检查已完成；不代表新规范已存在 |
| R3 | 目标分组被 Trellis 发现，输出的索引真实存在，主题与适用源码对应 | 直接扫描目录和发现输出，不只检查 YAML 语法 |
| R4 | 指定 AGENTS 章节的有效内容得到保留；共享与业务规则不互相复制；server 文件边界、biz/data 声明顺序与命名、应用专有 internal 模块的归属、Servora CRUD 使用流程与推荐写法均有明确正文 | 对照开发者确认要求、章节来源、源码示例与实际加载的编码规范逐项检查；沿 Example 核对 CRUD 组件声明、构造、配置和跨层调用的完整关系 |
| R4 基础覆盖 | 测试与质量检查、错误／日志／配置／生命周期、API 演进契约及业务前端基本组织都有适用规范 | 按 3.5 对照主题、来源及检查入口，不要求每个应用复制相同文件清单 |
| R5 | 已确认约定、当前实现差异和范围内历史规则均有可追溯依据，状态与证据一致 | 未运行、尚未整改或未知项保留准确标记 |
| R6 | 规划资料、任务元数据、有效 JSONL 与实际读取一致 | 缺失文件被跳过、索引未展开或注入被截断，均须识别和处理 |
| 范围保护 | 变更仅涉及授权的配置、规范、任务资料及必要导航 | 对比本轮及迁移前基线，保留既有改动与暂存状态 |

文档质量检查包括：索引与正文对应、引用路径有效、没有空模板、推荐模式有证据、维护状态准确。简单关键词匹配只用于定位疑点，不能把真实示例中的词语或待确认的历史记录直接当作模板残留。

本任务的主要验收是规范、引用和上下文验证。未来规范中描述的业务测试、构建、生成或部署命令，不因被写入文档就视为本轮已经运行；确需运行时按 `implement.md` 明确其范围与副作用。

## 9. 兼容性与回退边界

- 配置只依赖已存在的 package、path、default_package 和 Git 标识能力；正文分组由目录发现，不扩展配置 schema。
- 新规范布局通过索引及显式任务清单接入现有工作流，不增加隐式继承或全量加载机制。
- 迁移前记录受影响文件、既有未提交内容及暂存状态；旧内容与新内容的归属关系可追溯后再移除旧路径。
- 若布局或引用检查失败，只修正本任务涉及的规范、索引和清单；不得用整仓恢复、清理或覆盖模板来消除用户既有改动。
- 若发现其他布局兼容问题，先核对包根索引与真实子层，不扩大到无关的 Trellis 运行时改造。

## 10. 实施与资料状态

启动前产物包括中文 `prd.md`、本设计、`implement.md`、`research/planning-context.md`、真实的 `implement.jsonl`／`check.jsonl`，以及已同步的任务元数据。启动前检查结果记录在 `research/planning-review.md`。

开发者发出“开始执行”后，已完成真实会话激活、源码基线调查、规范正文建设、审查修订及历史对照。超出已定范围的业务修改保留在差异资料中另行处理，未混入本轮实施。

启动记录见 [执行基线](research/execution-baseline.md)，完成项以 `implement.md` 与 PRD 验收清单为准。上下文读取预览与宿主 hook 触发分别记录；提交、推送和归档不随文档实施自动进行。
