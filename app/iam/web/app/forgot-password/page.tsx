"use client"

import Link from "next/link"
import { useRef, useState, type FormEvent } from "react"

import { AuthShell } from "@/components/auth-shell"
import {
  CapVerification,
  type CapVerificationHandle,
} from "@/components/cap-verification"
import { SubmitButton, TextField } from "@/components/form-controls"
import { StatusMessage } from "@/components/status-message"
import { useSingleFlight } from "@/hooks/use-single-flight"
import { requestErrorMessage } from "@/lib/errors"
import { createIamApi } from "@/lib/iam-api"
import { validateEmail } from "@/lib/validation"

export default function ForgotPasswordPage() {
  const { pending, run } = useSingleFlight()
  const capRef = useRef<CapVerificationHandle>(null)
  const [email, setEmail] = useState("")
  const [emailError, setEmailError] = useState<string | null>(null)
  const [capToken, setCapToken] = useState<string | null>(null)
  const [message, setMessage] = useState<string | null>(null)
  const [complete, setComplete] = useState(false)

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (pending) return

    const nextEmailError = validateEmail(email)
    setEmailError(nextEmailError)
    setMessage(null)
    if (nextEmailError) return
    if (!capToken) {
      setMessage("请先完成人机验证")
      return
    }

    const submittedToken = capToken
    setCapToken(null)
    const result = await run((signal) =>
      createIamApi(signal).account.RequestPasswordReset({
        email: email.trim(),
        capToken: submittedToken,
      })
    )
    capRef.current?.reset()

    if (result.status === "success") {
      setComplete(true)
      return
    }
    if (result.status !== "failure") return
    setMessage(
      requestErrorMessage(result.error, "请求未完成，请重新验证后重试")
    )
  }

  if (complete) {
    return (
      <AuthShell
        title="检查您的邮箱"
        description="如果账户符合条件，我们已经发送了密码重置邮件。"
      >
        <StatusMessage tone="success">
          请按照邮件中的说明设置新密码。没有收到邮件时，请稍后再次尝试。
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
      title="找回密码"
      description="输入账户邮箱，我们会在符合条件时发送重置邮件。"
      footer={
        <Link
          href="/login/"
          className="text-foreground underline underline-offset-4"
        >
          返回登录
        </Link>
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
          error={emailError}
          disabled={pending}
          onChange={(event) => setEmail(event.target.value)}
          required
        />
        <CapVerification
          ref={capRef}
          busy={pending}
          onTokenChange={setCapToken}
        />
        <SubmitButton pending={pending} disabled={!capToken}>
          发送重置邮件
        </SubmitButton>
      </form>
    </AuthShell>
  )
}
