# 公共 Proto 注解

框架 Proto 位于 `api/protos/servora/<namespace>/v1/`，`package` 必须是带版本后缀的 `servora.<domain>.v1`，目录与 package 对齐，`go_package` 指向 `api/gen/go/servora/<namespace>/v1`。来源为 [`api/protos/AGENTS.md`](../../../../../servora/api/protos/AGENTS.md) 和 [`api/AGENTS.md`](../../../../../servora/api/AGENTS.md)。annotation 号段按根约定从 `5xx00` 规划，方法/消息用 `+0`，服务/字段用 `+1`。

现有 `servora.audit.v1.rule` 和 `service_default` 分别扩展 MethodOptions/ServiceOptions；方法有显式有效值时覆盖服务默认，否则继承，定义在 [`audit/v1/annotations.proto`](../../../../../servora/api/protos/servora/audit/v1/annotations.proto)。生成器和运行时必须保留这个合并语义。

`servora.conf.v1.section`/`field` 表达配置 section、optional、default、required；`default` 与 `required` 互斥，定义在 [`conf/v1/annotations.proto`](../../../../../servora/api/protos/servora/conf/v1/annotations.proto)。`servora.errors.v1` 只为错误 enum 给出默认和逐值 HTTP code，见 [`errors/v1/errors.proto`](../../../../../servora/api/protos/servora/errors/v1/errors.proto)；业务 reason 仍由业务 Proto 定义。CRUD 框架 reason 只涵盖框架输入/不变量，见 [`crud/v1/errors.proto`](../../../../../servora/api/protos/servora/crud/v1/errors.proto)。

配置 schema 归其能力 owner：TLS、Redis、transport、obs 等在各自 namespace 定义，`core/bootstrap` 只扫描与装配，不拥有业务或 provider 配置。`protoc-gen-servora-conf` 按实际注解及可达的子字段，为 message 生成所需的 `SectionKey`、可选性、`ApplyDefaults`、`CheckRequired` 与 `ApplyConf`；`ApplyConf` 先检查 required，再应用 defaults。生成的 defaults 对 nil receiver 无操作，只为传递性含默认字段的普通 singular message 子树 allocate 后递归；无默认字段的子树保留 nil，oneof 仅在当前成员已设置时递归，不会为了默认值选择 oneof。生成细节见 [`protoc-gen-servora-conf/main.go`](../../../../../servora/cmd/protoc-gen-servora-conf/main.go) 和 [`bootstrap.pb.servora-conf.go`](../../../../../servora/api/gen/go/servora/core/v1/bootstrap.pb.servora-conf.go)。这不放宽运行时 `Scan` 对 nil target 的拒绝规则，见 [Bootstrap](../framework/bootstrap.md)。

先定义 owner、版本、兼容性和生成消费方，再加入 annotation；不要把业务 API、存储错误或 UI 文案写入框架公共 Proto。
