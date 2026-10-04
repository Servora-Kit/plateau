# 服务层边界

适用于 `app/example/service`，分层遵循 [共享微服务规范](../service/backend/index.md)，资源契约见 [资源规范](user-crud.md)。

- 路径参数仅定位资源，不能作为已认证身份或授权依据。
- 生产服务从已认证 context 读取操作者，并独立执行资源授权；不得复制参考实现中的身份简化。
