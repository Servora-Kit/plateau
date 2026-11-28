"use client"

import type {
  plateauiamuserv1_User,
  plateauiamuserv1_UserProfile,
} from "@plateau/api/plateau/iam/account/v1"
import { CheckCircle2, Mail } from "lucide-react"
import { useRouter } from "next/navigation"
import { useState, type FormEvent } from "react"

import { AccountCard, AccountShell } from "@/components/account-shell"
import { UserAvatar } from "@/components/app-logo"
import { SubmitButton, TextField } from "@/components/form-controls"
import { StatusMessage } from "@/components/status-message"
import { Button } from "@/components/ui/button"
import { useProfile } from "@/hooks/use-profile"
import { useSingleFlight } from "@/hooks/use-single-flight"
import {
  isConflict,
  isUnauthenticated,
  requestErrorMessage,
} from "@/lib/errors"
import { createIamApi } from "@/lib/iam-api"
import { validateOptionalURL } from "@/lib/validation"

const profileFields = [
  ["name", "姓名", "name"],
  ["givenName", "名", "given-name"],
  ["familyName", "姓", "family-name"],
  ["nickname", "昵称", "nickname"],
  ["preferredUsername", "常用用户名", "username"],
  ["picture", "头像网址", "url"],
  ["locale", "语言区域", "language"],
] as const

type ProfileField = (typeof profileFields)[number][0]
type ProfileValues = Record<ProfileField, string>

function valuesFromProfile(
  profile?: plateauiamuserv1_UserProfile
): ProfileValues {
  return {
    name: profile?.name ?? "",
    givenName: profile?.givenName ?? "",
    familyName: profile?.familyName ?? "",
    nickname: profile?.nickname ?? "",
    preferredUsername: profile?.preferredUsername ?? "",
    picture: profile?.picture ?? "",
    locale: profile?.locale ?? "",
  }
}

export default function AccountPage() {
  const { state, reload, setUser } = useProfile()
  const [savedFeedback, setSavedFeedback] = useState<string | null>(null)

  if (state.status === "loading" || state.status === "unauthenticated") {
    return (
      <AccountShell
        title="个人信息"
        description="管理您在 Plateau 各项服务及授权应用中使用的基本资料与联系方式。"
      >
        <div className="rounded-2xl border bg-card p-6 shadow-2xs">
          <p className="text-sm text-muted-foreground" role="status">
            正在读取账户资料…
          </p>
        </div>
      </AccountShell>
    )
  }

  if (state.status === "error") {
    return (
      <AccountShell
        title="个人信息"
        description="管理您在 Plateau 各项服务及授权应用中使用的基本资料与联系方式。"
      >
        <div className="rounded-2xl border bg-card p-6 shadow-2xs">
          <StatusMessage tone="error">{state.message}</StatusMessage>
          <Button className="mt-4 h-10" onClick={() => void reload()}>
            重新加载
          </Button>
        </div>
      </AccountShell>
    )
  }

  return (
    <AccountShell
      title="个人信息"
      description="管理您在 Plateau 各项服务及授权应用中使用的基本资料与联系方式。"
      user={state.user}
      email={state.user.email}
    >
      {savedFeedback ? (
        <StatusMessage tone="success">{savedFeedback}</StatusMessage>
      ) : null}
      <ProfileForm
        key={state.user.etag ?? state.user.updateTime ?? "profile"}
        user={state.user}
        onClearSavedFeedback={() => setSavedFeedback(null)}
        onUpdated={(updatedUser) => {
          setUser(updatedUser)
          setSavedFeedback("个人信息已保存。")
        }}
      />
    </AccountShell>
  )
}

