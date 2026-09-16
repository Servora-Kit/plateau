# Admin 应用规范

Admin 面向 Plateau 平台管理与运维人员。当前 `app/admin/web` 已有独立 Vben workspace，Ant Design Vue 应用位于 `apps/web-antd`；Admin 后端尚未建立，Web 仍使用 Vben 演示登录。

| 入口 | 职责 |
| --- | --- |
| [架构](architecture.md) | 定位、范围、领域所有权和前后端共同接入关系 |
| [前端](frontend.md) | Vben 与 Ant Design Vue 选型、前端职责 |
| [后端](backend.md) | IAM 用户管理接入与两层授权边界 |

开发前先读架构，再按涉及的端读取相应规范与共享规范。现有 Web 命令见前端规范；后端建立时再登记对应测试和联调命令，不把 Vben 演示页面或已有 IAM 接口当作 Admin 已验收。
