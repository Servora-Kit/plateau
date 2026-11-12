"use client"

import { LoaderCircle, LogOut, Trash2 } from "lucide-react"
import { useId, useState, type FormEvent } from "react"
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
  const confirmCheckboxId = useId()
  const confirmErrorId = `${confirmCheckboxId}-error`
  const { state, reload } = useProfile()
  const currentPassword = useSensitiveValue()
  const newPassword = useSensitiveValue()
  const confirmation = useSensitiveValue()
  const deletePassword = useSensitiveValue()
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
  const [deleteOpen, setDeleteOpen] = useState(false)
  const [deleteConfirmed, setDeleteConfirmed] = useState(false)
  const [deleteErrors, setDeleteErrors] = useState<{
    confirmed?: string
    password?: string
  }>({})
  const [deleteMessage, setDeleteMessage] = useState<string | null>(null)

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
    deletePassword.clear()

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
    deletePassword.clear()

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

  function openDeleteConfirmation() {
    if (pending) return
    deletePassword.clear()
    setDeleteConfirmed(false)
    setDeleteErrors({})
    setDeleteMessage(null)
    setDeleteOpen(true)
  }

  function cancelDeleteConfirmation() {
    if (pending) return
    deletePassword.clear()
    setDeleteConfirmed(false)
    setDeleteErrors({})
    setDeleteMessage(null)
    setDeleteOpen(false)
  }

  async function deleteAccount(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (pending) return

    const nextDeleteErrors = {
      confirmed: deleteConfirmed ? undefined : "请先勾选确认删除账户的影响",
      password:
        validatePassword(deletePassword.current.current, "当前密码") ??
        undefined,
    }
    setDeleteErrors(nextDeleteErrors)
    setDeleteMessage(null)
    setMessage(null)
    if (nextDeleteErrors.confirmed || nextDeleteErrors.password) return

    const submittedCurrent = deletePassword.current.current
    const result = await run((signal) =>
      createIamApi(signal).account.DeleteAccount({
        currentPassword: submittedCurrent,
      })
    )
    currentPassword.clear()
    newPassword.clear()
    confirmation.clear()
    deletePassword.clear()

    if (result.status === "success") {
      window.location.assign("/login/")
      return
    }
    if (result.status !== "failure") return
    if (isUnauthenticated(result.error)) {
      window.location.assign("/login/")
      return
    }
    if (isInvalidPassword(result.error)) {
      setDeleteErrors({ password: "当前密码不正确" })
      return
    }
    setDeleteMessage("无法确认是否已完成删除，请刷新或重新确认账户状态")
  }

  if (state.status === "loading" || state.status === "unauthenticated") {
    return (
      <AccountShell
        title="账户安全"
        description="更新密码、结束当前登录或删除账户。"
      >
        <p className="text-sm text-muted-foreground" role="status">
          正在确认账户状态…
        </p>
      </AccountShell>
    )
  }

  if (state.status === "error") {
    return (
      <AccountShell
        title="账户安全"
        description="更新密码、结束当前登录或删除账户。"
      >
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
      description="更新密码、结束当前登录或删除账户。"
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

        <section
          className="rounded-xl border border-destructive/30 p-5 sm:p-6"
          aria-labelledby="delete-account-heading"
        >
          <h2
            id="delete-account-heading"
            className="text-lg font-semibold text-destructive"
          >
            删除账户
          </h2>
          <p className="mt-1 text-sm leading-6 text-muted-foreground">
            删除账户将立即退出所有设备并撤销全部登录与授权会话。删除后该邮箱仍保持占用，无法用于重新注册；自助删除不会永久擦除账户数据，仅平台管理员可恢复。
          </p>
          {!deleteOpen ? (
            <Button
              type="button"
              variant="destructive"
              className="mt-5 h-10"
              disabled={pending}
              onClick={openDeleteConfirmation}
            >
              <Trash2 aria-hidden="true" />
              删除账户
            </Button>
          ) : (
            <form
              className="mt-5 grid gap-5"
              onSubmit={deleteAccount}
              noValidate
              aria-busy={pending}
            >
              {deleteMessage ? (
                <StatusMessage tone="error">{deleteMessage}</StatusMessage>
              ) : null}
              <div className="grid gap-2">
                <label
                  htmlFor={confirmCheckboxId}
                  className="flex items-start gap-2.5 text-sm leading-6"
                >
                  <input
                    id={confirmCheckboxId}
                    type="checkbox"
                    checked={deleteConfirmed}
                    disabled={pending}
                    aria-invalid={Boolean(deleteErrors.confirmed)}
                    aria-describedby={
                      deleteErrors.confirmed ? confirmErrorId : undefined
                    }
                    onChange={(event) => {
                      setDeleteConfirmed(event.target.checked)
                      if (event.target.checked && deleteErrors.confirmed) {
                        setDeleteErrors((current) => ({
                          ...current,
                          confirmed: undefined,
                        }))
                      }
                    }}
                    className="mt-1 size-4 shrink-0 rounded border-input accent-destructive outline-none focus-visible:ring-3 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
                  />
                  <span>
                    我已知晓删除账户会立即退出所有设备、邮箱继续保留占用，且仅可由平台管理员恢复。
                  </span>
                </label>
                {deleteErrors.confirmed ? (
                  <p
                    id={confirmErrorId}
                    className="text-sm text-destructive"
                    role="alert"
                  >
                    {deleteErrors.confirmed}
                  </p>
                ) : null}
              </div>
              <PasswordField
                label="当前密码"
                name="delete-current-password"
                autoComplete="current-password"
                value={deletePassword.value}
                error={deleteErrors.password}
                disabled={pending}
                onChange={(event) =>
                  deletePassword.setValue(event.target.value)
                }
                required
              />
              <div className="flex flex-col gap-3 sm:flex-row">
                <SubmitButton
                  variant="destructive"
                  pending={pending}
                  disabled={pending || !deleteConfirmed}
                  className="sm:w-fit"
                >
                  确认删除账户
                </SubmitButton>
                <Button
                  type="button"
                  variant="outline"
                  className="h-10 sm:w-fit"
                  disabled={pending}
                  onClick={cancelDeleteConfirmation}
                >
                  取消
                </Button>
              </div>
            </form>
          )}
        </section>
      </div>
    </AccountShell>
  )
}
