"use client"

import type { iamuserv1_User } from "@plateau/api/iam/account/v1"
import { useEffect, useState } from "react"
import { useRouter } from "next/navigation"

import { createIamApi } from "@/lib/iam-api"
import { isUnauthenticated, requestErrorMessage } from "@/lib/errors"

type ProfileState =
  | { status: "loading"; user: null; message: null }
  | { status: "ready"; user: iamuserv1_User; message: null }
  | { status: "error"; user: null; message: string }
  | { status: "unauthenticated"; user: null; message: null }

export function useProfile() {
  const router = useRouter()
  const [revision, setRevision] = useState(0)
  const [state, setState] = useState<ProfileState>({
    status: "loading",
    user: null,
    message: null,
  })

  useEffect(() => {
    const controller = new AbortController()
    void createIamApi(controller.signal)
      .account.GetProfile({})
      .then(
        ({ user }) => {
          if (controller.signal.aborted) return
          setState(
            user
              ? { status: "ready", user, message: null }
              : {
                  status: "error",
                  user: null,
                  message: "账户资料响应不完整，请重试",
                }
          )
        },
        (error: unknown) => {
          if (controller.signal.aborted) return
          if (isUnauthenticated(error)) {
            setState({ status: "unauthenticated", user: null, message: null })
            router.replace("/login/")
            return
          }
          setState({
            status: "error",
            user: null,
            message: requestErrorMessage(error, "无法读取账户资料，请重试"),
          })
        }
      )
    return () => controller.abort()
  }, [router, revision])

  return {
    state,
    reload: () => {
      setState({ status: "loading", user: null, message: null })
      setRevision((current) => current + 1)
    },
    setUser: (user: iamuserv1_User) =>
      setState({ status: "ready", user, message: null }),
  }
}
