# Example 应用规范

适用于 `app/example/service` 与 `app/example/web/`。Example 是新增平台微服务 CRUD 分层和公开 HTTP 外观的参考应用，不是生产 IAM、租户认证或授权实现，也不是第二份 API 契约来源。

## 开发前检查

- 修改后端 User API、生命周期、列表或 Ent 映射时，读取 [后端规范](backend.md) 与 [User CRUD 实例](user-crud.md)。
- 修改前端目录、页面或 Pinia 状态时读取 [前端规范](frontend.md)；请求、生成代码消费或错误映射还须读取 [请求](requests.md)，表单、加载与可访问性改动还须读取 [表单](forms.md)。
- 资源名 `example.servora.dev/User` 与 `tenants/{tenant}/users/{user}` 必须以生成 helper 和 service Proto 为准，不手写平行契约。
- 不将 `UserScope` 的 path tenant 提取复制为生产认证方案；生产服务从已认证 operator context 获取身份，并独立执行授权。

## 导航

| 范围 | 规范 | 内容 |
| --- | --- | --- |
| 后端 | [后端规范](backend.md) | 分层参考、开发边界与后端检查入口 |
| 后端 | [User CRUD 实例](user-crud.md) | 当前 User 的 descriptor、数据语义和验证分工 |
| 前端 | [前端规范](frontend.md) | Vue、路由、Pinia、生成 API 的职责与前端检查入口 |
| 前端 | [请求](requests.md) | 应用自有 transport、ProtoJSON、CRUD helper 与错误分类 |
| 前端 | [表单](forms.md) | 提交、加载、错误与键盘可操作性 |

## 质量检查

- 后端测试入口与端到端验收边界见 [后端规范](backend.md)。
- 前端静态检查、构建和真实 CRUD 浏览器验收边界见 [前端规范](frontend.md)。
