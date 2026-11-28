"use client"

import type { plateauiamuserv1_User } from "@plateau/api/plateau/iam/account/v1"
import {
  createContext,
  createElement,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react"
import { useRouter } from "next/navigation"

import { isUnauthenticated, requestErrorMessage } from "@/lib/errors"
import { createIamApi } from "@/lib/iam-api"

export type ProfileState =
  | { status: "loading"; user: null; message: null }
  | { status: "ready"; user: plateauiamuserv1_User; message: null }
  | { status: "error"; user: null; message: string }
  | { status: "unauthenticated"; user: null; message: null }

export interface ProfileController {
  state: ProfileState
  reload(): void
  setUser(user: plateauiamuserv1_User): void
}

const AccountProfileContext = createContext<ProfileController | null>(null)

function useStandaloneProfile(enabled: boolean): ProfileController {
  const router = useRouter()
  const [revision, setRevision] = useState(0)
  const [state, setState] = useState<ProfileState>({
    status: "loading",
    user: null,
    message: null,
  })

  useEffect(() => {
    if (!enabled) return
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

    const handlePageShow = (event: PageTransitionEvent) => {
      if (!event.persisted) return
      setState({ status: "loading", user: null, message: null })
      setRevision((current) => current + 1)
    }
    window.addEventListener("pageshow", handlePageShow)

    return () => {
      controller.abort()
      window.removeEventListener("pageshow", handlePageShow)
    }
  }, [enabled, router, revision])

  const reload = useCallback(() => {
    setState({ status: "loading", user: null, message: null })
    setRevision((current) => current + 1)
  }, [])

  const setUser = useCallback((user: plateauiamuserv1_User) => {
    setState({ status: "ready", user, message: null })
  }, [])

  return useMemo(
    () => ({
      state,
      reload,
      setUser,
    }),
    [reload, setUser, state]
  )
}

export function AccountProfileProvider({ children }: { children: ReactNode }) {
  const controller = useStandaloneProfile(true)
  return createElement(
    AccountProfileContext.Provider,
    { value: controller },
    children
  )
}

export function useProfile(): ProfileController {
  const shared = useContext(AccountProfileContext)
  const fallback = useStandaloneProfile(shared === null)
  return shared ?? fallback
}
