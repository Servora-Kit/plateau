# 表单与交互反馈

- 提交只调用 store action，由 store 统一维护忙碌、成功与错误状态；忙碌期间禁用冲突操作和关键查询控件。
- 状态使用 `<output aria-live="polite" aria-atomic="true">`，错误使用 `role="alert"`；列表区分加载、失败、空结果与可继续分页。
- 编辑初值随选中资源同步；一次性敏感输入成功后清除，破坏性操作明确说明后果与结果。
- 控件保留关联 label、适合的 `autocomplete` 和 `required`；不能以按钮禁用替代完整的状态与错误处理。
