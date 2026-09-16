# Example 表单、加载与错误状态

`HomeView` 通过 Pinia 的 `busy` 防止并发操作，所有会改变数据的按钮和关键查询控件在忙碌时禁用。提交函数只调用 store action；store 的 `execute` 负责在开始、成功和失败时更新 `status`、`error` 与 `busy`，因此新增请求不得在视图和 store 中各自维护一套冲突的加载状态。

## 反馈规则

- 状态区域使用 `<output aria-live="polite" aria-atomic="true">`，错误使用 `role="alert"`，证据见 [`HomeView.vue`](../../../app/example/web/src/views/HomeView.vue)。
- 列表明确显示 loading、error、空结果和可继续分页的状态；不要用空表格同时表示加载失败或没有数据。
- 创建成功后仅清空一次性临时密码并更新默认 ID/email；编辑值随着选中资源变化同步。删除与恢复是明确按钮，并向用户解释 tombstone 语义。
- 表单控件保留连接的 label、适合的 `autocomplete` 和 `required`；破坏性操作保持显式文字和可理解的结果状态。

现有单元测试覆盖 transport 和错误 reason 映射，不覆盖完整 DOM 表单交互。涉及真实 CRUD 交互时仍需按 [前端规范](frontend.md) 中的浏览器步骤验收。
