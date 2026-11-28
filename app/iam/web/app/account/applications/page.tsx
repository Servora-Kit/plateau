"use client"

import type { AuthorizedApp } from "@plateau/api/plateau/iam/account/v1"
import { Blocks, Clock, LoaderCircle, ShieldOff } from "lucide-react"
import { useRouter } from "next/navigation"
import { useEffect, useState } from "react"

import { AccountCard, AccountShell } from "@/components/account-shell"
import { AppLogo } from "@/components/app-logo"
import { StatusMessage } from "@/components/status-message"
import { Button } from "@/components/ui/button"
import { useProfile } from "@/hooks/use-profile"
import { useSingleFlight } from "@/hooks/use-single-flight"
import { isUnauthenticated, requestErrorMessage } from "@/lib/errors"
import { createIamApi } from "@/lib/iam-api"
import {
  dedupeScopes,
  describeOidcScope,
  formatDateTime,
  resolveOidcClientName,
} from "@/lib/oidc-display"

type AppsLoadState =
  | { status: "loading" }
  | { status: "ready"; applications: AuthorizedApp[] }
  | { status: "error"; message: string }

export default function AccountApplicationsPage() {
  const { state, reload } = useProfile()

  if (state.status === "loading" || state.status === "unauthenticated") {
    return (
      <AccountShell
        title="第三方应用与服务"
        description="查看已获准访问您 Plateau 账户的第三方应用，并随时管理或移除其访问权限。"
      >
        <div className="rounded-2xl border bg-card p-6 shadow-2xs">
          <p className="text-sm text-muted-foreground" role="status">
            正在确认账户状态…
          </p>
        </div>
      </AccountShell>
    )
  }

  if (state.status === "error") {
    return (
      <AccountShell
        title="第三方应用与服务"
        description="查看已获准访问您 Plateau 账户的第三方应用，并随时管理或移除其访问权限。"
      >
        <div className="rounded-2xl border bg-card p-6 shadow-2xs">
          <StatusMessage tone="error">{state.message}</StatusMessage>
          <Button className="mt-4 h-10" onClick={() => void reload()}>
            重新加载
          </Button>
        </div>
      </AccountShell>
    )
  }

  return (
    <AccountShell
      title="第三方应用与服务"
      description="查看已获准访问您 Plateau 账户的第三方应用，并随时管理或移除其访问权限。"
      user={state.user}
      email={state.user.email}
    >
      <AuthorizedAppsPanel />
    </AccountShell>
  )
}

