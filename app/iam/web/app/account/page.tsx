"use client"

import type {
  iamuserv1_User,
  iamuserv1_UserProfile,
} from "@plateau/api/iam/account/v1"
import { useState, type FormEvent } from "react"
import { useRouter } from "next/navigation"

import { AccountShell } from "@/components/account-shell"
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

function valuesFromProfile(profile?: iamuserv1_UserProfile): ProfileValues {
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

  if (state.status === "loading" || state.status === "unauthenticated") {
    return (
      <AccountShell
        title="个人资料"
        description="管理用于 Plateau 和 OpenID Connect 的公开资料。"
      >
        <p className="text-sm text-muted-foreground" role="status">
          正在读取账户资料…
        </p>
      </AccountShell>
    )
  }

  if (state.status === "error") {
    return (
      <AccountShell
        title="个人资料"
        description="管理用于 Plateau 和 OpenID Connect 的公开资料。"
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
      title="个人资料"
      description="管理用于 Plateau 和 OpenID Connect 的公开资料。"
      email={state.user.email}
    >
      <ProfileForm
        key={state.user.etag ?? state.user.updateTime ?? "profile"}
        user={state.user}
        onUpdated={setUser}
      />
    </AccountShell>
  )
}

function ProfileForm({
  user,
  onUpdated,
}: {
  user: iamuserv1_User
  onUpdated(user: iamuserv1_User): void
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

    const profile: iamuserv1_UserProfile = {}
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

  return (
    <form
      className="grid gap-5"
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
              value={values[field]}
              error={field === "picture" ? pictureError : undefined}
              disabled={pending}
              onChange={(event) =>
                setValues((current) => ({
                  ...current,
                  [field]: event.target.value,
                }))
              }
            />
          </div>
        ))}
      </div>
      <div className="flex justify-end">
        <SubmitButton pending={pending} className="w-full sm:w-auto">
          保存更改
        </SubmitButton>
      </div>
    </form>
  )
}
