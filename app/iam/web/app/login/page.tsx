"use client"

import Link from "next/link"
import { useState, type FormEvent } from "react"

import { AuthShell } from "@/components/auth-shell"
import {
  PasswordField,
  SubmitButton,
  TextField,
} from "@/components/form-controls"
import { StatusMessage } from "@/components/status-message"
import { Button } from "@/components/ui/button"
import { useOneTimeRequestId } from "@/hooks/use-one-time-url"
import { useSensitiveValue } from "@/hooks/use-sensitive-value"
import { useSingleFlight } from "@/hooks/use-single-flight"
import {
  isAccountInvalidState,
  isInvalidPassword,
  isUnauthenticated,
  requestErrorMessage,
} from "@/lib/errors"
import { createIamApi } from "@/lib/iam-api"
import { validateEmail, validatePassword } from "@/lib/validation"

function navigateAfterAuth(requestId: string | null) {
  if (requestId) {
    window.location.assign(
      `/authorize/callback?id=${encodeURIComponent(requestId)}`
    )
  } else {
    window.location.assign("/account/")
  }
}

export default function LoginPage() {
  const requestId = useOneTimeRequestId()
  const password = useSensitiveValue()
  const initialCurrentPassword = useSensitiveValue()
  const initialNewPassword = useSensitiveValue()
  const initialConfirmation = useSensitiveValue()
  const { pending, run } = useSingleFlight()
  const [step, setStep] = useState<"login" | "change-initial-password">("login")
  const [email, setEmail] = useState("")
  const [errors, setErrors] = useState<{ email?: string; password?: string }>(
    {}
  )
  const [initialErrors, setInitialErrors] = useState<{
    email?: string
    currentPassword?: string
    newPassword?: string
    confirmation?: string
  }>({})
  const [message, setMessage] = useState<string | null>(null)

  async function submitLogin(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (pending) return

    const nextErrors = {
      email: validateEmail(email) ?? undefined,
      password: validatePassword(password.current.current) ?? undefined,
    }
    setErrors(nextErrors)
    setMessage(null)
    if (nextErrors.email || nextErrors.password) return

    const normalizedEmail = email.trim()
    const submittedPassword = password.current.current
    const result = await run((signal) =>
      createIamApi(signal).authn.Login({
        email: normalizedEmail,
        password: submittedPassword,
      })
    )
    password.clear()

    if (result.status === "success") {
      const requiredActions = result.value.user?.requiredActions ?? []
      if (requiredActions.length === 0) {
        navigateAfterAuth(requestId.value)
        return
      }

      const hasUnsupportedAction = requiredActions.some(
        (action) => action !== "USER_REQUIRED_ACTION_UPDATE_PASSWORD"
      )
      if (hasUnsupportedAction) {
        setMessage("当前账户包含未支持的登录要求，请联系管理员处理")
        return
      }

      initialCurrentPassword.clear()
      initialNewPassword.clear()
      initialConfirmation.clear()
      setEmail(normalizedEmail)
      setErrors({})
      setInitialErrors({})
      setMessage(null)
      setStep("change-initial-password")
      return
    }
    if (result.status !== "failure") return

    const infrastructureMessage = requestErrorMessage(result.error, "")
    setMessage(infrastructureMessage || "邮箱或密码错误，请检查后重试")
  }

  async function submitInitialPassword(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (pending) return

    const currentVal = initialCurrentPassword.current.current
    const nextVal = initialNewPassword.current.current
    const confirmVal = initialConfirmation.current.current
    const newPasswordFormatError = validatePassword(nextVal, "新密码")
    const nextErrors = {
      email: validateEmail(email) ?? undefined,
      currentPassword: validatePassword(currentVal, "初始密码") ?? undefined,
      newPassword:
        newPasswordFormatError ??
        (nextVal === currentVal ? "新密码不能与初始密码相同" : undefined),
      confirmation: !confirmVal
        ? "请再次输入新密码"
        : confirmVal !== nextVal
          ? "两次输入的密码不一致"
          : undefined,
    }
    setInitialErrors(nextErrors)
    setMessage(null)
    if (
      nextErrors.email ||
      nextErrors.currentPassword ||
      nextErrors.newPassword ||
      nextErrors.confirmation
    ) {
      return
    }

    const normalizedEmail = email.trim()
    const result = await run((signal) =>
      createIamApi(signal).account.ChangeInitialPassword({
        email: normalizedEmail,
        currentPassword: currentVal,
        newPassword: nextVal,
      })
    )
    initialCurrentPassword.clear()
    initialNewPassword.clear()
    initialConfirmation.clear()

    if (result.status === "success") {
      navigateAfterAuth(requestId.value)
      return
    }
    if (result.status !== "failure") return

    if (isUnauthenticated(result.error)) {
      setMessage("邮箱、初始密码不正确或账户状态暂不可用，请核对后重试")
      return
    }
    if (isInvalidPassword(result.error)) {
      setMessage("新密码须为 8 至 128 个字符，且不能与初始密码相同")
      return
    }
    if (isAccountInvalidState(result.error)) {
      password.clear()
      setInitialErrors({})
      setErrors({})
      setStep("login")
      setMessage("账户或凭据状态已更新，请重新登录")
      return
    }
    setMessage(
      requestErrorMessage(
        result.error,
        "无法确认登录结果，请尝试使用新密码重新登录"
      )
    )
  }

  function cancelInitialPassword() {
    if (pending) return
    password.clear()
    initialCurrentPassword.clear()
    initialNewPassword.clear()
    initialConfirmation.clear()
    setInitialErrors({})
    setErrors({})
    setMessage(null)
    setStep("login")
  }

  if (!requestId.ready) {
    return (
      <AuthShell title="登录" description="正在准备安全登录…">
        <p className="text-sm text-muted-foreground" role="status">
          请稍候
        </p>
      </AuthShell>
    )
  }

  if (step === "change-initial-password") {
    return (
      <AuthShell
        title="修改初始密码"
        description={
          requestId.value
            ? "当前账户需先修改初始密码，完成后将继续返回发起授权的应用。"
            : "当前账户需先修改初始密码，请重新输入初始密码并设置新密码。"
        }
      >
        <form
          key="change-initial-password"
          className="grid gap-5"
          onSubmit={submitInitialPassword}
          noValidate
          aria-busy={pending}
        >
          {message ? (
            <StatusMessage tone="error">{message}</StatusMessage>
          ) : null}
          <TextField
            label="邮箱"
            name="email"
            type="email"
            inputMode="email"
            autoComplete="email"
            value={email}
            error={initialErrors.email}
            disabled={pending}
            onChange={(event) => setEmail(event.target.value)}
            required
          />
          <PasswordField
            label="初始密码"
            name="current-password"
            autoComplete="current-password"
            value={initialCurrentPassword.value}
            error={initialErrors.currentPassword}
            disabled={pending}
            onChange={(event) =>
              initialCurrentPassword.setValue(event.target.value)
            }
            required
          />
          <PasswordField
            label="新密码"
            name="new-password"
            autoComplete="new-password"
            value={initialNewPassword.value}
            error={initialErrors.newPassword}
            hint="使用 8 至 128 个字符，且不能与初始密码相同"
            disabled={pending}
            onChange={(event) =>
              initialNewPassword.setValue(event.target.value)
            }
            required
          />
          <PasswordField
            label="确认新密码"
            name="new-password-confirmation"
            autoComplete="new-password"
            value={initialConfirmation.value}
            error={initialErrors.confirmation}
            disabled={pending}
            onChange={(event) =>
              initialConfirmation.setValue(event.target.value)
            }
            required
          />
          <div className="grid gap-3">
            <SubmitButton pending={pending}>更新密码并登录</SubmitButton>
            <Button
              type="button"
              variant="outline"
              size="lg"
              className="h-10 w-full"
              disabled={pending}
              onClick={cancelInitialPassword}
            >
              取消返回登录
            </Button>
          </div>
        </form>
      </AuthShell>
    )
  }

  return (
    <AuthShell
      title="登录"
      description={
        requestId.value
          ? "登录后将返回发起授权的应用。"
          : "使用您的邮箱和密码继续。"
      }
      footer={
        <>
          还没有账户？
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
        key="login"
        className="grid gap-5"
        onSubmit={submitLogin}
        noValidate
        aria-busy={pending}
      >
        {message ? <StatusMessage tone="error">{message}</StatusMessage> : null}
        <TextField
          label="邮箱"
          name="email"
          type="email"
          inputMode="email"
          autoComplete="email"
          value={email}
          error={errors.email}
          disabled={pending}
          onChange={(event) => setEmail(event.target.value)}
          required
        />
        <PasswordField
          label="密码"
          name="password"
          autoComplete="current-password"
          value={password.value}
          error={errors.password}
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
        <SubmitButton pending={pending}>登录</SubmitButton>
      </form>
      <p className="mt-5 text-sm leading-6 text-muted-foreground">
        {requestId.value
          ? "若授权流程已失效，请返回原应用重新开始登录。"
          : "如果需要登录其他应用，请返回该应用重新开始登录。"}
      </p>
    </AuthShell>
  )
}
