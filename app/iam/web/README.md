# IAM Web

IAM Web 使用 Next.js App Router，通过同源路径调用 IAM、CAP 与 OIDC 接口。Next 的原生 rewrite 将这些路径转发到 `IAM_BACKEND_ORIGIN`。

## 本地开发

在 Plateau 根目录执行 `pnpm install`，并启动 Docker 中间件。随后在两个终端分别运行：

```bash
just service::iam::run
just web::iam::dev
```

Web 默认入口为 `http://localhost:10002`，Go HTTP/gRPC 使用 `10000/10001`。修改入口时通过 `IAM_PUBLIC_ORIGIN` 同步邮件与 OIDC 地址；非 localhost 环境应使用受信任的 HTTPS。

## 构建与运行

```bash
just web::iam::build
just web::iam::preview
```

`dev` 使用 `next dev` 热重载，`preview` 和 `start` 使用 `next start`；默认共用 `10002`，不要同时启动。
