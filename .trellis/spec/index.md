# Plateau 开发规范索引

按职责组织开发规范，任务显式组合相关正文。规范保持精简，只保留可复用约束；不记录业务接口、字段、调用实例或验收历史，修订已有规则优先于增量追加。

## 规范归属与文件结构

每个应用使用一个目录 `.trellis/spec/<应用>/`，与 `app/<应用>` 对应，不再按前后端拆成两个 package。

| 文件 | 职责 |
| --- | --- |
| `index.md` | 范围、导航与检查入口 |
| `architecture.md` | 应用定位、范围、领域所有权、前后端共同契约与设计取舍 |
| `frontend.md` | 前端组织、交互、请求消费和前端检查入口 |
| `backend.md` | 后端分层、接口实现、数据与后端检查入口 |
| 具体专题 `.md` | 已有独立职责的规范，如身份、会话与 OIDC |

按实际职责建立文件，不建空白占位；架构规范不重复两端实现，通用规则不复制到应用目录。

共享包继续按实际职责分组，例如 `service/backend`、`plateau/security`、`web/client`。索引与上下文清单按实际工作显式引用入口或专题，不能假定读一个索引就自动加载全部规则。

## Package 导航

| Package | 入口与职责 |
| --- | --- |
| plateau | [project](plateau/project/index.md)、[security](plateau/security/index.md)、[infra](plateau/infra/index.md)、[codegen](plateau/codegen/index.md) |
| api | [Proto 契约与生成](api/proto/index.md) |
| web | [共享 client](web/client/index.md) |
| service | [共通微服务](service/backend/index.md) |
| servora | [framework](servora/framework/index.md)、[proto](servora/proto/index.md)、[cmd](servora/cmd/index.md)、[web](servora/web/index.md) |
| 应用 | 按 `app/<应用>` 选择同名规范目录，以 `<应用>/index.md` 为入口 |

[通用思考指南](guides/index.md) 提供跨层检查与复用思路；具体规则以所属主题为准，新增职责时同步导航，不复制默认模板。
