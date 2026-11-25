# OpenFGA 配置与官方 SDK

本包只将 Plateau 配置映射到官方 SDK client，不封装业务授权数据面。具体主体、资源、检查结果归 [AuthZ](../security/authZ.md)。

[New](../../../../infra/openfga/client.go) 接收生成的 OpenFGA 配置并执行 Apply，将 api_url、store_id、model_id 传给 SDK。model_id 可留空，不能据此宣称已固定模型版本。构造不执行 model apply 或授权请求。

配置 api_token 时要求合法 HTTPS URL、有效 hostname 且不含 userinfo；克隆 HTTP client 并禁用重定向，避免凭据随重定向泄漏。不修改全局默认 HTTP client，也不为了开发便利放宽带 token 的明文 URL。

配置源：[config.proto](../../../../api/protos/plateau/infra/openfga/v1/config.proto)。模型文件和脚本位于 manifests，命令入口见 [development](../project/development.md)。构造 SDK 成功不代表服务可访问或模型已部署。

## 模型、关系与部署

模型按应用权限边界组织；关系主体与受保护对象必须匹配，不能把人类身份与服务身份混用。

关系元组按职责分层维护，SDK 构造函数不自动同步身份或授权关系，模型测试夹具也不代表运行环境关系已写入：
- **Store 与模型层**（`init` / `apply`）：模型修改先 validate/test，再向明确环境 apply。`apply` 写入远端模型并更新目标 env 文件的 `FGA_MODEL_ID`，`init` 还可创建 store；二者均不自动写入任何关系元组，确保测试临时 Store 零预设元组隔离。
- **平台静态服务元组层**（`sync-tuples`）：不含动态 UUID 与个人信息的跨服务调用关系集中声明于 `manifests/openfga/tuples/platform-services.yaml`，通过 `just openfga-tuple-sync`（要求显式固定 `FGA_STORE_ID` 与 `FGA_MODEL_ID`，`--on-duplicate ignore`）幂等写入；本地开发可通过 `just openfga-setup` 一键串联 `openfga-init` 与 `openfga-tuple-sync`。
- **动态人类与业务元组层**：由拥有对应业务流程的应用（如 `admin.service` 启动期 `SeedInitializer` 及管理 API）写入与撤销。模型、model ID、元组状态与接收服务 PEP 的装配须分别核验，apply 成功不等同于权限闭环。

检查无构造网络、参数映射、token/redirect 限制和空 model。入口：`go test ./infra/openfga`。应用不要绕开 SubjectMapper 在中间件任意拼接身份。