function ProfileForm({
  user,
  onUpdated,
  onClearSavedFeedback,
}: {
  user: plateauiamuserv1_User
  onUpdated(user: plateauiamuserv1_User): void
  onClearSavedFeedback(): void
}) {
  const router = useRouter()
  const { pending, run } = useSingleFlight()
  const initial = valuesFromProfile(user.profile)
  const [values, setValues] = useState<ProfileValues>(initial)
  const [pictureError, setPictureError] = useState<string | null>(null)
  const [message, setMessage] = useState<{
    tone: "error" | "success" | "info"
    text: string
  } | null>(null)

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (pending) return

    onClearSavedFeedback()
    const nextPictureError = validateOptionalURL(values.picture)
    setPictureError(nextPictureError)
    setMessage(null)
    if (nextPictureError) return

    const changed = profileFields
      .map(([field]) => field)
      .filter((field) => values[field] !== initial[field])
    if (changed.length === 0) {
      setMessage({ tone: "info", text: "没有需要保存的更改。" })
      return
    }

    const profile: plateauiamuserv1_UserProfile = {}
    for (const field of changed) profile[field] = values[field]
    const updateMask = changed.join(",")

    const result = await run((signal) =>
      createIamApi(signal).account.UpdateProfile({ profile, updateMask })
    )
    if (result.status === "success") {
      if (result.value.user) {
        onUpdated(result.value.user)
      } else {
        setMessage({
          tone: "error",
          text: "资料已提交，但响应不完整，请重新加载。",
        })
      }
      return
    }
    if (result.status !== "failure") return
    if (isUnauthenticated(result.error)) {
      router.replace("/login/")
      return
    }
    if (isConflict(result.error)) {
      setMessage({
        tone: "error",
        text: "资料已在其他位置更新，请重新加载后再修改。",
      })
      return
    }
    setMessage({
      tone: "error",
      text: requestErrorMessage(result.error, "无法保存资料，请重试"),
    })
  }

  const previewName = values.name.trim() || user.email || "P"

  return (
    <AccountCard
      id="basic-profile"
      title="基本信息"
      description="部分公开资料（如姓名、昵称与头像）会在您使用或授权访问的 Plateau 应用中展示。"
    >
      <div className="divide-y">
        <div className="flex flex-col gap-4 px-5 py-5 sm:flex-row sm:items-center sm:justify-between sm:px-6">
          <div className="space-y-1">
            <h3 className="text-sm font-medium text-foreground">个人资料照片</h3>
            <p className="text-sm leading-6 text-muted-foreground">
              当前支持填写公开可访问的头像网址（http/https）；留空或图片无法访问时自动显示首字母圆形徽章。
            </p>
          </div>
          <UserAvatar src={values.picture} name={previewName} size="lg" />
        </div>

        {user.email ? (
          <div className="flex flex-col gap-2 px-5 py-4 sm:flex-row sm:items-center sm:justify-between sm:px-6">
            <div className="flex items-center gap-2.5 text-sm font-medium text-foreground">
              <Mail
                className="size-4 shrink-0 text-muted-foreground"
                aria-hidden="true"
              />
              <span>登录邮箱</span>
            </div>
            <div className="flex flex-wrap items-center gap-2 text-sm">
              <span className="font-medium text-foreground">{user.email}</span>
              {user.emailVerified ? (
                <span className="inline-flex items-center gap-1 rounded-full border bg-muted/50 px-2.5 py-0.5 text-xs font-medium text-muted-foreground">
                  <CheckCircle2 className="size-3 text-primary" aria-hidden="true" />
                  已验证
                </span>
              ) : null}
            </div>
          </div>
        ) : null}

        <form
          className="grid gap-5 px-5 py-5 sm:px-6 sm:py-6"
          onSubmit={submit}
          noValidate
          aria-busy={pending}
        >
          {message ? (
            <StatusMessage tone={message.tone}>{message.text}</StatusMessage>
          ) : null}
          <div className="grid gap-5 sm:grid-cols-2">
            {profileFields.map(([field, label, autoComplete]) => (
              <div
                key={field}
                className={
                  field === "name" || field === "picture"
                    ? "sm:col-span-2"
                    : undefined
                }
              >
                <TextField
                  label={label}
                  name={field}
                  type={field === "picture" ? "url" : "text"}
                  autoComplete={autoComplete}
                  placeholder={
                    field === "picture"
                      ? "https://example.com/avatar.png"
                      : field === "locale"
                        ? "zh-CN"
                        : undefined
                  }
                  value={values[field]}
                  error={field === "picture" ? pictureError : undefined}
                  disabled={pending}
                  onChange={(event) => {
                    onClearSavedFeedback()
                    if (field === "picture" && pictureError) {
                      setPictureError(null)
                    }
                    setValues((current) => ({
                      ...current,
                      [field]: event.target.value,
                    }))
                  }}
                />
              </div>
            ))}
          </div>
          <div className="flex justify-end pt-1">
            <SubmitButton pending={pending} className="w-full sm:w-auto sm:px-6">
              保存更改
            </SubmitButton>
          </div>
        </form>
      </div>
    </AccountCard>
  )
}
