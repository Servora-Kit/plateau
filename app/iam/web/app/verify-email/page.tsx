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
import { useOneTimeFragment } from "@/hooks/use-one-time-url"
import { useSingleFlight } from "@/hooks/use-single-flight"
import { isInvalidOneTimeToken, requestErrorMessage } from "@/lib/errors"
import { createIamApi } from "@/lib/iam-api"
import { validateEmail } from "@/lib/validation"

export default function VerifyEmailPage() {
  const token = useOneTimeFragment()

  if (!token.ready) {
    return (
      <AuthShell title="验证邮箱" description="正在读取验证链接…">
        <p className="text-sm text-muted-foreground" role="status">
          请稍候
        </p>
      </AuthShell>
    )
  }

  return <VerifyEmailFlow initialToken={token.value} onConsume={token.clear} />
}

function VerifyEmailFlow({
  initialToken,
  onConsume,
}: {
  initialToken: string | null
  onConsume(): void
}) {
  const { pending, run } = useSingleFlight()
  const [token, setToken] = useState(initialToken)
  const [verified, setVerified] = useState(false)
  const [message, setMessage] = useState<string | null>(
    initialToken ? null : "验证链接缺少必要信息，请重新发送验证邮件。"
  )

  async function verify() {
    if (pending || !token) return
    const submittedToken = token
    setToken(null)
    onConsume()
    setMessage(null)
    const result = await run((signal) =>
      createIamApi(signal).account.VerifyEmail({ token: submittedToken })
    )

    if (result.status === "success") {
      setVerified(true)
      return
    }
    if (result.status !== "failure") return
    if (isInvalidOneTimeToken(result.error)) {
      setMessage("验证链接已失效或已使用，请重新发送验证邮件。")
      return
    }
    setMessage(
      requestErrorMessage(
        result.error,
        "无法确认验证结果。您可以尝试登录，或重新发送验证邮件。"
      )
    )
  }

  if (verified) {
    return (
      <AuthShell title="邮箱验证完成" description="您的账户现在可以登录。">
        <StatusMessage tone="success">邮箱已成功验证。</StatusMessage>
        <Link
          href="/login/"
          className="mt-5 flex h-10 items-center justify-center rounded-lg bg-primary text-sm font-medium text-primary-foreground"
        >
          前往登录
        </Link>
      </AuthShell>
    )
  }

  return (
    <AuthShell
      title="验证邮箱"
      description={
        pending
          ? "正在确认此一次性验证链接。"
          : token
            ? "确认使用此一次性链接验证您的邮箱。"
            : "重新发送一封验证邮件。"
      }
      footer={
        <Link
          href="/login/"
          className="text-foreground underline underline-offset-4"
        >
          返回登录
        </Link>
      }
    >
      <div className="grid gap-5">
        {message ? (
          <StatusMessage tone={token ? "info" : "error"}>
            {message}
          </StatusMessage>
        ) : null}
        {pending ? (
          <StatusMessage>正在验证邮箱，请勿关闭页面。</StatusMessage>
        ) : token ? (
          <SubmitButton pending={false} onClick={() => void verify()}>
            确认验证邮箱
          </SubmitButton>
        ) : (
          <ResendVerificationForm />
        )}
      </div>
    </AuthShell>
  )
}

function ResendVerificationForm() {
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
      createIamApi(signal).account.ResendVerificationEmail({
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
      <StatusMessage tone="success">
        如果账户需要验证，我们已经发送了邮件。请检查收件箱。
      </StatusMessage>
    )
  }

  return (
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
        发送验证邮件
      </SubmitButton>
    </form>
  )
}
