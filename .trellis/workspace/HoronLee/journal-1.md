# Journal - HoronLee (Part 1)

> AI development session journal
> Started: 2026-09-13

---


## Session 1: 建立项目规范并完成 0 号任务收尾
<!-- trellis-session: v=2 fp=e81746b51adece27 -->

**Date**: 2026-09-13
**Task**: 建立项目规范并完成 0 号任务收尾
**Package**: plateau
**Branch**: `main`

### Summary

完成 OpenSpec 到 Trellis 的项目规范建设、业务 spec 目录扁平化、工作提交及任务归档；开发者和 AI 共用同一套规范。

### Main Changes

- 形成 11 个 package、80 份规范；落实服务 layout/layers、biz/data 写法、CRUD 使用以及平台与母框架边界。
- 六个业务 package 的 21 份 Markdown 直接放在包根；更新配置说明、全部引用及本地包根索引发现。
- 核对 69 份历史规范的 467 条 Requirement、974 个 Scenario，明确静态存在、过时与未验收边界。
- 0 号任务已归档至 .trellis/tasks/archive/2026-09/00-bootstrap-guidelines，归档相对链接与 JSONL 已复核；会话绑定已解除。
- 保留无关 .gitignore 与 .mcp.json 的暂存状态；未推送，未改业务源码、生成物、联调配置及历史 OpenSpec。

### Git Commits

| Hash | Message |
|------|---------|
| `e5161916894940cae9d213f0d3f76f01d9a4d115` | chore(trellis): 初始化工作流并建立项目开发规范 |

### Testing

- [OK] 8 项 Python 发现回归、脚本语法检查和 just --fmt --check 通过。
- [OK] 归档前后 100 份 Markdown、5077 个本地链接均有效；80 份规范均有主题索引。
- [OK] implement/check 各六条清单有效，归档后两角色各完整读取九份文档、82416 bytes，无缺失或截断；不等同于原生 hook 事件验收。
- [OK] 此前定向 go test ./security/... ./infra/... ./cmd/... 通过；未运行完整产品 lint 或真实业务端到端验证。
- [OK] 工作及归档提交前 git diff --check 与 git diff --cached --check 通过。

### Status

[OK] **Completed**

### Next Steps

- 后续 Trellis 更新时复核 packages_context.py 的包根索引兼容；新增业务规范延续包根 index.md 与实际主题布局。


## Session 2: Servora 与 Plateau 统一为单 Go module
<!-- trellis-session: v=2 fp=d318a9c47417d10e -->

**Date**: 2026-09-15
**Task**: Servora 与 Plateau 统一为单 Go module
**Package**: plateau
**Branch**: `main`

### Summary

Servora 合并生成代码到根 module、发布 v0.9.7 并验证 Buf CI 自动维护 BSR labels；Plateau 合并 API 与三个服务到根 module，移除仓库 go.work，修正 Just/Go tools/集成测试入口并同步可执行规范。

### Git Commits

| Hash | Message |
|------|---------|
| `1d331c0d` | refactor(go)!: 合并生成代码到根模块 |
| `d1aab7dc` | docs(api): clarify automatic BSR publishing |
| `1f907e0a` | refactor(go)!: 合并平台后端到根模块 |

### Testing

- [OK] 两仓固定 Go 1.27.0 且 GOWORK=off 的 tidy/list/build/lint/generation 门禁通过。
- [OK] Plateau 排除既有 IAM 配置基线项后的全仓短测试、三个 leaf build/lint 与 Example SQLite 集成测试通过。

### Status

[OK] **Completed**

### Next Steps

- 保留 Admin 初始化与 spec 重组的并行工作区改动，另行完成对应任务。


## Session 3: 配置契约修复与 Servora v0.9.9 发布
<!-- trellis-session: v=2 fp=2d743d62ddd01906 -->

**Date**: 2026-09-23
**Task**: 配置契约修复与 Servora v0.9.9 发布
**Package**: servora
**Branch**: `main`

### Summary

完成两仓配置契约提交，发布 Servora v0.9.9，验证 Plateau 正式依赖并归档任务。

### Main Changes

- 配置处理统一为 Apply；修复默认值、必填、集合、跨包和失败清理，相关说明使用易读中文。
- Servora main 与 v0.9.9 标签已推送，GitHub Release 和 BSR 发布成功；Plateau Go 依赖与插件版本升级为 v0.9.9，BSR 保留默认引用并由锁文件固定。

### Git Commits

