# AGENTS.md - servora-platform

<!-- Updated: 2026-07-21 -->

## 项目概览

`servora-platform` 是 Servora 的平台服务与参考应用仓库，当前包含 Audit 微服务和单一 User CRUD 参考应用。

## 依赖关系

- go.work 用于连接代码生成与各个微服务依赖
- Proto BSR 依赖：`buf.build/servora/servora`

## 开发规范

- 修改 proto 后执行 `make gen`
- 修改 Wire 依赖图后执行 `make wire`
- 不要手改 `api/gen/go/*`、`app/*/web/src/api/generated/*`、`wire_gen.go`
- `servora-platform/app/example` 是当前servora最标准的微服务使用流程，也是新增平台级微服务的推荐模板。微服务的后端分为：
- api
  - gen/go 所有微服务的 Go proto 代码生成输出目录；前端 TypeScript 生成到各自 `web/src/api/generated/`
- app/ 微服务全都在这个目录里面
  - {ServiceName/}|{OtherServiceName/}
    - service/ 后端
      - api/ 本微服务proto接口定义和私有配置文件proto定义
        - protos/{DomainName}/service/ 该微服务各个领域的 Proto API 定义
        - protos/{ConfigName}/conf.proto 该微服务自己的业务配置 proto
        - buf.openapi.gen.yaml 该服务的OpenAPI的 buf 生成配置
        - buf.xxx.gen.yaml 该服务特有的 某语言的 buf 生成配置
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
      - Makefile 服务级 make 命令入口
    - web/ 前端
- make/ Makefile
- manifests/ 部署资源文件
- Makefile 项目级 make 命令入口
- buf.yaml buf 总配置，依赖以及 lint 规则
- `buf.go.gen.yaml` 项目级统一 Go 生成配置；服务特有语言配置放在服务 `api/` 下
- go.work 统一管理各个微服务与 ./api/gen 的依赖
- go.mod|go.sum 总依赖管理


- 修改 OpenFGA model 后执行 `make openfga.model.apply`

## 常用命令

```bash
# 初始化
make init              # 安装工具（protoc 插件 + CLI）

# 代码生成
make gen               # 统一生成（api + wire）
make api               # 生成统一 Go API 与各服务可选 TypeScript API
make api-go            # 仅生成 proto Go 代码
make wire              # 仅生成 Wire

# 质量检查
make lint              # Go lint
make lint.proto        # Proto lint

# OpenFGA
make openfga.init             # 初始化 store
make openfga.model.validate   # 验证 model
make openfga.model.test       # 测试 model
make openfga.model.apply      # 应用 model 更新
```

## 维护提示
