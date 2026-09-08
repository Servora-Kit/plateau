"use client"

import Link from "next/link"
import { useState, type FormEvent } from "react"

import { AuthShell } from "@/components/auth-shell"
import { PasswordField, SubmitButton } from "@/components/form-controls"
import { StatusMessage } from "@/components/status-message"
import { useOneTimeFragment } from "@/hooks/use-one-time-url"
import { useSensitiveValue } from "@/hooks/use-sensitive-value"
import { useSingleFlight } from "@/hooks/use-single-flight"
import {
  isInvalidOneTimeToken,
  isInvalidPassword,
  requestErrorMessage,
} from "@/lib/errors"
import { createIamApi } from "@/lib/iam-api"
import { validatePassword } from "@/lib/validation"

export default function ResetPasswordPage() {
  const token = useOneTimeFragment()

  if (!token.ready) {
    return (
      <AuthShell title="重置密码" description="正在读取重置链接…">
        <p className="text-sm text-muted-foreground" role="status">
          请稍候
        </p>
      </AuthShell>
    )
  }

  return (
    <ResetPasswordFlow initialToken={token.value} onConsume={token.clear} />
  )
}

function ResetPasswordFlow({
  initialToken,
  onConsume,
}: {
  initialToken: string | null
  onConsume(): void
}) {
  const password = useSensitiveValue()
  const confirmation = useSensitiveValue()
  const { pending, run } = useSingleFlight()
  const [token, setToken] = useState(initialToken)
  const [complete, setComplete] = useState(false)
  const [message, setMessage] = useState<string | null>(
    initialToken ? null : "重置链接缺少必要信息，请重新申请。"
  )
  const [errors, setErrors] = useState<{
    password?: string
    confirmation?: string
  }>({})

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (pending || !token) return

    const nextErrors = {
      password:
        validatePassword(password.current.current, "新密码") ?? undefined,
      confirmation: !confirmation.current.current
        ? "请再次输入新密码"
        : confirmation.current.current !== password.current.current
          ? "两次输入的密码不一致"
          : undefined,
    }
    setErrors(nextErrors)
    setMessage(null)
    if (nextErrors.password || nextErrors.confirmation) return

    const submittedToken = token
    const submittedPassword = password.current.current
    setToken(null)
    onConsume()
    const result = await run((signal) =>
      createIamApi(signal).account.ConfirmPasswordReset({
        token: submittedToken,
        newPassword: submittedPassword,
      })
    )
    password.clear()
    confirmation.clear()

    if (result.status === "success") {
      setComplete(true)
      return
    }
    if (result.status !== "failure") return
    if (isInvalidOneTimeToken(result.error)) {
      setMessage("重置链接已失效或已使用，请重新申请。")
      return
    }
    if (isInvalidPassword(result.error)) {
      setErrors({ password: "新密码不符合要求，请使用 8 至 128 个字符" })
      return
    }
    setMessage(
      requestErrorMessage(
        result.error,
        "无法确认重置结果。请尝试登录；若仍无法登录，请重新申请。"
      )
    )
  }

  if (complete) {
    return (
      <AuthShell title="密码已重置" description="请使用新密码重新登录。">
        <StatusMessage tone="success">
          密码设置成功，原有会话已失效。
        </StatusMessage>
        <Link
          href="/login/"
          className="mt-5 flex h-10 items-center justify-center rounded-lg bg-primary text-sm font-medium text-primary-foreground"
        >
          前往登录
        </Link>
      </AuthShell>
    )
  }

  if (pending && !token) {
    return (
      <AuthShell title="重置密码" description="正在提交新密码。">
        <StatusMessage>正在重置密码，请勿关闭页面。</StatusMessage>
      </AuthShell>
    )
  }

  if (!token) {
    return (
      <AuthShell title="重置密码" description="此一次性链接当前不可用。">
        {message ? <StatusMessage tone="error">{message}</StatusMessage> : null}
        <div className="mt-5 grid gap-3">
          <Link
            href="/forgot-password/"
            className="flex h-10 items-center justify-center rounded-lg bg-primary text-sm font-medium text-primary-foreground"
          >
            重新申请重置邮件
          </Link>
          <Link
            href="/login/"
            className="flex h-10 items-center justify-center rounded-lg border text-sm font-medium"
          >
            尝试登录
          </Link>
        </div>
      </AuthShell>
    )
  }

  return (
    <AuthShell title="设置新密码" description="设置完成后，您需要重新登录。">
      <form
        className="grid gap-5"
        onSubmit={submit}
        noValidate
        aria-busy={pending}
      >
        {message ? <StatusMessage tone="error">{message}</StatusMessage> : null}
        <PasswordField
          label="新密码"
          name="new-password"
          autoComplete="new-password"
          value={password.value}
          error={errors.password}
          hint="使用 8 至 128 个字符"
          disabled={pending}
          onChange={(event) => password.setValue(event.target.value)}
          required
        />
        <PasswordField
          label="确认新密码"
          name="new-password-confirmation"
          autoComplete="new-password"
          value={confirmation.value}
          error={errors.confirmation}
          disabled={pending}
          onChange={(event) => confirmation.setValue(event.target.value)}
          required
        />
        <SubmitButton pending={pending}>重置密码</SubmitButton>
      </form>
    </AuthShell>
  )
}