| Hash | Message |
|------|---------|
| `fe93c3d397cb228d9e3213711ef227f83ae9cc08` | fix(conf)!: 统一配置应用契约 |
| `0cf02934` | refactor(conf)!: 接入统一配置应用契约 |
| `d25ff73a` | build(deps): 升级 Servora 至 v0.9.9 |

### Testing

- [OK] Servora 本地753项短测试通过；远端 main CI、Release 与 Buf CI 成功。
- [OK] Plateau GOWORK=off 的 list/build、257项短测试、根目录及服务 lint、三个服务构建、TypeScript检查与 Example真实HTTP请求均通过。

### Status

[OK] **Completed**

### Next Steps

- 第三仓 servora-example 升级时需要迁移 logger.New 的三个返回值；本次未修改该仓。


## Session 4: Go 1.27.3 升级与 etcd 重试修复
<!-- trellis-session: v=2 fp=d1d280e04dc95e8c -->

**Date**: 2026-11-09
**Task**: Go 1.27.3 升级与 etcd 重试修复
**Package**: plateau
**Branch**: `main`

### Summary

完成三个本地模块 Go 1.27.3 升级和指定语法优化，移除旧示例工作区成员，修复 etcd 重试通知及既有集成 fixture；四仓业务改动已提交，任务已归档。

### Main Changes

- Plateau、Servora、TCP 模块及父级 go.work 升至 Go 1.27.3；servora-example 四成员移出，仅本机工作区修改，不提交父级 go.work。
- 迁移 new(expr)、errors.AsType、strings.CutPrefix；删除四个私有取址 helper；修复 SQLite fixture 构造参数与 etcd 失败信号通道。
- GitNexus/OpenSSL 临时绕过及永久修复待办记录到 nix-config/TODO.md；nix-config 的 Zed 设置改动保持未提交。
- 任务归档至 .trellis/tasks/archive/2026-11/11-08-go-1-27-3-upgrade；直接 main 任务使用 skip-branch-validation，补齐未跟踪目录造成的自动归档提交失败，并修复归档上下文引用。

### Git Commits

| Hash | Message |
|------|---------|
| `3f87fe124579306dab20a7514f5977e38317948e` | build: 升级 Go 1.27.3 并精简语法 |
| `599252632613439266bf08b6c7524d23a1a838d8` | build: 升级 Go 1.27.3 并精简语法 |
| `265559baa8eb3534629a861e5f21f29847093d1b` | fix(registry): 明确 etcd 心跳失败重试信号 |
| `cb9d326a477dc56384cdff185a87a6e68e44f630` | chore(go): upgrade to Go 1.27.3 |
| `945e282a5ff1ea9cfb5b9a24cf26e1f41b305951` | docs: note GitNexus OpenSSL workaround |

### Testing

- [OK] Plateau 独立 build、27 个测试包的 short/race 测试与全仓 lint 通过。
- [OK] Servora 独立 build、45 个测试包的 short/race 测试与全仓 lint 通过；etcd 故障注入 20 轮通过。
- [OK] TCP build/test、三模块工作区 build、真实 SQLite integration、CLI/插件生成执行和服务二进制 smoke 均通过。
- [OK] CGO 验证按命令临时使用 Xcode clang/SDK；已恢复同版本 Nacos 缓存并清理旧构建索引；无依赖升级和全局配置修改。

### Status

[OK] **Completed**

### Next Steps

- GitNexus/OpenSSL 永久修复另见 nix-config/TODO.md；本任务不发布版本标签。


## Session 5: 完成 10-18-admin-v1-iam 实现、会话过滤公共化与结项归档
<!-- trellis-session: v=2 fp=9e400984387928ea -->

**Date**: 2026-11-12
**Task**: 完成 10-18-admin-v1-iam 实现、会话过滤公共化与结项归档
**Package**: plateau
**Branch**: `main`

### Summary

完成 IAM 用户软删除/恢复/永久删除、管理建号与发信拆分、首次改密、自助删除及 IAM Web 交互；提炼 security/session.NewFilter 三类路径防护并清理 *bootstrap.Runtime 误注入与冗余判空；通过全量单元/集成测试与 lint 并归档任务。

### Git Commits

| Hash | Message |
|------|---------|
| `1b44d10a` | feat(security): 统一浏览器会话 HTTP 路由过滤与跨源写防护 |
| `23075871` | feat(iam): 补齐账号生命周期、软删除恢复、永久清理、自助删除与首次改密 |
| `f3a2bc18` | chore(api): 同步 protoc-gen-go 生成器版本注释 |
| `40439aca` | docs: 同步 IAM 与会话安全规范、AGENTS 编码规范及 TODO 清单 |

### Status

[OK] **Completed**
