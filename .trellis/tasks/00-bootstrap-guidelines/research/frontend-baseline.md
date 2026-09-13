# 前端规范基线

调查日期：2026-09-13。Plateau 提交基线为 `d1fa919`。本记录只描述已读取的源码、配置与测试；没有启动 Web、后端、中间件或浏览器，因此运行时能力均未在本轮验收。

## 共享 client

[`web/packages/client`](../../../../web/packages/client) 导出 `createHttpClient`、`createTransport`、`createServerStream` 和 `createDuplexStream`。HTTP 实现支持取消、超时和对 GET 的受控重试；其 Node 测试覆盖读请求重试、写请求不重试、响应体超时、取消与浏览器相对路径解析，见 [`test/http.test.ts`](../../../../web/packages/client/test/http.test.ts)。SSE 与 WebSocket 实现存在于源码，但当前搜索未发现业务应用对其流接口的消费或端到端测试。

历史 OpenSpec finding 10 的前端部分已核对：HTTP 的条件 Content-Type、取消、超时和重试，SSE 的失败分级与重试，WebSocket 的关闭、错误和 1 MiB 有界发送缓冲均已有当前规范与源码依据。补充确认 [`createTransport`](../../../../web/packages/client/src/index.ts) 对生成 unary 调用固定使用 `application/json`；当前 client 未建立 `application/protojson` 兼容承诺。服务端 ProtoJSON/普通 JSON 分流、原生 middleware 顺序和 CORS 认证边界属于 Servora transport，不在本范围重复写入。IAM smoke、真实同源 cookie、CAP 和 OIDC 验收仍未执行。

## 应用消费差异

| 应用 | 当前请求实现 | 已确认静态证据 | 未在本轮验收 |
| --- | --- | --- | --- |
| IAM Web | 使用 `@plateau/client` 的 HTTP transport | [`lib/iam-api.ts`](../../../../app/iam/web/lib/iam-api.ts)、[`next.config.ts`](../../../../app/iam/web/next.config.ts) | 同源 cookie、CAP、邮件/OIDC 回调、真实请求 |
| Example Web | 应用自有原生 `fetch` `ClientTransport` | [`src/api/transport.ts`](../../../../app/example/web/src/api/transport.ts)、[`transport.test.ts`](../../../../app/example/web/src/api/transport.test.ts) | Vite 代理到 Example service 的完整 CRUD |
| Test Web | 无请求实现，只渲染生成 API 标识 | [`app/page.tsx`](../../../../app/test/web/app/page.tsx)、[`package.json`](../../../../app/test/web/package.json) | 任意服务可达性或 API 行为 |

## 前端结构与检查入口

- IAM：Next App Router，CSR 页面、组件、hook 和 `lib` 分层；`useSensitiveValue` 只提供 `clear()` 与卸载时清空 ref，登录、注册、重置密码和账户安全页在 `await run(...)` 后显式调用各密码实例的 `clear()`；`just web::iam::typecheck`、`just web::iam::lint`、`just web::iam::build` 是已有入口。未找到前端测试文件。
- Example：Vue 3 + Vue Router + Pinia，生成 API、应用 adapter、store 与视图职责清晰；`just web::example::lint`、`just web::example::test-unit` 与构建入口存在。测试覆盖 transport 和 error reason 文案，不覆盖浏览器 CRUD。
- Test：Next 单页构建验证，已有 build 与 lint 命令，无 typecheck/test script。

## 迁移处理

四个目标 spec 目录原有内容均为未填写的初始化模板，含 `To be filled`、占位注释和英文通用说明，没有项目特有规则或有效用户内容。本轮按任务设计将其替换为 `web/client`、`iam-web`、`example-web`、`test-web` 的真实主题与索引；未修改业务源码、生成物、AGENTS、配置或任务状态。
