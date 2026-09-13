# OpenFGA 配置与官方 SDK

本包只将 Plateau 配置映射到官方 SDK client，不封装业务授权数据面。具体主体、资源、检查结果归 [AuthZ](../security/authZ.md)。

[New](../../../../infra/openfga/client.go) 接收生成的 OpenFGA 配置并执行 ApplyConf，将 api_url、store_id、model_id 传给 SDK。model_id 可留空，不能据此宣称已固定模型版本。构造不执行 model apply 或授权请求。

配置 api_token 时要求合法 HTTPS URL、有效 hostname 且不含 userinfo；克隆 HTTP client 并禁用重定向，避免凭据随重定向泄漏。不修改全局默认 HTTP client，也不为了开发便利放宽带 token 的明文 URL。

配置来源是 [config.proto](../../../../api/protos/plateau/infra/openfga/v1/config.proto)；模型文件和脚本在 manifests，入口见 [development](../project/development.md)。模型修改先 validate/test，再在明确环境执行 apply；构造 SDK 成功不代表服务可访问或授权模型已经部署。

## 模型、关系与部署

[fga.mod](../../../../manifests/openfga/fga.mod) 使用 schema 1.2 并组合 IAM 与 Admin 模块。当前 [IAM 模型](../../../../manifests/openfga/iam.fga) 的 `iam.manage_users` 接受 service；[Admin 模型](../../../../manifests/openfga/admin.fga) 的 user admin 关系派生 `manage_users`。两者的关系主体与受保护对象不同，不能把管理员用户身份直接当作 IAM 服务身份。

关系 tuple 由拥有相应业务关系和管理流程的应用负责写入／撤销，SDK 构造函数不自动同步身份或授权关系。测试 YAML 中的 tuples 是模型测试夹具，不能当作运行环境已写入的证据。

`just openfga-model-validate` 与 `just openfga-model-test` 检查模型和声明的断言；`just openfga-model-apply` 在这些检查之后调用 [脚本](../../../../manifests/scripts/openfga/openfga.sh) 写模型，并更新所选 env 文件的 `FGA_MODEL_ID`。它有外部写入和本地配置写入副作用；init 还可创建 store。模型内容、实际 model ID、tuple 状态和接收服务 PEP 的装配应分别核验，不把 apply 成功等同于业务权限闭环。本任务未执行这些模型命令。

检查入口：`go test ./infra/openfga`。依据：[client tests](../../../../infra/openfga/client_test.go)，覆盖无构造网络、参数映射、token/redirect 限制和空 model。业务端不要绕开 SubjectMapper 在中间件任意拼接身份。
