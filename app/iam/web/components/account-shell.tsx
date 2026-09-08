"use client"

import { LockKeyhole, UserRound } from "lucide-react"
import Link from "next/link"
import { usePathname } from "next/navigation"

import { cn } from "@/lib/utils"

const navigation = [
  { href: "/account/", label: "个人资料", icon: UserRound },
  { href: "/account/security/", label: "账户安全", icon: LockKeyhole },
]

interface AccountShellProps {
  title: string
  description: string
  email?: string
  children: React.ReactNode
}

export function AccountShell({
  title,
  description,
  email,
  children,
}: AccountShellProps) {
  const pathname = usePathname()

  return (
    <main
      id="main-content"
      className="mx-auto min-h-dvh w-full max-w-5xl px-4 py-8 sm:px-6 sm:py-12"
    >
      <header className="mb-8 border-b pb-6">
        <p className="text-sm font-semibold">Plateau 账户</p>
        <div className="mt-3 flex flex-col justify-between gap-2 sm:flex-row sm:items-end">
          <div>
            <h1 className="text-2xl font-semibold tracking-tight">{title}</h1>
            <p className="mt-1 text-sm leading-6 text-muted-foreground">
              {description}
            </p>
          </div>
          {email ? (
            <p className="overflow-wrap-anywhere max-w-full text-sm text-muted-foreground">
              {email}
            </p>
          ) : null}
        </div>
      </header>
      <div className="grid gap-8 md:grid-cols-[12rem_minmax(0,1fr)]">
        <nav aria-label="账户设置" className="flex gap-2 md:flex-col">
          {navigation.map(({ href, label, icon: Icon }) => {
            const active = pathname === href || pathname === href.slice(0, -1)
            return (
              <Link
                key={href}
                href={href}
                aria-current={active ? "page" : undefined}
                className={cn(
                  "flex min-h-10 flex-1 items-center gap-2 rounded-lg px-3 py-2 text-sm font-medium outline-none focus-visible:ring-3 focus-visible:ring-ring md:flex-none",
                  active
                    ? "bg-muted text-foreground"
                    : "text-muted-foreground hover:bg-muted/60 hover:text-foreground"
                )}
              >
                <Icon className="size-4" aria-hidden="true" />
                {label}
              </Link>
            )
          })}
        </nav>
        <section className="min-w-0">{children}</section>
      </div>
    </main>
  )
}
