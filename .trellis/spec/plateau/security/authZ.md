# AuthZ：OpenFGA 授权语义

适用于平台具体授权能力与路由适配。连接配置归 [infra/openfga](../infra/openfga.md)，模型和业务 subject 映射由业务拥有；本层不兼任凭据验证或身份数据库。

## 具体能力

[Authorizer](../../../../security/authz/openfga/authz.go) 接收官方 SDK client 与应用 `SubjectMapper`，将有效、非匿名 Actor 映射为直接 subject，再组装 relation 和 object。

- `Check` 返回是否允许；拒绝与 provider 故障分开表达。
- `BatchCheck` 用输入序号作为 correlation ID，按原输入顺序返回。数量、ID、allowed 或单项错误异常使整个调用失败，不能把缺结果默认为拒绝或允许。
- `ListAllowed` 调用 ListObjects，要求返回 object 前缀精确匹配资源类型，剥离后返回裸 ID；不把跨类型对象混入结果。
- mapper 输出必须是合法的直接 subject，禁止 wildcard/userset 代替执行主体。调用参数须满足当前 relation/type/object 校验，不用静默 trim 掩盖错误。

## 路由中间件

`authz.WithRulesFuncs` 合并生成规则，后注册 provider 覆盖同名 operation。必须显式配置规则：
- `NONE` 跳过授权，不代表已经认证，也不为请求授予身份。
- `REQUIRED` 读取 AuthN 写入的可信 Actor，使用静态 resource_id 或从 Proto 请求的 resource_id_field 解析裸 ID，再检查。
- 缺 transport、缺规则、未知 mode 都失败；缺身份为 Unauthenticated，明确拒绝为 PermissionDenied，参数错误为 InvalidArgument，已分类依赖故障为 Unavailable。

字段路径按 Proto 字段名遍历，不按 JSON 别名；中间节点必须是已存在的单值 message，不能走 repeated/map/真实 oneof；终点只接受支持的字符串或整数类型。optional 字段缺失与有效零值要按 presence 区分，不能直接字符串化任意字段。

## 应用职责

接收请求的服务入口／中间件承担 PEP（执行权限检查），OpenFGA 承担 PDP（计算允许或拒绝）；拥有权限管理流程的应用承担 PAP（维护模型或关系）。IAM 的身份认证、服务 token 与 subject 映射不能替代接收业务服务的资源权限检查。业务代码可以发起并执行检查，不因此拥有策略计算引擎；模型与 tuple 的归属见 [模型运行边界](../infra/openfga.md)，IAM 的具体应用职责见 [IAM 授权规范](../../iam-service/authorization.md)。

## 检查

重点覆盖授权前拒绝、静态/动态目标、嵌套 oneof、批量响应完整性、取消和 provider 错误分类。入口：`go test ./security/authz/...`。跨层改动同时核对 [annotations](../../api/proto/annotations.md) 与 [生成器](../codegen/cmd.md)。

依据：[middleware](../../../../security/authz/openfga/middleware.go)、[能力测试](../../../../security/authz/openfga/authz_test.go)、[路由测试](../../../../security/authz/openfga/middleware_test.go)、[规则聚合](../../../../security/authz/rules.go)。
