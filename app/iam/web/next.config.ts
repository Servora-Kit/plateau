import path from "node:path"
import type { NextConfig } from "next"

const workspaceRoot = path.join(__dirname, "../../../..")
const iamBackendOrigin = (
  process.env.IAM_BACKEND_ORIGIN ?? "http://127.0.0.1:10000"
).replace(/\/+$/u, "")
const development = process.env.NODE_ENV !== "production"
const contentSecurityPolicy = [
  "default-src 'self'",
  `script-src 'self' 'unsafe-inline' 'wasm-unsafe-eval'${development ? " 'unsafe-eval'" : ""}`,
  "style-src 'self' 'unsafe-inline'",
  "img-src 'self' data:",
  "font-src 'self'",
  "connect-src 'self' ws: wss:",
  "worker-src 'self' blob:",
  "frame-src 'self'",
  "object-src 'none'",
  "base-uri 'self'",
  "form-action 'self'",
  "frame-ancestors 'none'",
].join("; ")

const nextConfig: NextConfig = {
  trailingSlash: true,
  skipTrailingSlashRedirect: true,
  outputFileTracingRoot: workspaceRoot,
  transpilePackages: ["@plateau/api", "@plateau/client"],
  logging: {
    incomingRequests: {
      ignore: [/^\/login\/?(?:\?|$)/u, /^\/authorize\/callback\/?(?:\?|$)/u],
    },
    browserToTerminal: false,
    serverFunctions: false,
  },
  turbopack: {
    root: workspaceRoot,
  },
  async headers() {
    return [
      {
        source: "/:path*",
        headers: [
          { key: "Content-Security-Policy", value: contentSecurityPolicy },
          { key: "Cross-Origin-Opener-Policy", value: "same-origin" },
          { key: "Cross-Origin-Resource-Policy", value: "same-origin" },
          {
            key: "Permissions-Policy",
            value: "camera=(), microphone=(), geolocation=()",
          },
          { key: "Referrer-Policy", value: "no-referrer" },
          { key: "X-Content-Type-Options", value: "nosniff" },
          { key: "X-Frame-Options", value: "DENY" },
        ],
      },
    ]
  },
  async rewrites() {
    const paths = [
      "/v1/:path*",
      "/cap/:path*",
      "/.well-known/:path*",
      "/keys",
      "/authorize/:path*",
      "/oauth/:path*",
      "/userinfo",
      "/revoke",
      "/end_session",
    ]
    return {
      beforeFiles: paths.map((source) => ({
        source,
        destination: `${iamBackendOrigin}${source}`,
      })),
    }
  },
}

export default nextConfig
