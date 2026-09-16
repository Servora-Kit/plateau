# Plateau 开发规范索引

这里面向开发者与 AI，按职责组织当前项目的开发规范。任务需要显式组合相关正文；索引链接不会自动注入全部内容。

## 规范归属与文件结构

每个应用使用一个目录 `.trellis/spec/<应用>/`，与 `app/<应用>` 对应，不再按前后端拆成两个 package。

| 文件 | 职责 |
| --- | --- |
| `index.md` | 应用概况、规范导航、开发前与质量检查入口 |
| `architecture.md` | 应用定位、范围、领域所有权、前后端共同契约与设计取舍 |
| `frontend.md` | 前端组织、交互、请求消费和前端检查入口 |
| `backend.md` | 后端分层、接口实现、数据与后端检查入口 |
| 具体专题 `.md` | 已有独立含义的详细契约，由相应入口链接，如 IAM 的身份、会话和 OIDC |

按实际内容建立文件：没有前端或后端的应用不建空白占位；`architecture.md` 不重复两端实现细节。应用设计归应用目录，`plateau/project/boundaries.md` 只维护平台、框架与应用之间的职责边界。设计目标与当前实现分开标注。

共享包继续按实际职责分组，例如 `service/backend`、`plateau/security`、`web/client`。索引与上下文清单按实际工作显式引用入口或专题，不能假定读一个索引就自动加载全部规则。

## Package 导航

| Package | 入口与职责 |
| --- | --- |
| plateau | [project](plateau/project/index.md)、[security](plateau/security/index.md)、[infra](plateau/infra/index.md)、[codegen](plateau/codegen/index.md) |
| api | [Proto 契约与生成](api/proto/index.md) |
| web | [共享 client](web/client/index.md) |
| service | [共通微服务](service/backend/index.md) |
| servora | [framework](servora/framework/index.md)、[proto](servora/proto/index.md)、[cmd](servora/cmd/index.md)、[web](servora/web/index.md) |
| iam | [IAM](iam/index.md)：身份平台、用户自助前端与后端协议 |
| example | [Example](example/index.md)：参考应用前后端 |
| audit | [Audit](audit/index.md)：现有后端与维护限制 |
| test | [Test](test/index.md)：生成 API 构建验证前端 |
| admin | [Admin](admin/index.md)：平台管理应用设计 |

[通用思考指南](guides/index.md) 提供跨层检查与复用思路，具体规则以各 package 的权威主题为准。新功能按实际能力扩展，目录和 topic 不必复制默认模板；新增/调整规范时同步索引和真实任务上下文引用。
