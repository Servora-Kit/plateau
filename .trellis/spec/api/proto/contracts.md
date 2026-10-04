# 平台 API 契约

适用于平台源 Proto 和各业务服务 API。gRPC 与 HTTP 共用同一个领域 service，HTTP annotation 与 RPC 同处一份源定义，不维护两套独立 DTO 合同。

## 源码与资源

平台共享 Proto 在 `api/protos/plateau/<domain>/<version>`，业务 Proto 在 `app/<应用>/service/api/protos`。go_package 指向共享生成模块对应路径，生成布局见 [generation](generation.md)。

资源名由 Proto resource pattern 定义，不能与数据库 ID 或 URL 编码片段混用；租户与父子资源结构由领域决定，不推广业务实例。

## 查询与字段语义

- page_token 是不透明续页 token，不由前端拆解；filter/order_by 只承诺实现支持的确定性子集，不宣称完整查询语言。
- 列表扩展须明确声明；未计算的统计值与零值区分，删除可见性与分页语义一致。
- 可选字段及其设置状态、INPUT_ONLY、OUTPUT_ONLY、IMMUTABLE 与字段更新共同组成契约。update_mask 选择字段，省略与显式清除不可混淆。
- 并发控制、软删与恢复由业务实现，不由注解自动保证。
- 生成描述和字段常量、ResourcePlan、数据映射须保持同一语义；使用流程见 [service CRUD](../../service/backend/crud.md)，框架内部见 [framework CRUD](../../servora/framework/crud.md)。

## 错误与兼容性

领域失败使用所属 Proto error reason；共享安全错误由 [SecurityErrorReason](../../../../api/protos/plateau/security/errors/v1/errors.proto) 声明。不要将网络失败、认证失败和业务冲突压成单一成功响应字段。

`buf.yaml` 当前配置 STANDARD lint 和 FILE breaking，并对标准 CRUD 响应命名及复用请求类型作例外。调整 Proto 应保留字段号与已有语义；删除或破坏兼容性需显式评估消费者、生成 diff 和迁移方式，不能靠改生成类型掩盖源契约变化。breaking 比较须使用任务确定的真实基线，不凭空选远端 ref。

检查入口：`just lint-proto`、`just api-ts-check`，生成变化还需受影响 Go 服务／前端检查。配置中有 breaking 规则不等于本次已执行 breaking 比较。

入口：[buf.yaml](../../../../buf.yaml)、[生成流程](generation.md)。
