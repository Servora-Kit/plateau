"use client"

import type { AuthFlow } from "@plateau/api/plateau/iam/authn/v1"
import { AuthnErrorReason } from "@plateau/api/plateau/iam/authn/v1/authn.errors"
import Link from "next/link"
import { useCallback, useEffect, useState, type FormEvent } from "react"

import { AuthShell } from "@/components/auth-shell"
import {
  PasswordField,
  SubmitButton,
  TextField,
} from "@/components/form-controls"
import { StatusMessage } from "@/components/status-message"
import { Button } from "@/components/ui/button"
import { useSensitiveValue } from "@/hooks/use-sensitive-value"
import { useSingleFlight } from "@/hooks/use-single-flight"
import { authnErrorReason, requestErrorMessage } from "@/lib/errors"
import { createIamApi } from "@/lib/iam-api"
import { validateEmail, validatePassword } from "@/lib/validation"

function syncFlowUrl(flowId?: string) {
  const nextUrl = flowId
    ? `${window.location.pathname}?flow=${encodeURIComponent(flowId)}`
    : window.location.pathname
  window.history.replaceState(window.history.state, "", nextUrl)
}

function expiredOrInvalidNotice(reason: AuthnErrorReason | null): string {
  switch (reason) {
    case AuthnErrorReason.AUTHN_ERROR_REASON_FLOW_EXPIRED:
      return "登录流程已超时，请重新输入凭据继续。"
    case AuthnErrorReason.AUTHN_ERROR_REASON_FLOW_INVALIDATED:
      return "账户或凭据状态已变更，请重新登录。"
    case AuthnErrorReason.AUTHN_ERROR_REASON_OIDC_REQUEST_INVALID:
      return "原应用授权请求已失效或过期，已切换为独立账户登录；如需授权原应用请返回该应用重新发起登录。"
    default:
      return "原登录流程已结束或不可用，请重新登录。"
  }
}

