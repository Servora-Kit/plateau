"use client"

import type { ConsentView } from "@plateau/api/plateau/iam/authn/v1"
import { CheckCircle2, KeyRound, LoaderCircle } from "lucide-react"
import Link from "next/link"
import { useSearchParams } from "next/navigation"
import { Suspense, useEffect, useState } from "react"

import { AppLogo, UserAvatar } from "@/components/app-logo"
import { AuthShell } from "@/components/auth-shell"
import { StatusMessage } from "@/components/status-message"
import { Button } from "@/components/ui/button"
import { useSingleFlight } from "@/hooks/use-single-flight"
import {
  isConsentRequestInvalid,
  isUnauthenticated,
  requestErrorMessage,
} from "@/lib/errors"
import { createIamApi } from "@/lib/iam-api"
import {
  dedupeScopes,
  describeOidcScope,
  resolveOidcClientName,
} from "@/lib/oidc-display"

type ConsentLoadState =
  | { status: "loading" }
  | { status: "ready"; consent: ConsentView }
  | { status: "invalid"; message: string }
  | { status: "error"; message: string }

function ConsentLoadingShell() {
  return (
    <AuthShell title="应用授权确认" description="正在核对授权请求信息…">
      <p className="text-sm text-muted-foreground" role="status">
        请稍候…
      </p>
    </AuthShell>
  )
}

function ConsentInvalidShell({ message }: { message: string }) {
  return (
    <AuthShell
      title="授权请求不可用"
      description="无法继续完成本次第三方应用授权。"
    >
      <div className="grid gap-5">
        <StatusMessage tone="info">{message}</StatusMessage>
        <Button asChild size="lg" className="h-10 w-full">
          <Link href="/account/">返回个人中心</Link>
        </Button>
      </div>
    </AuthShell>
  )
}

export default function ConsentPage() {
  return (
    <Suspense fallback={<ConsentLoadingShell />}>
      <ConsentQueryGate />
    </Suspense>
  )
}

function ConsentQueryGate() {
  const searchParams = useSearchParams()
  const requestId = searchParams.get("request_id")?.trim() ?? ""

  if (!requestId) {
    return (
      <ConsentInvalidShell message="链接缺少必要的授权请求标识（request_id），请返回发起登录的应用重新操作。" />
    )
  }

  return <ConsentFlow key={requestId} requestId={requestId} />
}

