# Servora CRUD 嵌套映射与资源名错误结构化优化

## 目标

补齐 `servora` CRUD 生态中两处高频能力缺口，消除 `plateau` 各微服务在 `data` 层与 `service` 层的重复手写样板代码，并清理 `TODO.md` 对应待办。

## 已知事实

- `protoc-gen-servora-crud` 已为嵌套子消息生成带点号路径的 `FieldPath` 常量（如 `userpb.UserFields.ProfileName`，`String() == "profile.name"`），`contrib/db/entgo/crud.ClearHelper` 也已支持嵌套路径，但 `servora/core/crud/mapper/mapper.go:167` 仍拒绝含 `.` 的 `FieldPath`，迫使 `app/iam` 手写 `WithPostToDTOHook` 组装 `User.Profile`。
- `servora/cmd/protoc-gen-servora-crud/generate_go.go:135` 生成的 `Parse<Resource>Name` 解析失败时仅返回裸 `fmt.Errorf`，迫使 `app/iam`、`app/admin`、`app/example` 均在 `service/user.go` 重复编写私有 `parseUserName` 包装以返回 `400 CRUD_ERROR_REASON_INVALID_RESOURCE_NAME`。
- 生成伴生文件 `*_crud.pb.go` 受插件规范约束不得引入运行时包 `core/crud`，但可直接引入纯 Proto 错误生成包 `github.com/Servora-Kit/servora/api/gen/go/servora/crud/v1`。

## 需求与验收标准

- [x] **AC-1（嵌套 Message 声明式映射与同名拍平）**：`crudmapper.WithFieldMapping(poField, resourceField)` 支持单层及多层 singular 子消息的点路径（如 `profile.name`），`crudmapper.WithFlatten(subMessages...)` 支持单层/多层 singular 子消息按同名规则自动投影并可与显式 `WithFieldMapping` 混用；构造期校验路径段存在性、非列表/非字典/非 WKT、防歧义与防顶层 `IDENTIFIER` 误拷贝。当子消息下至少一个映射值存在时按需初始化子消息并深拷贝赋值；全空时保持子消息指针为 `nil`。
- [x] **AC-2（资源名解析结构化错误）**：`protoc-gen-servora-crud` 生成的 `Parse<Resource>Name` 与 `Validate()` 在失败时直接返回 `crudv1.ErrorCrudErrorReasonInvalidResourceName(...)`（HTTP `400`，`CRUD_ERROR_REASON_INVALID_RESOURCE_NAME`），且不引入 `core/crud` 依赖。
- [x] **AC-3（Plateau 样板清理与联调回归）**：本机从 `../servora` 重装 `protoc-gen-servora-crud` 并重跑两仓代码生成；移除 `app/iam/service/internal/data/user.go` 的 `Profile` `WithPostToDTOHook`（精简为 1 行 `crudmapper.WithFlatten(userpb.UserFields.Profile)`），移除 `app/iam`、`app/admin`、`app/example` 三处 `service/user.go` 的重复私有错误包装（`admin` 一并移除仅用于 `ParseName` 的冗余 `plan` 字段）；更新两仓对应规范与 `TODO.md`，两仓测试与 lint 全部通过。

## 本期不做

- 不支持 `repeated` 列表消息或 `map` 字段内部的点路径映射。
- 不在本任务单独发版 `servora`（开发期通过父目录 `../go.work` 联调；等 `admin-v1` 整体收尾后再统一发版 `servora` 并升级 `plateau/go.mod` 与 `justfile` 版本号）。
