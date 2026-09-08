"use client"

import { LoaderCircle, LogOut } from "lucide-react"
import { useState, type FormEvent } from "react"
import { useRouter } from "next/navigation"

import { AccountShell } from "@/components/account-shell"
import { PasswordField, SubmitButton } from "@/components/form-controls"
import { StatusMessage } from "@/components/status-message"
import { Button } from "@/components/ui/button"
import { useProfile } from "@/hooks/use-profile"
import { useSensitiveValue } from "@/hooks/use-sensitive-value"
import { useSingleFlight } from "@/hooks/use-single-flight"
import {
  isInvalidPassword,
  isUnauthenticated,
  requestErrorMessage,
} from "@/lib/errors"
import { createIamApi } from "@/lib/iam-api"
import { validatePassword } from "@/lib/validation"

export default function AccountSecurityPage() {
  const router = useRouter()
  const { state, reload } = useProfile()
  const currentPassword = useSensitiveValue()
  const newPassword = useSensitiveValue()
  const confirmation = useSensitiveValue()
  const { pending, run } = useSingleFlight()
  const [errors, setErrors] = useState<{
    current?: string
    next?: string
    confirmation?: string
  }>({})
  const [message, setMessage] = useState<{
    tone: "error" | "success"
    text: string
  } | null>(null)

  async function changePassword(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (pending) return

    const nextErrors = {
      current:
        validatePassword(currentPassword.current.current, "当前密码") ??
        undefined,
      next:
        validatePassword(newPassword.current.current, "新密码") ?? undefined,
      confirmation: !confirmation.current.current
        ? "请再次输入新密码"
        : confirmation.current.current !== newPassword.current.current
          ? "两次输入的新密码不一致"
          : undefined,
    }
    setErrors(nextErrors)
    setMessage(null)
    if (nextErrors.current || nextErrors.next || nextErrors.confirmation) return

    const submittedCurrent = currentPassword.current.current
    const submittedNext = newPassword.current.current
    const result = await run((signal) =>
      createIamApi(signal).account.ChangePassword({
        currentPassword: submittedCurrent,
        newPassword: submittedNext,
      })
    )
    currentPassword.clear()
    newPassword.clear()
    confirmation.clear()

    if (result.status === "success") {
      setMessage({ tone: "success", text: "密码已更新，当前登录保持有效。" })
      return
    }
    if (result.status !== "failure") return
    if (isUnauthenticated(result.error)) {
      router.replace("/login/")
      return
    }
    if (isInvalidPassword(result.error)) {
      setMessage({
        tone: "error",
        text: "当前密码不正确，或新密码不符合要求。",
      })
      return
    }
    setMessage({
      tone: "error",
      text: requestErrorMessage(result.error, "密码未更新，请重新输入后重试"),
    })
  }

  async function logout() {
    if (pending) return
    setMessage(null)
    const result = await run((signal) =>
      createIamApi(signal).session.Logout({})
    )
    currentPassword.clear()
    newPassword.clear()
    confirmation.clear()

    if (result.status === "success") {
      window.location.assign("/login/")
      return
    }
    if (result.status !== "failure") return
    if (isUnauthenticated(result.error)) {
      window.location.assign("/login/")
      return
    }
    setMessage({
      tone: "error",
      text: requestErrorMessage(
        result.error,
        "无法确认是否已退出，请保持此页面并重试"
      ),
    })
  }

  if (state.status === "loading" || state.status === "unauthenticated") {
    return (
      <AccountShell title="账户安全" description="更新密码或结束当前登录。">
        <p className="text-sm text-muted-foreground" role="status">
          正在确认账户状态…
        </p>
      </AccountShell>
    )
  }

  if (state.status === "error") {
    return (
      <AccountShell title="账户安全" description="更新密码或结束当前登录。">
        <StatusMessage tone="error">{state.message}</StatusMessage>
        <Button className="mt-4 h-10" onClick={() => void reload()}>
          重新加载
        </Button>
      </AccountShell>
    )
  }

  return (
    <AccountShell
      title="账户安全"
      description="更新密码或结束当前登录。"
      email={state.user.email}
    >
      <div className="grid gap-8">
        {message ? (
          <StatusMessage tone={message.tone}>{message.text}</StatusMessage>
        ) : null}
        <section
          className="rounded-xl border p-5 sm:p-6"
          aria-labelledby="password-heading"
        >
          <h2 id="password-heading" className="text-lg font-semibold">
            修改密码
          </h2>
          <p className="mt-1 text-sm leading-6 text-muted-foreground">
            修改后当前登录继续有效，其他登录和授权会话将失效。
          </p>
          <form
            className="mt-5 grid gap-5"
            onSubmit={changePassword}
            noValidate
            aria-busy={pending}
          >
            <PasswordField
              label="当前密码"
              name="current-password"
              autoComplete="current-password"
              value={currentPassword.value}
              error={errors.current}
              disabled={pending}
              onChange={(event) => currentPassword.setValue(event.target.value)}
              required
            />
            <PasswordField
              label="新密码"
              name="new-password"
              autoComplete="new-password"
              value={newPassword.value}
              error={errors.next}
              hint="使用 8 至 128 个字符"
              disabled={pending}
              onChange={(event) => newPassword.setValue(event.target.value)}
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
            <SubmitButton pending={pending} className="sm:w-fit">
              更新密码
            </SubmitButton>
          </form>
        </section>

        <section
          className="rounded-xl border p-5 sm:p-6"
          aria-labelledby="logout-heading"
        >
          <h2 id="logout-heading" className="text-lg font-semibold">
            退出当前账户
          </h2>
          <p className="mt-1 text-sm leading-6 text-muted-foreground">
            仅在服务确认注销成功后，页面才会返回登录入口。
          </p>
          <Button
            type="button"
            variant="outline"
            className="mt-5 h-10"
            disabled={pending}
            onClick={() => void logout()}
          >
            {pending ? (
              <LoaderCircle className="animate-spin" aria-hidden="true" />
            ) : (
              <LogOut aria-hidden="true" />
            )}
            退出登录
          </Button>
        </section>
      </div>
    </AccountShell>
  )
}
