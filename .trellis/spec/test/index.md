# Test 应用规范

适用于 `app/test/web/`。该应用只用于构建验证 workspace 生成 API；它不是 OIDC 业务、业务 UI 或服务端请求入口。本目录只有前端规范，不提供后端规范。

## 开发前检查

- 新增内容仍须服务于生成 API 或 workspace 构建验证。业务交互应进入对应应用，而不是在这里建立临时产品功能。
- 阅读 [前端规范](frontend.md)，保持 `@plateau/api` 作为生成代码来源，不复制类型、resource name 或 error reason。

## 导航

| 范围 | 规范 | 内容 |
| --- | --- | --- |
| 前端 | [前端规范](frontend.md) | 当前页面、依赖、路由与构建验证范围 |

## 质量检查

- 构建、lint 与未定义 typecheck/test 脚本的边界见 [前端规范](frontend.md)。
