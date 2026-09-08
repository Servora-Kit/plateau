import Link from "next/link"

import { AuthShell } from "@/components/auth-shell"

export default function Page() {
  return (
    <AuthShell
      title="Plateau 账户"
      description="登录或创建账户以继续使用 Plateau 服务。"
    >
      <div className="grid gap-3">
        <Link
          href="/login/"
          className="flex h-10 items-center justify-center rounded-lg bg-primary text-sm font-medium text-primary-foreground"
        >
          登录
        </Link>
        <Link
          href="/register/"
          className="flex h-10 items-center justify-center rounded-lg border text-sm font-medium"
        >
          创建账户
        </Link>
      </div>
    </AuthShell>
  )
}
