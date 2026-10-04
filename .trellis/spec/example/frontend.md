# 页面与状态职责

适用于 `app/example/web/`。请求与生成契约见 [请求](requests.md)，交互反馈见 [表单](forms.md)。

- 入口安装 router 与 Pinia，路由决定页面，视图负责交互，静态资源负责样式；不将请求实现混入视图。
- 生成 API 只消费，不手改或复制类型；应用自有 transport 负责请求适配，不因其他应用的实现而混用装配边界。
- store 管理跨控件共享的资源、查询和操作状态；视图调用 action，表单编辑值保留为局部状态，并随选中资源同步初值。
- 共享 client 的职责见 [Web client 架构](../web/client/architecture.md)，资源名与字段能力以 Proto 和生成 helper 为准。
