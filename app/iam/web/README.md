# IAM Web

IAM Web 使用 Next.js App Router，通过同源路径调用 IAM、CAP 与 OIDC 接口。Next 的原生 rewrite 将这些路径转发到 `IAM_BACKEND_ORIGIN`。

## 本地开发

推荐在仓库根目录通过一键编排启动全栈环境：

```bash
just init
just dev
```

本地默认通过 Traefik 网关以 `http://iam.plateau.localhost:8080` 访问（遵循 RFC 6761 免改 `/etc/hosts`），网关将 API、CAP、OIDC 与 `/docs` 路径直转 `iam.service:10000`，其余页面流量转发至 `iam-web:10002`。

若脱离网关单独调试，也可保留 Next.js 内置同源 rewrite 回退能力，在两个终端分别运行 `just service::iam::run` 与 `just web::iam::dev` 并覆盖 `IAM_PUBLIC_ORIGIN=http://localhost:10002`。修改公开入口时须同步邮件与 OIDC 地址；非回环环境必须使用受信任的 HTTPS。

## 构建与运行

```bash
just web::iam::build
pnpm --filter @plateau/iam-web run preview
```

`dev` 使用 `next dev` 热重载，`preview` 和 `start` 使用 `next start`；默认共用 `10002`，不要同时启动。