function AuthorizedAppsPanel() {
  const router = useRouter()
  const { pending, run } = useSingleFlight()
  const [revision, setRevision] = useState(0)
  const [loadState, setLoadState] = useState<AppsLoadState>({
    status: "loading",
  })
  const [confirmingClientId, setConfirmingClientId] = useState<string | null>(
    null
  )
  const [feedback, setFeedback] = useState<{
    tone: "error" | "success"
    text: string
  } | null>(null)

  useEffect(() => {
    const controller = new AbortController()
    void createIamApi(controller.signal)
      .account.ListAuthorizedApps({})
      .then(
        ({ applications }) => {
          if (controller.signal.aborted) return
          setLoadState({
            status: "ready",
            applications: applications ?? [],
          })
        },
        (err: unknown) => {
          if (controller.signal.aborted) return
          if (isUnauthenticated(err)) {
            router.replace("/login/")
            return
          }
          setLoadState({
            status: "error",
            message: requestErrorMessage(
              err,
              "无法读取已授权的第三方应用列表，请重试"
            ),
          })
        }
      )
    return () => controller.abort()
  }, [router, revision])

  async function revokeApp(app: AuthorizedApp) {
    const clientId = app.clientId?.trim()
    if (pending || !clientId) return

    const appName = resolveOidcClientName(app, clientId)
    setFeedback(null)

    const result = await run((signal) =>
      createIamApi(signal).account.RevokeAuthorizedApp({ clientId })
    )

    if (result.status === "success") {
      setConfirmingClientId(null)
      setLoadState((current) =>
        current.status === "ready"
          ? {
              status: "ready",
              applications: current.applications.filter(
                (item) => item.clientId !== clientId
              ),
            }
          : current
      )
      setFeedback({
        tone: "success",
        text: `已移除「${appName}」的访问权限，关联的活跃授权会话已立即失效。`,
      })
      return
    }

    if (result.status !== "failure") return

    if (isUnauthenticated(result.error)) {
      router.replace("/login/")
      return
    }
    setFeedback({
      tone: "error",
      text: requestErrorMessage(
        result.error,
        `无法移除「${appName}」的访问权限，请稍后重试`
      ),
    })
  }

  if (loadState.status === "loading") {
    return (
      <div className="rounded-2xl border bg-card p-6 shadow-2xs">
        <p className="text-sm text-muted-foreground" role="status">
          正在加载已授权的第三方应用…
        </p>
      </div>
    )
  }

  if (loadState.status === "error") {
    return (
      <div className="rounded-2xl border bg-card p-6 shadow-2xs">
        <StatusMessage tone="error">{loadState.message}</StatusMessage>
        <Button
          type="button"
          className="mt-4 h-10"
          onClick={() => {
            setLoadState({ status: "loading" })
            setRevision((current) => current + 1)
          }}
        >
          重新加载
        </Button>
      </div>
    )
  }

  const { applications } = loadState

  return (
    <div className="grid gap-6">
      {feedback ? (
        <StatusMessage tone={feedback.tone}>{feedback.text}</StatusMessage>
      ) : null}

      {applications.length === 0 ? (
        <section className="rounded-2xl border bg-card px-6 py-12 text-center shadow-2xs">
          <div className="mx-auto flex size-12 items-center justify-center rounded-full bg-muted text-muted-foreground">
            <Blocks className="size-6" aria-hidden="true" />
          </div>
          <h2 className="mt-4 text-base font-semibold text-foreground sm:text-lg">
            您尚未向任何第三方应用授予账户访问权限
          </h2>
          <p className="mx-auto mt-2 max-w-md text-sm leading-6 text-muted-foreground">
            当您使用 Plateau 账户登录外部应用并同意授权后，应用记录将在此处展示，您可以随时查看权限详情或撤销访问权限。
          </p>
        </section>
      ) : (
        <AccountCard
          id="authorized-apps"
          title="已获授权的应用"
          description="以下第三方应用已获得您的授权，可按所列范围访问您的 Plateau 账户信息。"
          badge={
            <span className="inline-flex items-center rounded-full border bg-muted/60 px-2.5 py-0.5 text-xs font-medium text-muted-foreground">
              共 {applications.length} 个应用
            </span>
          }
        >
          <ul className="divide-y" aria-label="已授权的第三方应用">
            {applications.map((app, index) => {
              const clientId = app.clientId?.trim() || ""
              const itemKey = clientId || `app-${index}`
              const appName = resolveOidcClientName(app, "第三方应用")
              const scopes = dedupeScopes(app.grantedScopes)
              const authorizedAt = formatDateTime(
                app.updateTime || app.createTime
              )
              const isConfirming =
                Boolean(clientId) && confirmingClientId === clientId

              return (
                <li key={itemKey} className="px-5 py-5 sm:px-6">
                  <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
                    <div className="flex min-w-0 items-start gap-3.5">
                      <AppLogo src={app.logoUri} name={appName} size="md" />
                      <div className="min-w-0 flex-1 space-y-2">
                        <div className="flex flex-wrap items-baseline gap-x-2 gap-y-1">
                          <h3 className="text-base font-semibold text-foreground">
                            {appName}
                          </h3>
                          {clientId && app.clientName?.trim() ? (
                            <span className="font-mono text-xs text-muted-foreground">
                              {clientId}
                            </span>
                          ) : null}
                        </div>

                        {app.description?.trim() ? (
                          <p className="text-sm leading-6 text-muted-foreground">
                            {app.description.trim()}
                          </p>
                        ) : null}

                        {authorizedAt ? (
                          <p className="flex items-center gap-1.5 text-xs text-muted-foreground">
                            <Clock className="size-3.5 shrink-0" aria-hidden="true" />
                            <span>授权时间：{authorizedAt}</span>
                          </p>
                        ) : null}

                        {scopes.length > 0 ? (
                          <div className="flex flex-wrap gap-1.5 pt-0.5">
                            {scopes.map((rawScope) => {
                              const descriptor = describeOidcScope(rawScope)
                              return (
                                <span
                                  key={descriptor.scope}
                                  title={descriptor.description}
                                  className="inline-flex items-center gap-1 rounded-full border bg-muted/45 px-2.5 py-0.5 text-xs text-foreground"
                                >
                                  <span>{descriptor.title}</span>
                                  {descriptor.known ? (
                                    <span className="font-mono text-[11px] text-muted-foreground">
                                      ({descriptor.scope})
                                    </span>
                                  ) : null}
                                </span>
                              )
                            })}
                          </div>
                        ) : null}
                      </div>
                    </div>

                    {clientId && !isConfirming ? (
                      <div className="shrink-0">
                        <Button
                          type="button"
                          variant="outline"
                          className="h-9 w-full sm:w-auto"
                          disabled={pending}
                          onClick={() => {
                            setFeedback(null)
                            setConfirmingClientId(clientId)
                          }}
                        >
                          <ShieldOff className="size-4" aria-hidden="true" />
                          移除访问权限
                        </Button>
                      </div>
                    ) : null}
                  </div>

                  {isConfirming ? (
                    <div
                      className="mt-4 rounded-xl border border-destructive/30 bg-destructive/5 p-4"
                      role="region"
                      aria-label={`确认移除 ${appName} 的访问权限`}
                      aria-busy={pending}
                    >
                      <p className="text-sm leading-6 text-foreground">
                        移除后，
                        <strong className="font-semibold">{appName}</strong>{" "}
                        将无法继续访问您的 Plateau 账户，当前活跃授权会话将立即失效。
                      </p>
                      <div className="mt-3.5 flex flex-col gap-2.5 sm:flex-row">
                        <Button
                          type="button"
                          variant="destructive"
                          className="h-9 sm:w-auto"
                          disabled={pending}
                          onClick={() => void revokeApp(app)}
                        >
                          {pending ? (
                            <LoaderCircle
                              className="animate-spin"
                              aria-hidden="true"
                            />
                          ) : null}
                          确认移除访问权限
                        </Button>
                        <Button
                          type="button"
                          variant="outline"
                          className="h-9 sm:w-auto"
                          disabled={pending}
                          onClick={() => setConfirmingClientId(null)}
                        >
                          取消
                        </Button>
                      </div>
                    </div>
                  ) : null}
                </li>
              )
            })}
          </ul>
        </AccountCard>
      )}
    </div>
  )
}