function ConsentFlow({ requestId }: { requestId: string }) {
  const { pending, run } = useSingleFlight()
  const [revision, setRevision] = useState(0)
  const [loadState, setLoadState] = useState<ConsentLoadState>({
    status: "loading",
  })
  const [activeAction, setActiveAction] = useState<
    "approve" | "deny" | "switch" | null
  >(null)
  const [actionError, setActionError] = useState<string | null>(null)

  useEffect(() => {
    const controller = new AbortController()
    const api = createIamApi(controller.signal)

    void api.authn.GetConsentView({ requestId }).then(
      ({ consent }) => {
        if (controller.signal.aborted) return
        if (!consent) {
          setLoadState({
            status: "invalid",
            message: "授权请求不存在、已完成或已过期，请返回目标应用重新发起登录。",
          })
          return
        }
        setLoadState({ status: "ready", consent })
      },
      (err: unknown) => {
        if (controller.signal.aborted) return
        if (isUnauthenticated(err)) {
          window.location.replace(
            `/login/?request_id=${encodeURIComponent(requestId)}`
          )
          return
        }
        if (isConsentRequestInvalid(err)) {
          setLoadState({
            status: "invalid",
            message: "授权请求不存在、已完成或已过期，请返回目标应用重新发起登录。",
          })
          return
        }
        setLoadState({
          status: "error",
          message: requestErrorMessage(err, "无法读取授权请求详情，请稍后重试"),
        })
      }
    )

    const handlePageShow = (event: PageTransitionEvent) => {
      if (!event.persisted) return
      setActionError(null)
      setLoadState({ status: "loading" })
      setRevision((current) => current + 1)
    }
    window.addEventListener("pageshow", handlePageShow)

    return () => {
      controller.abort()
      window.removeEventListener("pageshow", handlePageShow)
    }
  }, [requestId, revision])

  async function submitDecision(approved: boolean) {
    if (pending) return
    setActionError(null)
    setActiveAction(approved ? "approve" : "deny")

    const result = await run((signal) =>
      createIamApi(signal).authn.SubmitConsentDecision({
        requestId,
        approved,
      })
    )

    if (result.status === "success") {
      const target = result.value.redirectUri?.trim()
      window.location.assign(target || "/account/")
      return
    }

    setActiveAction(null)
    if (result.status !== "failure") return

    if (isUnauthenticated(result.error)) {
      window.location.replace(
        `/login/?request_id=${encodeURIComponent(requestId)}`
      )
      return
    }
    if (isConsentRequestInvalid(result.error)) {
      setLoadState({
        status: "invalid",
        message: "该授权请求已过期或已处理，请返回目标应用重新发起登录。",
      })
      return
    }
    setActionError(
      requestErrorMessage(
        result.error,
        approved ? "未能完成同意授权，请重试" : "未能取消本次授权，请重试"
      )
    )
  }

  async function switchAccount() {
    if (pending) return
    setActionError(null)
    setActiveAction("switch")

    const result = await run((signal) =>
      createIamApi(signal).session.Logout({})
    )

    if (
      result.status === "success" ||
      (result.status === "failure" && isUnauthenticated(result.error))
    ) {
      window.location.assign(
        `/login/?request_id=${encodeURIComponent(requestId)}`
      )
      return
    }

    setActiveAction(null)
    if (result.status === "failure") {
      setActionError(
        requestErrorMessage(result.error, "无法退出当前账户，请稍后重试")
      )
    }
  }

  if (loadState.status === "loading") {
    return <ConsentLoadingShell />
  }

  if (loadState.status === "invalid") {
    return <ConsentInvalidShell message={loadState.message} />
  }

  if (loadState.status === "error") {
    return (
      <AuthShell
        title="无法加载授权信息"
        description="读取应用授权详情时遇到问题。"
      >
        <div className="grid gap-5">
          <StatusMessage tone="error">{loadState.message}</StatusMessage>
          <div className="grid gap-3">
            <Button
              type="button"
              size="lg"
              className="h-10 w-full"
              disabled={pending}
              onClick={() => {
                setLoadState({ status: "loading" })
                setRevision((current) => current + 1)
              }}
            >
              重试
            </Button>
            <Button asChild variant="outline" size="lg" className="h-10 w-full">
              <Link href="/account/">返回个人中心</Link>
            </Button>
          </div>
        </div>
      </AuthShell>
    )
  }

  const { consent } = loadState
  const appName = resolveOidcClientName(consent, "第三方应用")
  const scopes = dedupeScopes(consent.requestedScopes)
  const grantedSet = new Set(consent.grantedScopes ?? [])

  return (
    <AuthShell
      badge={<AppLogo src={consent.logoUri} name={appName} size="lg" />}
      title={
        <>
          <span className="text-foreground">{appName}</span> 请求访问您的账户
        </>
      }
      description={
        <div className="space-y-3">
          <p>请确认您信任此应用，并核对它将获得的 Plateau 账户访问权限。</p>
          <div className="flex flex-wrap items-center gap-2.5 pt-0.5">
            <div className="inline-flex max-w-full items-center gap-2 rounded-full border bg-muted/50 py-1 pr-3 pl-1.5 text-xs font-medium text-foreground">
              <UserAvatar name={consent.userEmail || "U"} size="xs" />
              <span className="truncate">
                {consent.userEmail || "当前已登录账户"}
              </span>
            </div>
            <button
              type="button"
              disabled={pending}
              onClick={() => void switchAccount()}
              className="inline-flex items-center gap-1 rounded text-xs font-medium text-muted-foreground underline underline-offset-4 outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring disabled:pointer-events-none disabled:opacity-50"
            >
              {activeAction === "switch" ? (
                <LoaderCircle className="size-3 animate-spin" aria-hidden="true" />
              ) : null}
              切换账号
            </button>
          </div>
        </div>
      }
    >
      <div className="grid gap-5" aria-busy={pending}>
        {actionError ? (
          <StatusMessage tone="error">{actionError}</StatusMessage>
        ) : null}

        <div className="grid gap-2">
          <h2 className="text-xs font-semibold tracking-wide text-muted-foreground uppercase">
            允许 {appName} 执行以下操作
          </h2>
          {scopes.length > 0 ? (
            <ul
              className="divide-y rounded-xl border bg-muted/20"
              aria-label="请求的权限范围"
            >
              {scopes.map((rawScope) => {
                const descriptor = describeOidcScope(rawScope)
                const alreadyGranted = grantedSet.has(descriptor.scope)
                return (
                  <li
                    key={descriptor.scope}
                    className="flex items-start gap-3 px-4 py-3.5 text-sm"
                  >
                    {descriptor.known ? (
                      <CheckCircle2
                        className="mt-0.5 size-4 shrink-0 text-primary"
                        aria-hidden="true"
                      />
                    ) : (
                      <KeyRound
                        className="mt-0.5 size-4 shrink-0 text-muted-foreground"
                        aria-hidden="true"
                      />
                    )}
                    <div className="min-w-0 flex-1 space-y-1">
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="leading-5 font-medium text-foreground">
                          {descriptor.description}
                        </span>
                        {alreadyGranted ? (
                          <span className="inline-flex items-center rounded-full border bg-background px-2 py-0.5 text-[11px] font-medium text-muted-foreground">
                            此前已授权
                          </span>
                        ) : null}
                      </div>
                      <p className="font-mono text-xs text-muted-foreground">
                        {descriptor.scope}
                      </p>
                    </div>
                  </li>
                )
              })}
            </ul>
          ) : (
            <div className="rounded-xl border bg-muted/20 px-4 py-3.5 text-sm text-muted-foreground">
              验证您的基础身份标识
            </div>
          )}
        </div>

        <p className="text-xs leading-5 text-muted-foreground">
          同意授权后，您可以随时前往个人中心的「第三方应用与服务」查看或移除此应用的访问权限。
        </p>

        <div className="grid gap-3 sm:grid-cols-2">
          <Button
            type="button"
            variant="outline"
            size="lg"
            className="order-2 h-10 w-full sm:order-1"
            disabled={pending}
            onClick={() => void submitDecision(false)}
          >
            {activeAction === "deny" ? (
              <LoaderCircle className="animate-spin" aria-hidden="true" />
            ) : null}
            取消
          </Button>
          <Button
            type="button"
            size="lg"
            className="order-1 h-10 w-full sm:order-2"
            disabled={pending}
            onClick={() => void submitDecision(true)}
          >
            {activeAction === "approve" ? (
              <LoaderCircle className="animate-spin" aria-hidden="true" />
            ) : null}
            同意授权
          </Button>
        </div>
      </div>
    </AuthShell>
  )
}
