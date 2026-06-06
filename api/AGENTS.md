# AGENTS.md - api/

<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-03-15 | Updated: 2026-06-06 -->

## 目录职责

`api/` 承载三类内容：
- 仓库级生成产物：`api/gen/`（Go 在 `go/`，TypeScript 在 `ts/`）
- Go 生成模块：`api/gen/go.mod`，模块路径为 `github.com/Servora-Kit/servora-platform/api/gen`
- TypeScript API client 包：`api/gen/package.json`，包名为 `@servora-platform/api-client`

仓库使用 **Buf v2 workspace**。当前根目录 `buf.yaml` 纳管 `app/audit/service/api/protos`；后续新增服务 proto 时继续加入根 `buf.yaml`，统一从仓库根生成 Go 与 TypeScript 产物。

## 当前结构

```text
api/
├── AGENTS.md
└── gen/
    ├── go.mod        # Go 生成代码模块根
    ├── go.sum
    ├── package.json  # TypeScript API client 包根
    ├── go/           # Go 生成代码（make api-go 输出，可清空）
    │   └── audit/service/...
    └── ts/           # TypeScript 生成代码（make api-ts 输出，可清空）
        └── audit/service/...
```

## 生成规则

| 命令 | 模板 | 输出 | clean |
|------|------|------|-------|
| `make api` | `buf.go.gen.yaml` + `buf.typescript.gen.yaml` | `api/gen/go/` + `api/gen/ts/` | true |
| `make api-go` | `buf.go.gen.yaml`（含 authz + mapper + audit + conf 插件） | `api/gen/go/` | true |
| `make api-ts` | `buf.typescript.gen.yaml` | `api/gen/ts/` | true |
| `make openapi` | 各服务 `api/buf.openapi.gen.yaml` | 各服务目录 | — |

`api/gen` 是双包根：Go module root 与 npm package root 共用同一个目录。Buf 插件输出必须写到 `api/gen/go` 或 `api/gen/ts`，不要写到 `api/gen` 根目录。这样 `clean: true` 只会清理生成子目录，不会删除 `api/gen/go.mod`、`api/gen/go.sum`、`api/gen/package.json` 等包根文件。

## 关键文件

- `../buf.yaml`：Buf v2 workspace 配置
- `../buf.go.gen.yaml`：Go 代码生成模板
- `../buf.typescript.gen.yaml`：TypeScript client 生成模板
- `gen/go.mod`：Go 生成代码模块定义
- `gen/package.json`：TypeScript API client 包定义

## 开发约定

- 服务专属业务 proto 放在对应服务的 `app/{service}/service/api/protos/`
- 修改 proto 后在仓库根目录运行 `make api`，同时刷新 Go 与 TypeScript 生成产物
- **禁止手动编辑** `api/gen/go/` 和 `api/gen/ts/`
- 可以维护 `api/gen/go.mod`、`api/gen/go.sum`、`api/gen/package.json` 等包根文件
- Go 插件使用 `paths=source_relative`，TypeScript 的 `protoc-gen-typescript-http` 不配置 `paths=source_relative`
- 后续 `protoc-gen-servora-crud target=ts` 也输出到 `api/gen/ts`，生成业务 `ResourceSchema`

## 常用命令

```bash
make api          # 生成 Go 与 TypeScript API 代码
make api-go       # 仅生成 Go API 代码
make api-ts       # 仅生成 TypeScript API client
make openapi      # 生成各服务 OpenAPI 文档
buf lint
buf format -w
buf breaking --against '.git#branch=main'
```
