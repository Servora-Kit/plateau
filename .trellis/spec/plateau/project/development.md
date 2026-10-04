# 本地开发与检查入口

仓库使用单一根 Go module；父级工作区只用于本机跨仓联调，仓库门禁须在 `GOWORK=off` 下独立运行。命令从仓库根目录执行，服务级命令从对应 service 目录执行。

## 固定端口

每个应用保留 10 个端口：+0 HTTP、+1 gRPC、+2 Web，其余预留。新增应用使用已登记的空闲段，不重排已有编号；具体分配以[项目约定](../../../../AGENTS.md)为准，不在规范重复业务端口表。

原生服务与容器宿主映射不能占用同一端口，Web dev 与 preview/start 不同时运行。公开入口变更须同步 origin、Web 端口和后端代理地址。

## 跨仓联调

父级 Go/pnpm workspace 仅作为本机源码联调层，不提交到仓库。pnpm 使用最近的 workspace；跨仓命令显式从共同父目录执行，独立第三方 workspace 保持自己的依赖和 lockfile。

本地包链接在安装后生效；exports 指向构建产物的共享包必须先构建或启用 watch，不能把 workspace 注册等同于已可消费。

## 命令及副作用

| 入口 | 用途 |
| --- | --- |
| `just init` | 安装 CLI／插件及冻结锁文件的 pnpm 依赖，会产生本地安装副作用 |
| `just gen` | 根 API 生成后执行各服务 OpenAPI、Wire、Ent；修改 Proto 后使用 |
| `just wire` | 各服务 Wire 装配刷新 |
| `just lint` | API TS typecheck、Buf lint、从根 module 执行的全仓 Go lint；不等于全仓 Go 测试 |
| `just api-ts-check` | 共享生成 TS 契约检查 |
| `just web::<应用>::dev`／`build`／`lint` | 应用 Web 开发、构建与 lint；其他工具使用所属 package 的 pnpm script |
| `just openfga-model-validate`／`test` | 本地 model 与场景检查 |
| `just openfga-model-apply` | 修改 model 后的应用步骤，会写外部 model／环境配置，需在明确目标环境执行 |

根插件安装中 Plateau AuthN/AuthZ 必须来自当前 checkout。Servora 工具版本由当前 Just 变量配置；不能因为父级 workspace 引用源码就假定已安装的插件也已同步。服务 Wire/Ent 生成器由根 `go.mod` 的 `tool` 声明固定，并通过 `go tool` 执行。生成路径与入口详见 [generation](../../api/proto/generation.md)。

开发或审查时先选择受影响模块和检查命令；记录实际执行与未执行项。基础设施 provider 的业务日志由 data/bootstrap 边界决定，不在构造函数中重复记录成功或失败。

## Go module 与门禁

- 根 `go.mod`、`go.sum` 统一管理共享代码和服务，不在生成目录或应用目录新增 Go module，也不提交 `go.work`。
- Wire/Ent 工具版本由根 `tool` 声明维护，通过 `go tool` 执行；本机工作区不能替代仓库依赖和生成器版本验证。
- 受影响代码须通过 `GOWORK=off` 的构建、lint 和测试；根门禁覆盖全仓，服务级检查只提供定向反馈，不能互相替代。
- 依赖变更执行 `go mod tidy` 并确认结果稳定；父级工作区通过而独立检查失败时，不视为验收通过。
- 验证记录留在任务或变更说明，不写入规范。

入口：[根命令](../../../../justfile)、[服务注册](../../../../just/services.just)、[服务命令](../../../../just/service.just)。

