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
import { useOneTimeRequestId } from "@/hooks/use-one-time-url"
import { useSensitiveValue } from "@/hooks/use-sensitive-value"
import { useSingleFlight } from "@/hooks/use-single-flight"
import { requestErrorMessage } from "@/lib/errors"
import { createIamApi } from "@/lib/iam-api"
import { validateEmail, validatePassword } from "@/lib/validation"

export default function LoginPage() {
  const requestId = useOneTimeRequestId()
  const password = useSensitiveValue()
  const { pending, run } = useSingleFlight()
  const [email, setEmail] = useState("")
  const [errors, setErrors] = useState<{ email?: string; password?: string }>(
    {}
  )
  const [message, setMessage] = useState<string | null>(null)

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (pending) return

    const nextErrors = {
      email: validateEmail(email) ?? undefined,
      password: validatePassword(password.current.current) ?? undefined,
    }
    setErrors(nextErrors)
    setMessage(null)
    if (nextErrors.email || nextErrors.password) return

    const submittedPassword = password.current.current
    const result = await run((signal) =>
      createIamApi(signal).authn.Login({
        email: email.trim(),
        password: submittedPassword,
      })
    )
    password.clear()

    if (result.status === "success") {
      if (requestId.value) {
        window.location.assign(
          `/authorize/callback?id=${encodeURIComponent(requestId.value)}`
        )
      } else {
        window.location.assign("/account/")
      }
      return
    }
    if (result.status !== "failure") return

    const infrastructureMessage = requestErrorMessage(result.error, "")
    setMessage(infrastructureMessage || "邮箱或密码错误，请检查后重试")
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
        className="grid gap-5"
        onSubmit={submit}
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
