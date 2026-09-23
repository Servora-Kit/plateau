# 公共 Proto 注解

框架 Proto 位于 `api/protos/servora/<namespace>/v1/`，`package` 必须是带版本后缀的 `servora.<domain>.v1`，目录与 package 对齐，`go_package` 指向 `api/gen/go/servora/<namespace>/v1`。来源为 [`api/protos/AGENTS.md`](../../../../../servora/api/protos/AGENTS.md) 和 [`api/AGENTS.md`](../../../../../servora/api/AGENTS.md)。annotation 号段按根约定从 `5xx00` 规划，方法/消息用 `+0`，服务/字段用 `+1`。

现有 `servora.audit.v1.rule` 和 `service_default` 分别扩展 MethodOptions/ServiceOptions；方法有显式有效值时覆盖服务默认，否则继承，定义在 [`audit/v1/annotations.proto`](../../../../../servora/api/protos/servora/audit/v1/annotations.proto)。生成器和运行时必须保留这个合并语义。

`servora.conf.v1.section` 是布尔配置段标记；`field` 声明默认值或必填要求，两者互斥，定义在 [`conf/v1/annotations.proto`](../../../../../servora/api/protos/servora/conf/v1/annotations.proto)。`servora.errors.v1` 为错误枚举声明 HTTP 状态码；CRUD 框架错误只涵盖框架输入和不变量。

配置定义放在所属模块，`core/bootstrap` 只加载与装配。默认值、参数转换和资源检查各有明确负责位置，不为每个模块增加配置准备包装。CORS 的来源、方法和请求头默认列表由其中间件维护；其余已声明的固定字段默认由 Proto 维护。

先确定负责模块、接口版本、兼容性和真实消费者，再加入注解；不要把业务 API、存储错误或界面文案写入框架公共 Proto。

## 配置处理契约

### 适用范围

适用于标记为配置段的消息、声明配置字段规则的消息以及它们的父子配置。普通业务消息不会仅因使用值约束而批量生成配置方法。

### 接口

只生成 `Apply() error`；成功时默认值已补齐、配置检查已通过。bootstrap 自动处理交给它的配置；独立解码或构造入口在使用前调用同一方法。Apply 不读文件、不创建连接、不启动服务。

### 字段与加载规则

- `(servora.conf.v1.section) = true`：消息短名转小写下划线段名，例如 OpenFGA 对应 open_fga，不裁剪 Config/Contract 后缀；无标记时扫描整份配置。
- 缺段跳过且不填默认值；实际使用模块时，仍须检查该模块必需参数。
- 默认值只补未设置字段，保留明确的 0、false 和空字符串；普通父对象缺失时不创建。Duration 自身有默认值时可补值。
- 标量需要 optional 等原生设置标记。required 只检查是否明确设置；非空、范围和集合大小由 buf.validate 单独声明。
- 递归处理已有单值子消息、列表、映射和互斥分支。按目录分批生成时，不调用外部类型未生成的方法；无配置注解但含值规则的外部子树直接执行完整值校验。

### 校验与错误

| 输入 | 结果 |
| --- | --- |
| default 与 required 同时声明，或声明不支持的默认类型 | 生成期报错，包含字段位置 |
| 普通非必填父块缺失 | 保持缺失，不处理子字段 |
| 必填字段缺失 | Apply 返回字段路径错误 |
| 明确的空字符串且声明最小长度为 1 | 值校验失败 |
| 列表或映射中的消息值为 nil | 返回元素位置错误 |
| 多个映射值违反规则 | 错误顺序稳定，保留原值校验错误类型 |

### 正常、边界与错误示例

`optional int32 count` 默认值为 7：省略后得到 7，明确设置 0 后仍为 0。普通父块省略时仍为 nil，显式空对象才处理其内部规则。对同一有效对象重复 Apply，结果不变。

### 验证要求

执行 `go test ./cmd/protoc-gen-servora-conf ./cmd/internal/protoreach ./core/bootstrap/...`；使用真实标准 Go 生成物验证跨目录生成、同类型递归、集合错误位置、显式零值、缺段和日志根配置。局部值校验按对象身份过滤，不能仅按消息类型判断，否则同类型子节点仍会重复校验。

### 错误与正确写法

错误：用 `GetCount() == 0` 判断用户没有填写，并覆盖成默认值。

正确：使用字段的设置标记判断是否缺失；已设置的 0 保持原值，需要拒绝 0 时单独声明范围约束。
