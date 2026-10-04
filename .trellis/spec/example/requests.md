# 请求适配与生成契约

- 从应用生成目录导入 client、资源名、字段表、错误 enum 与 guard，不手写平行契约；filter、order、分页和更新 mask 使用生成 helper 与 `@servora/proto-utils/crud`。
- transport 统一确定请求 origin，设置 `Accept: application/json`，仅在 body 非空时设置 JSON Content-Type；直接传递已序列化的 ProtoJSON，不再次转换。
- 超时与网络错误分别归类，非成功 HTTP 响应保留 status、响应体及调用元数据；未实现的流式调用明确拒绝，不伪装为普通请求。
- 错误呈现先区分网络与超时，再解析 Kratos reason；已知 reason 映射为应用文案，不将机器 reason 作为兜底文案。
- 更新携带当前 `etag` 与生成 mask；响应资源名按生成规则核验，分页状态由 helper 推进。
