# 本地端口配置核查

2026-09-16，只读调查。用户明确这只是根 AGENTS.md 和应用前后端配置的附带调整，不另建迁移流程。以下是当前文件事实与实施目标，不表示服务正在运行或改动已实施。

## 新登记目标

| 应用 | 端口段 | HTTP | gRPC | Web |
| --- | --- | --- | --- | --- |
| IAM | 10000–10009 | 10000 | 10001 | 10002 |
| Admin | 10010–10019 | 10010 | 10011（预留） | 10012 |
| Example | 10080–10089 | 10080 | 10081 | 10082 |
| Test | 10090–10099 | 10090（预留） | 10091（预留） | 10092 |

Audit/CMS 暂不在新表登记。现有 Audit 配置占用 10010/10011；主 design 按用户“Admin 后面”的顺序将其顺移到 10020/10021，避免冲突，不涉及 Audit 功能建设。后续新增服务仍须检查实际配置占用。

## 实际配置位置

| 范围 | 当前证据 | 目标及边界 |
| --- | --- | --- |
| 根规则 | [AGENTS.md](../../../../AGENTS.md):25–36 仍是旧布局 | 更新上表及从中间空闲段分配的规则 |
| Example 后端 | [local/config.yaml](../../../../app/example/service/configs/local/config.yaml):4、8 为 10030/10031 | 改 10080/10081 |
| Example Web | [vite.config.ts](../../../../app/example/web/vite.config.ts):22、26、32 为 Web 10032、代理 10030 | dev/preview 改 10082，代理改 10080 |
| Test Web | [package.json](../../../../app/test/web/package.json):6、8 为 dev/start 10042 | 改 10092，10090/10091 只预留 |
| Test 回调 | IAM [local/oidc.yaml](../../../../app/iam/service/configs/local/oidc.yaml):9、[docker/oidc.yaml](../../../../app/iam/service/configs/docker/oidc.yaml):9 注释 callback 为 10042 | 改 10092 |
| Admin 后端 | 用户已删除 `app/admin/service`。删除前快照：`configs/local/bootstrap.yaml:6,10` 曾沿用 IAM 10000/10001，应用名和 origin 也指 IAM；该旧文件不再存在 | 在新建服务配置中设置 HTTP 10010，gRPC 槽位 10011；以重建后实际配置验收 |
| Admin Web | [.env.development](../../../../app/admin/web/apps/web-antd/.env.development):2 为 5666，仍开启 mock；[vite.config.ts](../../../../app/admin/web/apps/web-antd/vite.config.ts):7–16 将 /api 代理至 mock 5320 | dev/preview 统一 10012，真实 /auth、/v1/admin 代理到 10010，随原有 Admin 接入替换 mock |
| Audit 原生 | [local/bootstrap.yaml](../../../../app/audit/service/configs/local/bootstrap.yaml):4、8 为 10010/10011，:43 本地发现标签引用 10010 | 顺移 10020/10021，相关本地标签同步 |
| Audit 宿主映射 | [docker-compose.apps.yaml](../../../../docker-compose.apps.yaml):12–13 为 10010:8000、10011:8001 | 只改左侧为 10020、10021；内部 8000/8001 不动 |

IAM 的 [local bootstrap](../../../../app/iam/service/configs/local/bootstrap.yaml)、[Web package](../../../../app/iam/web/package.json)、[Web proxy](../../../../app/iam/web/next.config.ts) 和 [环境样例](../../../../.env.example) 继续使用 10000/10001/10002。本地 issuer 不变，仅同步受影响的 client callback。

排除 lockfile、生成产物和依赖噪声后，未发现 Example/Test 新段已有配置占用。Compose 当前只有 Audit 应用映射；中间件、容器内部和生产公开端口不重排。

## 工作区与验证

- 2026-09-16 后续更新：用户已删除 `app/admin/service`，本轮核实目录不存在；按 IAM、Example 与 Trellis 规范重新搭建，旧复制内容仅作为删除前快照。实施时仍先检查届时工作区，保留 Web 与其他已有改动。
- 活跃 task 设计与接入 research 已同步新端口。其他文档/页面可能含旧地址，本次不扩大为批量文档治理；spec 在规划期间不改。
- 核对监听、dev/preview、代理、callback、宿主映射及端口冲突，随原有应用联调完成，不新增测试框架或迁移工具。
