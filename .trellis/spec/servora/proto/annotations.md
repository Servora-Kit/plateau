# 公共 Proto 注解

框架 Proto 位于 `api/protos/servora/<namespace>/v1/`，package 为 `servora.<domain>.v1`，目录与版本对齐，`go_package` 指向 `api/gen/go/servora/<namespace>/v1`。annotation 号段从 `5xx00` 规划，方法/消息用 `+0`，服务/字段用 `+1`。

`servora.audit.v1.rule` 与 `service_default` 扩展 MethodOptions/ServiceOptions；方法有显式有效值时覆盖服务默认，否则继承。生成器与运行时须保持此语义。

`servora.conf.v1.section` 标记配置段，`field` 声明互斥的 default/required；`servora.errors.v1` 为错误枚举声明 HTTP 状态码，CRUD 错误只涵盖框架输入和不变量。

配置定义放在所属模块，`core/bootstrap` 只加载与装配。默认值、参数转换和资源检查各有明确负责位置，不为每个模块增加配置准备包装。CORS 的来源、方法和请求头默认列表由其中间件维护；其余已声明的固定字段默认由 Proto 维护。

先确定负责模块、接口版本、兼容性和真实消费者，再加入注解；不要把业务 API、存储错误或界面文案写入框架公共 Proto。

## 配置处理契约

- 配置段、声明字段规则的消息及其父子配置参与生成；普通业务消息不因仅有值约束而批量生成配置方法。
- 唯一接口为 `Apply() error`：成功时默认值补齐、配置检查通过；不读文件、创建连接或启动服务。bootstrap 自动调用，独立解码/构造入口须在使用前调用；有效对象重复 Apply 结果不变。
- section 标记按消息短名转小写下划线段名，不裁剪后缀；无标记时扫描整份配置。缺段跳过且不填默认值，实际消费模块仍须检查必需参数。
- 默认值只补未设置字段，保留明确的 0、false 和空字符串；普通父对象缺失时不创建，Duration 自身有默认值时可补值。
- 标量需要 optional 等原生 presence。required 只检查是否明确设置，非空、范围和集合大小由 buf.validate 单独声明。
- 递归处理已有单值子消息、列表、映射与互斥分支；跨目录生成不调用外部类型未生成的方法，无配置注解但含值规则的外部子树执行完整值校验。
- 局部值校验按对象身份过滤，不能仅按消息类型判断而重复校验同类型子节点。

| 输入 | 结果 |
| --- | --- |
| default 与 required 同时声明，或默认类型不支持 | 生成期报错，包含字段位置 |
| 普通非必填父块缺失 | 保持缺失，不处理子字段 |
| 必填字段缺失 | Apply 返回字段路径错误 |
| 明确空字符串且声明最小长度为 1 | 值校验失败 |
| 列表或映射中的消息值为 nil | 返回元素位置错误 |
| 多个映射值违反规则 | 错误顺序稳定，保留原值校验错误类型 |

检查跨目录生成、同类型递归、集合错误位置、显式零值、缺段和日志根配置，使用标准 Go 生成物。入口：`go test ./cmd/protoc-gen-servora-conf ./cmd/internal/protoreach ./core/bootstrap/...`。

