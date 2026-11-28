"use client"

import type { plateauiamuserv1_User } from "@plateau/api/plateau/iam/account/v1"
import {
  Blocks,
  LoaderCircle,
  LogOut,
  ShieldCheck,
  UserRound,
} from "lucide-react"
import Link from "next/link"
import { usePathname } from "next/navigation"
import { createContext, useContext, useState } from "react"

import { UserAvatar } from "@/components/app-logo"
import { StatusMessage } from "@/components/status-message"
import { Button } from "@/components/ui/button"
import { useProfile } from "@/hooks/use-profile"
import { useSingleFlight } from "@/hooks/use-single-flight"
import { isUnauthenticated, requestErrorMessage } from "@/lib/errors"
import { createIamApi } from "@/lib/iam-api"
import { cn } from "@/lib/utils"

const navigation = [
  { href: "/account/", label: "个人信息", icon: UserRound, exact: true },
  {
    href: "/account/security/",
    label: "安全性与登录",
    icon: ShieldCheck,
    exact: false,
  },
  {
    href: "/account/applications/",
    label: "第三方应用与服务",
    icon: Blocks,
    exact: false,
  },
] as const

function trimTrailingSlash(path: string): string {
  if (path.length > 1 && path.endsWith("/")) {
    return path.slice(0, -1)
  }
  return path
}

function isNavItemActive(
  pathname: string,
  href: string,
  exact: boolean
): boolean {
  const current = trimTrailingSlash(pathname)
  const target = trimTrailingSlash(href)
  if (exact) {
    return current === target
  }
  return current === target || current.startsWith(`${target}/`)
}

const AccountFrameMountedContext = createContext(false)

interface AccountLayoutFrameProps {
  userOverride?: plateauiamuserv1_User | null
  emailOverride?: string
  children: React.ReactNode
}

export function AccountLayoutFrame({
  userOverride,
  emailOverride,
  children,
}: AccountLayoutFrameProps) {
  const pathname = usePathname()
  const { state } = useProfile()
  const { pending: loggingOut, run } = useSingleFlight()
  const [logoutError, setLogoutError] = useState<string | null>(null)

  const resolvedUser =
    userOverride ?? (state.status === "ready" ? state.user : null)
  const displayEmail = resolvedUser?.email || emailOverride || ""
  const displayName = resolvedUser?.profile?.name?.trim() || displayEmail || "P"
  const avatarPicture = resolvedUser?.profile?.picture
  const isIdentityLoading =
    !resolvedUser && !emailOverride && state.status === "loading"

  async function handleHeaderLogout() {
    if (loggingOut) return
    setLogoutError(null)

    const result = await run((signal) =>
      createIamApi(signal).session.Logout({})
    )
    if (
      result.status === "success" ||
      (result.status === "failure" && isUnauthenticated(result.error))
    ) {
      window.location.assign("/login/")
      return
    }
    if (result.status === "failure") {
      setLogoutError(
        requestErrorMessage(
          result.error,
          "无法确认是否已退出登录，请保持此页面并重试"
        )
      )
    }
  }

  return (
    <AccountFrameMountedContext.Provider value={true}>
      <div className="min-h-dvh bg-muted/20">
        <header className="sticky top-0 z-30 border-b bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/80">
          <div className="mx-auto flex h-16 max-w-6xl items-center justify-between gap-4 px-4 sm:px-6">
            <Link
              href="/account/"
              className="inline-flex items-center gap-2.5 rounded-lg py-1 text-base font-semibold tracking-tight text-foreground outline-none focus-visible:ring-3 focus-visible:ring-ring"
            >
              <span className="inline-flex size-8 items-center justify-center rounded-lg bg-primary text-primary-foreground">
                <ShieldCheck className="size-4" aria-hidden="true" />
              </span>
              <span>Plateau 账户</span>
            </Link>

            <div className="flex min-w-0 items-center gap-2.5 sm:gap-3">
              {isIdentityLoading ? (
                <div
                  className="flex items-center gap-2.5"
                  aria-hidden="true"
                >
                  <span className="hidden h-4 w-28 animate-pulse rounded bg-muted sm:inline-block" />
                  <span className="size-8 animate-pulse rounded-full bg-muted" />
                </div>
              ) : displayEmail ? (
                <>
                  <div
                    className="flex min-w-0 items-center gap-2.5 rounded-full border bg-card py-1 pr-1.5 pl-3 shadow-2xs"
                    title={displayEmail}
                  >
                    <span className="hidden max-w-52 truncate text-xs font-medium text-muted-foreground sm:inline">
                      {displayEmail}
                    </span>
                    <UserAvatar
                      src={avatarPicture}
                      name={displayName}
                      size="sm"
                    />
                  </div>
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    className="h-8 rounded-full px-3 text-muted-foreground hover:text-foreground"
                    disabled={loggingOut}
                    onClick={() => void handleHeaderLogout()}
                  >
                    {loggingOut ? (
                      <LoaderCircle
                        className="size-3.5 animate-spin"
                        aria-hidden="true"
                      />
                    ) : (
                      <LogOut className="size-3.5" aria-hidden="true" />
                    )}
                    <span>退出</span>
                  </Button>
                </>
              ) : null}
            </div>
          </div>
        </header>

        <div className="mx-auto w-full max-w-6xl px-4 py-6 sm:px-6 md:py-8">
          {logoutError ? (
            <div className="mb-6">
              <StatusMessage tone="error">{logoutError}</StatusMessage>
            </div>
          ) : null}

          <div className="grid gap-6 md:grid-cols-[15rem_minmax(0,1fr)] lg:gap-10">
            <aside className="min-w-0 md:sticky md:top-24 md:self-start">
              <nav
                aria-label="账户中心导航"
                className="flex gap-1.5 overflow-x-auto pb-1 md:flex-col md:overflow-visible md:pb-0"
              >
                {navigation.map(({ href, label, icon: Icon, exact }) => {
                  const active = isNavItemActive(pathname, href, exact)
                  return (
                    <Link
                      key={href}
                      href={href}
                      aria-current={active ? "page" : undefined}
                      className={cn(
                        "inline-flex shrink-0 items-center gap-3 rounded-full px-4 py-2.5 text-sm font-medium transition-colors outline-none focus-visible:ring-3 focus-visible:ring-ring",
                        active
                          ? "bg-primary/10 font-semibold text-primary dark:bg-primary/20"
                          : "text-muted-foreground hover:bg-muted/80 hover:text-foreground"
                      )}
                    >
                      <Icon className="size-4 shrink-0" aria-hidden="true" />
                      <span>{label}</span>
                    </Link>
                  )
                })}
              </nav>
            </aside>

            <main id="main-content" className="min-w-0">
              {children}
            </main>
          </div>
        </div>
      </div>
    </AccountFrameMountedContext.Provider>
  )
}