export default function LoginPage() {
  const password = useSensitiveValue()
  const newPassword = useSensitiveValue()
  const confirmation = useSensitiveValue()
  const { pending, run } = useSingleFlight()

  const [revision, setRevision] = useState(0)
  const [ready, setReady] = useState(false)
  const [flow, setFlow] = useState<AuthFlow | null>(null)
  const [oidcInvalid, setOidcInvalid] = useState(false)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [email, setEmail] = useState("")
  const [loginErrors, setLoginErrors] = useState<{
    email?: string
    password?: string
  }>({})
  const [updateErrors, setUpdateErrors] = useState<{
    newPassword?: string
    confirmation?: string
  }>({})
  const [message, setMessage] = useState<string | null>(null)
  const [messageTone, setMessageTone] = useState<"error" | "info">("error")

  const { clear: clearPassword } = password
  const { clear: clearNewPassword } = newPassword
  const { clear: clearConfirmation } = confirmation

  const clearSensitiveInputs = useCallback(() => {
    clearPassword()
    clearNewPassword()
    clearConfirmation()
  }, [clearConfirmation, clearNewPassword, clearPassword])

  const applyAuthFlow = useCallback((nextFlow: AuthFlow): boolean => {
    if (nextFlow.state === "AUTH_FLOW_STATE_COMPLETED") {
      window.location.replace(nextFlow.continueUrl || "/account/")
      return true
    }
    if (nextFlow.flowId) {
      syncFlowUrl(nextFlow.flowId)
    }
    setFlow(nextFlow)
    setOidcInvalid(false)
    setLoadError(null)
    return false
  }, [])

  useEffect(() => {
    const controller = new AbortController()
    const api = createIamApi(controller.signal)

    async function bootstrapFlow() {
      const params = new URLSearchParams(window.location.search)
      const urlFlowId = params.get("flow")?.trim() || ""
      const urlRequestId = params.get("request_id")?.trim() || ""

      if (urlFlowId) {
        try {
          const { flow: existing } = await api.authn.GetAuthFlow({
            flowId: urlFlowId,
          })
          if (controller.signal.aborted || !existing) return
          if (applyAuthFlow(existing)) return
          setReady(true)
          return
        } catch (err) {
          if (controller.signal.aborted) return
          const reason = authnErrorReason(err)
          if (
            reason === AuthnErrorReason.AUTHN_ERROR_REASON_FLOW_NOT_FOUND ||
            reason === AuthnErrorReason.AUTHN_ERROR_REASON_FLOW_EXPIRED ||
            reason === AuthnErrorReason.AUTHN_ERROR_REASON_FLOW_INVALIDATED ||
            reason === AuthnErrorReason.AUTHN_ERROR_REASON_OIDC_REQUEST_INVALID
          ) {
            try {
              const { flow: fallback } = await api.authn.CreateAuthFlow({})
              if (controller.signal.aborted || !fallback) return
              if (applyAuthFlow(fallback)) return
              setMessageTone("info")
              setMessage(expiredOrInvalidNotice(reason))
              setReady(true)
              return
            } catch (fallbackErr) {
              if (controller.signal.aborted) return
              setLoadError(
                requestErrorMessage(fallbackErr, "无法初始化登录流程，请刷新重试")
              )
              setReady(true)
              return
            }
          }
          setLoadError(
            requestErrorMessage(err, "无法加载登录流程，请刷新页面重试")
          )
          setReady(true)
          return
        }
      }

      if (urlRequestId) {
        try {
          const { flow: created } = await api.authn.CreateAuthFlow({
            oidcRequestId: urlRequestId,
          })
          if (controller.signal.aborted || !created) return
          if (applyAuthFlow(created)) return
          setReady(true)
          return
        } catch (err) {
          if (controller.signal.aborted) return
          if (
            authnErrorReason(err) ===
            AuthnErrorReason.AUTHN_ERROR_REASON_OIDC_REQUEST_INVALID
          ) {
            syncFlowUrl()
            setFlow(null)
            setOidcInvalid(true)
            setLoadError(null)
            setReady(true)
            return
          }
          setLoadError(
            requestErrorMessage(err, "无法初始化授权登录流程，请稍后重试")
          )
          setReady(true)
          return
        }
      }

      try {
        const { flow: created } = await api.authn.CreateAuthFlow({})
        if (controller.signal.aborted || !created) return
        if (applyAuthFlow(created)) return
        setReady(true)
      } catch (err) {
        if (controller.signal.aborted) return
        setLoadError(requestErrorMessage(err, "无法初始化登录流程，请稍后重试"))
        setReady(true)
      }
    }

    void bootstrapFlow()

    const handlePageShow = (event: PageTransitionEvent) => {
      if (!event.persisted) return
      clearSensitiveInputs()
      setReady(false)
      setRevision((current) => current + 1)
    }
    window.addEventListener("pageshow", handlePageShow)

    return () => {
      controller.abort()
      window.removeEventListener("pageshow", handlePageShow)
    }
  }, [applyAuthFlow, clearSensitiveInputs, revision])

  async function recoverFlowAfterError(
    signal: AbortSignal,
    currentFlowId: string,
    reason: AuthnErrorReason | null,
    preferReset: boolean
  ): Promise<{ redirected: boolean; notice: string }> {
    const api = createIamApi(signal)
    if (preferReset && currentFlowId) {
      try {
        const { flow: resetFlow } = await api.authn.ResetAuthFlow({
          flowId: currentFlowId,
        })
        if (resetFlow) {
          const redirected = applyAuthFlow(resetFlow)
          return {
            redirected,
            notice:
              reason === AuthnErrorReason.AUTHN_ERROR_REASON_FLOW_EXPIRED
                ? "单步验证已超时，请重新输入邮箱和密码继续。"
                : "账户或凭据状态已变更，请重新输入凭据登录。",
          }
        }
      } catch {
        // Fall through to standalone CreateAuthFlow fallback.
      }
    }
    const { flow: fresh } = await api.authn.CreateAuthFlow({})
    if (fresh) {
      const redirected = applyAuthFlow(fresh)
      return { redirected, notice: expiredOrInvalidNotice(reason) }
    }
    return { redirected: false, notice: expiredOrInvalidNotice(reason) }
  }

  async function startStandaloneLogin() {
    if (pending) return
    clearSensitiveInputs()
    setLoginErrors({})
    setUpdateErrors({})
    setMessage(null)

    const result = await run((signal) =>
      createIamApi(signal).authn.CreateAuthFlow({})
    )
    if (result.status === "success" && result.value.flow) {
      applyAuthFlow(result.value.flow)
      return
    }
    if (result.status === "failure") {
      setLoadError(
        requestErrorMessage(result.error, "无法创建独立登录流程，请稍后重试")
      )
    }
  }

  async function submitPassword(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (pending || !flow?.flowId) return

    const nextErrors = {
      email: validateEmail(email) ?? undefined,
      password: validatePassword(password.current.current) ?? undefined,
    }
    setLoginErrors(nextErrors)
    setMessage(null)
    if (nextErrors.email || nextErrors.password) return

    const currentFlowId = flow.flowId
    const normalizedEmail = email.trim()
    const submittedPassword = password.current.current

    const result = await run(async (signal) => {
      const api = createIamApi(signal)
      try {
        const response = await api.authn.SubmitPassword({
          flowId: currentFlowId,
          email: normalizedEmail,
          password: submittedPassword,
        })
        return { kind: "ok" as const, flow: response.flow }
      } catch (err) {
        const reason = authnErrorReason(err)
        if (
          reason === AuthnErrorReason.AUTHN_ERROR_REASON_FLOW_EXPIRED ||
          reason === AuthnErrorReason.AUTHN_ERROR_REASON_FLOW_INVALIDATED ||
          reason === AuthnErrorReason.AUTHN_ERROR_REASON_FLOW_NOT_FOUND ||
          reason === AuthnErrorReason.AUTHN_ERROR_REASON_OIDC_REQUEST_INVALID
        ) {
          const recovered = await recoverFlowAfterError(
            signal,
            currentFlowId,
            reason,
            false
          )
          return { kind: "recovered" as const, ...recovered }
        }
        throw err
      }
    })
    password.clear()

    if (result.status === "success") {
      if (result.value.kind === "recovered") {
        if (result.value.redirected) return
        setLoginErrors({})
        setMessageTone("info")
        setMessage(result.value.notice)
        return
      }
      if (result.value.flow) {
        newPassword.clear()
        confirmation.clear()
        setLoginErrors({})
        setUpdateErrors({})
        setMessage(null)
        applyAuthFlow(result.value.flow)
      }
      return
    }
    if (result.status !== "failure") return

    if (
      authnErrorReason(result.error) ===
      AuthnErrorReason.AUTHN_ERROR_REASON_INVALID_CREDENTIALS
    ) {
      setMessageTone("error")
      setMessage("邮箱或密码错误，请检查后重试")
      return
    }
    setMessageTone("error")
    setMessage(requestErrorMessage(result.error, "登录未完成，请重试"))
  }

  async function submitUpdatePassword(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (pending || !flow?.flowId) return

    const nextVal = newPassword.current.current
    const confirmVal = confirmation.current.current
    const nextErrors = {
      newPassword: validatePassword(nextVal, "新密码") ?? undefined,
      confirmation: !confirmVal
        ? "请再次输入新密码"
        : confirmVal !== nextVal
          ? "两次输入的密码不一致"
          : undefined,
    }
    setUpdateErrors(nextErrors)
    setMessage(null)
    if (nextErrors.newPassword || nextErrors.confirmation) return

    const currentFlowId = flow.flowId
    const result = await run(async (signal) => {
      const api = createIamApi(signal)
      try {
        const response = await api.authn.CompleteUpdatePassword({
          flowId: currentFlowId,
          newPassword: nextVal,
        })
        return { kind: "ok" as const, flow: response.flow }
      } catch (err) {
        const reason = authnErrorReason(err)
        if (
          reason === AuthnErrorReason.AUTHN_ERROR_REASON_FLOW_EXPIRED ||
          reason === AuthnErrorReason.AUTHN_ERROR_REASON_FLOW_INVALIDATED ||
          reason === AuthnErrorReason.AUTHN_ERROR_REASON_FLOW_NOT_FOUND
        ) {
          const recovered = await recoverFlowAfterError(
            signal,
            currentFlowId,
            reason,
            reason !== AuthnErrorReason.AUTHN_ERROR_REASON_FLOW_NOT_FOUND
          )
          return { kind: "recovered" as const, ...recovered }
        }
        throw err
      }
    })
    newPassword.clear()
    confirmation.clear()

    if (result.status === "success") {
      if (result.value.kind === "recovered") {
        if (result.value.redirected) return
        setUpdateErrors({})
        setLoginErrors({})
        setMessageTone("error")
        setMessage(result.value.notice)
        return
      }
      if (result.value.flow) {
        setUpdateErrors({})
        setMessage(null)
        applyAuthFlow(result.value.flow)
      }
      return
    }
    if (result.status !== "failure") return

    if (
      authnErrorReason(result.error) ===
      AuthnErrorReason.AUTHN_ERROR_REASON_INVALID_PASSWORD
    ) {
      setMessageTone("error")
      setMessage("新密码须为 8 至 128 个字符，且不能与当前密码相同")
      return
    }
    setMessageTone("error")
    setMessage(
      requestErrorMessage(result.error, "密码更新未完成，请稍后重试")
    )
  }

  async function switchAccount() {
    if (pending || !flow?.flowId) return
    clearSensitiveInputs()
    setLoginErrors({})
    setUpdateErrors({})
    setMessage(null)

    const currentFlowId = flow.flowId
    const result = await run(async (signal) => {
      const api = createIamApi(signal)
      try {
        const { flow: resetFlow } = await api.authn.ResetAuthFlow({
          flowId: currentFlowId,
        })
        return { flow: resetFlow, notice: null as string | null }
      } catch (err) {
        const reason = authnErrorReason(err)
        const { flow: fresh } = await api.authn.CreateAuthFlow({})
        return { flow: fresh, notice: expiredOrInvalidNotice(reason) }
      }
    })

    if (result.status === "success" && result.value.flow) {
      setEmail("")
      if (applyAuthFlow(result.value.flow)) return
      if (result.value.notice) {
        setMessageTone("info")
        setMessage(result.value.notice)
      }
      return
    }
    if (result.status === "failure") {
      setMessageTone("error")
      setMessage(requestErrorMessage(result.error, "无法切换账户，请刷新重试"))
    }
  }

  async function cancelLogin() {
    if (pending || !flow?.flowId) return
    clearSensitiveInputs()
    setLoginErrors({})
    setUpdateErrors({})
    setMessage(null)

    const currentFlowId = flow.flowId
    const hasOidc = Boolean(flow.oidc?.clientId)
    const result = await run(async (signal) => {
      const api = createIamApi(signal)
      const cancelled = await api.authn.CancelAuthFlow({
        flowId: currentFlowId,
      })
      if (cancelled.redirectUri) {
        return { redirectUri: cancelled.redirectUri, nextFlow: null }
      }
      const { flow: fresh } = await api.authn.CreateAuthFlow({})
      return { redirectUri: "", nextFlow: fresh ?? null }
    })

    if (result.status === "success") {
      if (result.value.redirectUri) {
        window.location.replace(result.value.redirectUri)
        return
      }
      setEmail("")
      if (result.value.nextFlow) {
        applyAuthFlow(result.value.nextFlow)
      }
      if (hasOidc) {
        setMessageTone("info")
        setMessage("已取消原应用授权登录。")
      }
      return
    }
    if (result.status === "failure") {
      const reason = authnErrorReason(result.error)
      if (
        reason === AuthnErrorReason.AUTHN_ERROR_REASON_FLOW_EXPIRED ||
        reason === AuthnErrorReason.AUTHN_ERROR_REASON_FLOW_NOT_FOUND ||
        reason === AuthnErrorReason.AUTHN_ERROR_REASON_FLOW_INVALIDATED
      ) {
        await startStandaloneLogin()
        return
      }
      setMessageTone("error")
      setMessage(requestErrorMessage(result.error, "取消登录未完成，请重试"))
    }
  }

  if (!ready) {
    return (
      <AuthShell title="登录" description="正在准备安全登录…">
        <p className="text-sm text-muted-foreground" role="status">
          请稍候
        </p>
      </AuthShell>
    )
  }

  if (oidcInvalid) {
    return (
      <AuthShell
        title="授权请求已失效"
        description="当前应用发起的登录握手不存在、已完成或已过期。"
      >
        <div className="grid gap-5">
          <StatusMessage tone="info">
            请返回发起登录的应用重新点击登录；如果只需管理个人账户，可直接前往独立登录。
          </StatusMessage>
          <Button
            type="button"
            size="lg"
            className="h-10 w-full"
            disabled={pending}
            onClick={startStandaloneLogin}
          >
            前往独立登录
          </Button>
        </div>
      </AuthShell>
    )
  }

  if (loadError || !flow) {
    return (
      <AuthShell title="登录" description="暂时无法启动登录流程。">
        <div className="grid gap-5">
          <StatusMessage tone="error">
            {loadError || "登录流程状态异常，请重新尝试。"}
          </StatusMessage>
          <Button
            type="button"
            size="lg"
            className="h-10 w-full"
            disabled={pending}
            onClick={() => {
              setLoadError(null)
              setReady(false)
              setRevision((current) => current + 1)
            }}
          >
            重试
          </Button>
        </div>
      </AuthShell>
    )
  }

  const oidcClientId = flow.oidc?.clientId

  if (flow.state === "AUTH_FLOW_STATE_REQUIRED_ACTION") {
    const isUpdatePassword =
      flow.nextRequiredAction === "USER_REQUIRED_ACTION_UPDATE_PASSWORD"

    if (!isUpdatePassword) {
      return (
        <AuthShell
          title="需要完成账户操作"
          description="当前账户存在待处理的登录要求。"
        >
          <div className="grid gap-4">
            <StatusMessage tone="error">
              当前账户包含暂不支持在此页面完成的登录要求，请联系管理员处理。
            </StatusMessage>
            <Button
              type="button"
              variant="outline"
              size="lg"
              className="h-10 w-full"
              disabled={pending}
              onClick={switchAccount}
            >
              切换账号
            </Button>
            <Button
              type="button"
              variant="ghost"
              size="lg"
              className="h-10 w-full"
              disabled={pending}
              onClick={cancelLogin}
            >
              取消登录
            </Button>
          </div>
        </AuthShell>
      )
    }

    return (
      <AuthShell
        title="修改初始密码"
        description={
          oidcClientId
            ? `当前账户需先设置新密码，完成后将继续授权访问 ${oidcClientId}。`
            : "为保障账户安全，首次登录需设置新密码。"
        }
      >
        <form
          key={`update-password-${flow.flowId}`}
          className="grid gap-5"
          onSubmit={submitUpdatePassword}
          noValidate
          aria-busy={pending}
        >
          {message ? (
            <StatusMessage tone={messageTone}>{message}</StatusMessage>
          ) : null}
          {flow.email ? (
            <div className="grid gap-1.5 rounded-lg border bg-muted/40 px-3.5 py-2.5">
              <span className="text-xs text-muted-foreground">当前登录邮箱</span>
              <span className="truncate text-sm font-medium text-foreground">
                {flow.email}
              </span>
            </div>
          ) : null}
          <PasswordField
            label="新密码"
            name="new-password"
            autoComplete="new-password"
            value={newPassword.value}
            error={updateErrors.newPassword}
            hint="使用 8 至 128 个字符，且不能与当前密码相同"
            disabled={pending}
            onChange={(event) => newPassword.setValue(event.target.value)}
            required
          />
          <PasswordField
            label="确认新密码"
            name="new-password-confirmation"
            autoComplete="new-password"
            value={confirmation.value}
            error={updateErrors.confirmation}
            disabled={pending}
            onChange={(event) => confirmation.setValue(event.target.value)}
            required
          />
          <div className="grid gap-3">
            <SubmitButton pending={pending}>更新密码并继续</SubmitButton>
            <div className="grid grid-cols-2 gap-3">
              <Button
                type="button"
                variant="outline"
                size="lg"
                className="h-10 w-full"
                disabled={pending}
                onClick={switchAccount}
              >
                切换账号
              </Button>
              <Button
                type="button"
                variant="ghost"
                size="lg"
                className="h-10 w-full"
                disabled={pending}
                onClick={cancelLogin}
              >
                取消登录
              </Button>
            </div>
          </div>
        </form>
      </AuthShell>
    )
  }

  return (
    <AuthShell
      title="登录"
      description={
        oidcClientId
          ? `登录后将继续授权访问 ${oidcClientId}。`
          : "使用您的邮箱和密码继续。"
      }
      footer={
        <>
          还没有账户？{" "}
          <Link
            href="/register/"
            className="text-foreground underline underline-offset-4"
          >
            创建账户
          </Link>
        </>
      }
    >
      <form
        key={`authenticate-${flow.flowId}`}
        className="grid gap-5"
        onSubmit={submitPassword}
        noValidate
        aria-busy={pending}
      >
        {message ? (
          <StatusMessage tone={messageTone}>{message}</StatusMessage>
        ) : null}
        <TextField
          label="邮箱"
          name="email"
          type="email"
          inputMode="email"
          autoComplete="email"
          value={email}
          error={loginErrors.email}
          disabled={pending}
          onChange={(event) => setEmail(event.target.value)}
          required
        />
        <PasswordField
          label="密码"
          name="password"
          autoComplete="current-password"
          value={password.value}
          error={loginErrors.password}
          disabled={pending}
          onChange={(event) => password.setValue(event.target.value)}
          required
        />
        <div className="flex justify-end">
          <Link
            href="/forgot-password/"
            className="rounded text-sm underline underline-offset-4 outline-none focus-visible:ring-3 focus-visible:ring-ring"
          >
            忘记密码？
          </Link>
        </div>
        <div className="grid gap-3">
          <SubmitButton pending={pending}>登录</SubmitButton>
          {oidcClientId ? (
            <Button
              type="button"
              variant="outline"
              size="lg"
              className="h-10 w-full"
              disabled={pending}
              onClick={cancelLogin}
            >
              取消登录
            </Button>
          ) : null}
        </div>
      </form>
      <p className="mt-5 text-sm leading-6 text-muted-foreground">
        {oidcClientId
          ? "取消登录将安全返回原应用。"
          : "如果需要登录其他应用，请从目标应用发起登录。"}
      </p>
    </AuthShell>
  )
}
