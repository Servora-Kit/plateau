# AGENTS.md - servora-platform

<!-- Updated: 2026-07-30 -->

## 项目概览

`servora-platform` 是 Servora 的平台服务与参考应用仓库，当前包含 Audit 微服务和单一 User CRUD 参考应用。

## 依赖关系

- go.work 用于连接代码生成与各个微服务依赖
- Proto BSR 依赖：`buf.build/servora/servora`
- `pnpm-workspace.yaml` 独立管理 `api/gen` 与 `app/*/web`，仓库根 `pnpm-lock.yaml` 作为共享锁文件；顶层 `servora-kit` workspace 仅用于本地跨仓联调

## 开发规范

- 仓库根修改 proto 后执行 `just gen`，统一刷新 Go API、共享 TypeScript HTTP API 包、OpenAPI、Wire 与 Ent 产物；需要服务自有 Web client 时，在对应 leaf 执行 `just api-ts`
- 修改 Wire 依赖图后执行 `just wire`
- 不要手改 `api/gen/go/*`、`api/gen/ts/*`、`app/*/web/src/api/generated/*`、`wire_gen.go`
- 同一 API 的 gRPC 与 HTTP 必须由一个 Proto `service` 定义，HTTP annotation 与 RPC 放在同一个领域 Proto；不要创建 `i_xxx.proto` 或复制 `XxxHTTPService`
- `servora-platform/app/example` 是当前servora最标准的微服务使用流程，也是新增平台级微服务的推荐模板。微服务的后端分为：
- api
  - gen/go 所有微服务的 Go Proto 生成输出目录
  - gen/ts 所有微服务的 TypeScript Proto 生成输出目录
  - gen/package.json 管理共享 TS 包依赖与 exports
- app/ 微服务全都在这个目录里面
  - {ServiceName/}|{OtherServiceName/}
    - service/ 后端
      - api/ 本微服务proto接口定义和私有配置文件proto定义
        - protos/{DomainName}/service/ 该微服务各个领域的 Proto API 定义
        - protos/{ConfigName}/conf.proto 该微服务自己的业务配置 proto
        - buf.openapi.gen.yaml 该服务的OpenAPI的 buf 生成配置
        - buf.typescript.gen.yaml 可选的服务级模板；仅在服务 leaf 执行 `just api-ts` 时使用，并按模板自己的 `out` 生成到服务 Web。仓库根 `buf.typescript.gen.yaml` 则为 `buf.yaml` 中全部模块生成共享 HTTP client，两者互不清理
      - cmd/ 启动入口，一般只包含 server/
      - configs/local|docker 本地/容器运行时配置
      - internal/ 业务逻辑，其中包含
        - assets/ 默认包含protoc-gen-openapi所生成的openapi.yaml
        - server/ http/grpc等服务层
          - server.go 除了ProviderSet，一般不能有别的方法
          - http.go|grpc.go 服务具体实现，后续若有WebSocket、Asynq的服务端也可以放这里
        - service/ 接口实现层，无任何业务逻辑，不能被 biz 层 import
          - service.go 除了ProviderSet，一般不能有别的方法
          - xxx.go 某业务的接口适配
          - 定义 XxxService 结构体来嵌入 xxxv1.UnimplementedXxxServiceServer 进行接口实现，嵌入 *biz.UserUsecase 来将规范化后的请求让 biz 层处理
        - biz/ 业务逻辑层，具体的业务处理逻辑，可以被 service 层 import，不应 import data 层(不关心存储如何实现)
          - biz.go 除了ProviderSet，一般不能有别的方法
          - xxx.go 某业务的具体处理逻辑
          - 定义 XxxUsecase 结构体表示本领域的具体业务
          - 定义 XxxRepo 接口表示本业务所需要的 data 层方法
        - data/ 数据访问层，包含数据库访问逻辑，可以 import biz 层
          - data.go 除了ProviderSet，有且仅有 NewData 相关初始化逻辑
          - xxx.go 实现 biz 层定义的 XxxRepo 接口，实现具体的数据访问逻辑
          - ent/schema/ 如用了 Ent ORM ，推荐这里定义表结构
          - generate.go 如用了 Ent  ORM 框架，生成代码的入口
      - go.mod|go.sum
      - justfile 服务级 Just 命令入口
    - web/ 前端
- just/ 平台共享 Just settings、registry 与 service 实现
- manifests/ 部署资源文件
- justfile 项目级 Just 命令入口
- pnpm-workspace.yaml 统一纳管 `api/gen` 与 `app/*/web`
- pnpm-lock.yaml Platform workspace 共享依赖锁文件
- buf.yaml buf 总配置，依赖以及 lint 规则
- `buf.go.gen.yaml` 项目级统一 Go 生成配置
- `buf.typescript.gen.yaml` 项目级统一 TypeScript HTTP、error reason 与 CRUD helper 生成配置
- `buf.es.gen.yaml` 已停用并全部注释，仅保留作 Protobuf-ES 配置参考
- go.work 统一管理各个微服务与 ./api/gen 的依赖
- go.mod|go.sum 总依赖管理


- 修改 OpenFGA model 后执行 `just openfga-model-apply`

## 常用命令

```bash
# 初始化
just init              # 安装 protoc 插件、CLI 与 pnpm workspace 依赖

# 代码生成
just gen               # 统一生成 Go、共享 TypeScript HTTP、OpenAPI、Wire 与 Ent
just api               # 生成 Go 与共享 TypeScript HTTP API
just api-go            # 仅生成全部模块的 Proto Go 代码到 api/gen/go
just api-ts            # 仅生成全部模块的共享 HTTP client 到 api/gen/ts 并构建包
just api-ts-check      # 仅检查共享 TypeScript HTTP 包类型
just wire              # 仅生成 Wire

# 质量检查
just lint              # Go、Proto、共享 TypeScript 与 Web 只读 lint
just lint-proto        # Proto lint

# OpenFGA
just openfga-init             # 初始化 store
just openfga-model-validate   # 验证 model
just openfga-model-test       # 测试 model
just openfga-model-apply      # 应用 model 更新
```

## 维护提示

- 仓库根 `buf.typescript.gen.yaml` 不声明 `inputs`：`buf generate` 默认读取当前 `buf.yaml` workspace 的全部模块，三个本地插件共同输出到 `api/gen/ts/`；服务级模板拥有独立 `out`，不会相互清理。
- `api/gen/package.json` 用 `./*` 映射 package 目录的 `index`，并用更具体的 `./*.errors`、`./*.crud` 映射 sidecar；新增 workspace 模块或 API 无需逐项维护 exports。共享包仅保留生成代码实际需要的 `@servora/proto-utils` 运行时依赖。
- pnpm 依赖与锁文件统一由仓库根 workspace 管理；`api/gen` 与 `app/*/web` 不维护独立 `pnpm-lock.yaml`
