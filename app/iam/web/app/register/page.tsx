"use client"

import Link from "next/link"
import { useRef, useState, type FormEvent } from "react"

import { AuthShell } from "@/components/auth-shell"
import {
  CapVerification,
  type CapVerificationHandle,
} from "@/components/cap-verification"
import {
  PasswordField,
  SubmitButton,
  TextField,
} from "@/components/form-controls"
import { StatusMessage } from "@/components/status-message"
import { useSensitiveValue } from "@/hooks/use-sensitive-value"
import { useSingleFlight } from "@/hooks/use-single-flight"
import {
  isAlreadyRegistered,
  isInvalidPassword,
  requestErrorMessage,
} from "@/lib/errors"
import { createIamApi } from "@/lib/iam-api"
import { validateEmail, validatePassword } from "@/lib/validation"

export default function RegisterPage() {
  const password = useSensitiveValue()
  const confirmation = useSensitiveValue()
  const { pending, run } = useSingleFlight()
  const capRef = useRef<CapVerificationHandle>(null)
  const [email, setEmail] = useState("")
  const [name, setName] = useState("")
  const [capToken, setCapToken] = useState<string | null>(null)
  const [complete, setComplete] = useState(false)
  const [message, setMessage] = useState<string | null>(null)
  const [errors, setErrors] = useState<{
    email?: string
    password?: string
    confirmation?: string
  }>({})

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (pending) return

    const passwordError = validatePassword(password.current.current)
    const nextErrors = {
      email: validateEmail(email) ?? undefined,
      password: passwordError ?? undefined,
      confirmation: !confirmation.current.current
        ? "请再次输入密码"
        : confirmation.current.current !== password.current.current
          ? "两次输入的密码不一致"
          : undefined,
    }
    setErrors(nextErrors)
    setMessage(null)
    if (nextErrors.email || nextErrors.password || nextErrors.confirmation)
      return
    if (!capToken) {
      setMessage("请先完成人机验证")
      return
    }

    const submittedToken = capToken
    const submittedPassword = password.current.current
    setCapToken(null)
    const result = await run((signal) =>
      createIamApi(signal).account.Register({
        email: email.trim(),
        password: submittedPassword,
        profile: name.trim() ? { name: name.trim() } : {},
        capToken: submittedToken,
      })
    )
    password.clear()
    confirmation.clear()
    capRef.current?.reset()

    if (
      result.status === "success" ||
      (result.status === "failure" && isAlreadyRegistered(result.error))
    ) {
      setComplete(true)
      return
    }
    if (result.status !== "failure") return
    if (isInvalidPassword(result.error)) {
      setErrors({ password: "密码不符合要求，请使用 8 至 128 个字符" })
      return
    }
    setMessage(
      requestErrorMessage(result.error, "注册请求未完成，请重新验证后重试")
    )
  }

  if (complete) {
    return (
      <AuthShell
        title="检查您的邮箱"
        description="如果可以创建账户，我们已经发送了验证邮件。"
      >
        <StatusMessage tone="success">
          请打开邮件中的链接完成邮箱验证，然后返回登录。
        </StatusMessage>
        <Link
          href="/login/"
          className="mt-5 flex h-10 items-center justify-center rounded-lg bg-primary text-sm font-medium text-primary-foreground"
        >
          返回登录
        </Link>
      </AuthShell>
    )
  }

  return (
    <AuthShell
      title="创建账户"
      description="填写账户信息并完成人机验证。"
      footer={
        <>
          已有账户？
          <Link
            href="/login/"
            className="text-foreground underline underline-offset-4"
          >
            返回登录
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
        <TextField
          label="姓名（选填）"
          name="name"
          autoComplete="name"
          value={name}
          disabled={pending}
          onChange={(event) => setName(event.target.value)}
        />
        <PasswordField
          label="密码"
          name="password"
          autoComplete="new-password"
          value={password.value}
          error={errors.password}
          hint="使用 8 至 128 个字符"
          disabled={pending}
          onChange={(event) => password.setValue(event.target.value)}
          required
        />
        <PasswordField
          label="确认密码"
          name="password-confirmation"
          autoComplete="new-password"
          value={confirmation.value}
          error={errors.confirmation}
          disabled={pending}
          onChange={(event) => confirmation.setValue(event.target.value)}
          required
        />
        <CapVerification
          ref={capRef}
          busy={pending}
          onTokenChange={setCapToken}
        />
        <SubmitButton pending={pending} disabled={!capToken}>
          创建账户
        </SubmitButton>
      </form>
    </AuthShell>
  )
}
