# 规范目录迁移去向

旧规范在迁移前已保存逐文件副本。各组确认正文是未填写模板；guides 中通用的思考目的保留，重复的上游项目专有说明移除。

| 原目录 | 最终去向 | 处理 |
| --- | --- | --- |
| api/backend、api/frontend | api/proto | 源契约、注解和生成；去除数据库/组件等空模板 |
| client/backend、client/frontend | web/client | 共享 HTTP/SSE/WebSocket 规范，不保留 client package |
| gen/backend | api/proto、servora/proto/cmd/web | 生成约束分别回到源定义、生成器或消费方，不保留 gen package |
| web/backend、web/frontend | web/client | 配置对应共享 web 目录，实际应用规则进入对应业务端 |
| service/backend 旧模板 | service/backend 实际主题 | 按 layout/layers/编码/CRUD/测试等重写 |
| servora/backend | servora/framework、proto、cmd、web | 框架各责任分组 |
| iam-web、example-web、test-web 模板 | 各 package 根的实际主题和 index.md | 覆盖应用组织和业务交互，不强迫同一请求实现 |
| 原无 plateau、业务后端规范 | plateau 各层、iam-service、example-service、audit-service | 按确认约定及真实源码新增 |
| guides | guides | 保留跨层检查和复用思路，替换不适用模板/上游指令 |

配置维持 11 个 package，不扩展 schema。开发者在提交前确认六个业务包直接承载规范；已将 21 份 Markdown 移到包根，更新全部相对链接和引用。当前为 11 个有实际含义的共享 layer 加六个扁平业务包，各自具有有效 index；根 spec 索引提供全部 package 导航。空旧分组目录也移除。

本地 `packages_context.py` 增加包根 `specIndex` 发现，避免将无子层的业务包误报为未配置；`specLayers` 仍只表示真实子目录。文本、JSON、会话摘要和现有读取器均需验证，hooks 不变。此兼容改动属于后续 Trellis 更新时需要保留或复核的本地定制。

AGENTS 导航只修改根、app、api、app/example/web 四份。根与 api 的明显目录/生成事实同步到现状，其他有效正文保留，Trellis 托管块不变。安全子包旧文档中的剩余不准确概括在差异记录中明示，不借本任务扩大源目录文档改写。