export interface AccountShellProps {
  title: string
  description: string
  email?: string
  user?: plateauiamuserv1_User | null
  children: React.ReactNode
}

export function AccountShell({
  title,
  description,
  email,
  user,
  children,
}: AccountShellProps) {
  const insideFrame = useContext(AccountFrameMountedContext)

  const content = (
    <div className="mx-auto w-full max-w-3xl">
      <header className="mb-6 sm:mb-8">
        <h1 className="text-2xl font-semibold tracking-tight text-foreground sm:text-3xl">
          {title}
        </h1>
        <p className="mt-2 text-sm leading-6 text-muted-foreground sm:text-base">
          {description}
        </p>
      </header>
      <div className="grid gap-6">{children}</div>
    </div>
  )

  if (insideFrame) {
    return content
  }

  return (
    <AccountLayoutFrame userOverride={user} emailOverride={email}>
      {content}
    </AccountLayoutFrame>
  )
}

export interface AccountCardProps {
  id?: string
  title: string
  description?: React.ReactNode
  badge?: React.ReactNode
  tone?: "default" | "danger"
  children: React.ReactNode
}

export function AccountCard({
  id,
  title,
  description,
  badge,
  tone = "default",
  children,
}: AccountCardProps) {
  const headingId = id ? `${id}-heading` : undefined

  return (
    <section
      aria-labelledby={headingId}
      className={cn(
        "overflow-hidden rounded-2xl border bg-card text-card-foreground shadow-2xs",
        tone === "danger" && "border-destructive/35"
      )}
    >
      <div className="border-b px-5 py-5 sm:px-6">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <h2
            id={headingId}
            className={cn(
              "text-base font-semibold tracking-tight sm:text-lg",
              tone === "danger" && "text-destructive"
            )}
          >
            {title}
          </h2>
          {badge}
        </div>
        {description ? (
          <div className="mt-1 text-sm leading-6 text-muted-foreground">
            {description}
          </div>
        ) : null}
      </div>
      <div>{children}</div>
    </section>
  )
}
