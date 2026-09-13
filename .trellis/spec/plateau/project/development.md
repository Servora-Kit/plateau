# 本地开发与检查入口

本规范记录现有开发入口，不重新设计跨仓工作区。命令均从 Plateau 根目录运行；在采用 RTK 的环境中加 `rtk proxy` 前缀。

## 固定端口

从 10000 起，每个应用保留 10 个端口：+0 HTTP、+1 gRPC、+2 Web，+3～+9 预留。新增应用分配下一个空闲段，不重排旧编号。

| 应用 | HTTP | gRPC | Web |
| --- | --- | --- | --- |
| IAM | 10000 | 10001 | 10002 |
| Audit | 10010 | 10011 | 10012（预留） |
| CMS（占位） | 10020（预留） | 10021（预留） | 10022（预留） |
| Example | 10030 | 10031 | 10032 |
| Test | 10040（预留） | 10041（预留） | 10042 |
| Admin（占位） | 10050（预留） | 10051（预留） | 10052（预留） |

约定作用于本地监听和 Docker 宿主映射；容器内部、共享中间件和生产公开端口不受约束。相同应用的原生运行与容器映射不可同时占用同一端口，Web dev 与 preview/start 共用端口也不能同时启动。修改 IAM 公开入口须同步 `IAM_PUBLIC_ORIGIN`、Web 端口和后端代理地址。

## 命令及副作用

| 入口 | 用途 |
| --- | --- |
| `just init` | 安装 CLI／插件及冻结锁文件的 pnpm 依赖，会产生本地安装副作用 |
| `just gen` | 根 API 生成后执行各服务 OpenAPI、Wire、Ent；修改 Proto 后使用 |
| `just wire` | 各服务 Wire 装配刷新 |
| `just lint` | API TS typecheck、Buf lint、已注册服务 Go lint；不等于平台根全部 Go 测试 |
| `just api-ts-check` | 共享生成 TS 契约检查 |
| `just web::iam::typecheck`／`lint` | IAM 前端检查；dev/build/preview 使用同一注册入口 |
| `just openfga-model-validate`／`test` | 本地 model 与场景检查 |
| `just openfga-model-apply` | 修改 model 后的应用步骤，会写外部 model／环境配置，需在明确目标环境执行 |

根插件安装中 Plateau AuthN/AuthZ 必须来自当前 checkout。Servora 工具版本由当前 Just 变量配置；不能因为 go.work 引用源码就假定已安装的插件也已同步。生成路径与入口详见 [generation](../../api/proto/generation.md)。

开发或审查时先选择受影响模块和检查命令；记录实际执行与未执行项。基础设施 provider 的业务日志由 data/bootstrap 边界决定，不在构造函数中重复记录成功或失败。

来源：[根命令](../../../../justfile)、[服务注册](../../../../just/services.just)、[服务命令实现](../../../../just/service.just)、[根 AGENTS](../../../../AGENTS.md)。
