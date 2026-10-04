# OpenFGA 配置与官方 SDK

本包只将 Plateau 配置映射到官方 SDK client，不封装业务授权数据面。具体主体、资源、检查结果归 [AuthZ](../security/authZ.md)。

[New](../../../../infra/openfga/client.go) 接收生成的 OpenFGA 配置并执行 Apply，将 api_url、store_id、model_id 传给 SDK。model_id 可留空，不能据此宣称已固定模型版本。构造不执行 model apply 或授权请求。

配置 api_token 时要求合法 HTTPS URL、有效 hostname 且不含 userinfo；克隆 HTTP client 并禁用重定向，避免凭据随重定向泄漏。不修改全局默认 HTTP client，也不为了开发便利放宽带 token 的明文 URL。

配置源：[config.proto](../../../../api/protos/plateau/infra/openfga/v1/config.proto)。模型文件和脚本位于 manifests，命令入口见 [development](../project/development.md)。构造 SDK 成功不代表服务可访问或模型已部署。

## 模型、关系与部署

模型按应用权限边界组织；关系主体与受保护对象必须匹配，不能把人类身份与服务身份混用。

关系 tuple 由拥有相应业务关系和管理流程的应用写入／撤销，SDK 构造函数不自动同步身份或授权关系。模型测试夹具不代表运行环境关系已写入。

模型修改先 validate/test，再向明确环境 apply。apply 写入远端模型并更新所选 env 文件的 `FGA_MODEL_ID`，init 还可创建 store；这些操作有外部及本地配置写入副作用。模型、model ID、tuple 状态与接收服务 PEP 的装配应分别核验，apply 成功不等同于权限闭环。

检查无构造网络、参数映射、token/redirect 限制和空 model。入口：`go test ./infra/openfga`。应用不要绕开 SubjectMapper 在中间件任意拼接身份。
