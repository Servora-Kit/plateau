"use client"

import { useEffect, useState } from "react"
import { useRouter } from "next/navigation"

import { createIamApi } from "@/lib/iam-api"

interface UseGuestSessionOptions {
  enabled?: boolean
  requestId?: string | null
}

export function useGuestSession(options?: UseGuestSessionOptions) {
  const enabled = options?.enabled ?? true
  const requestId = options?.requestId ?? null
  const router = useRouter()
  const [revision, setRevision] = useState(0)
  const [ready, setReady] = useState(false)

  useEffect(() => {
    if (!enabled) return

    const controller = new AbortController()
    void createIamApi(controller.signal)
      .account.GetProfile({})
      .then(
        ({ user }) => {
          if (controller.signal.aborted) return
          if (user) {
            if (requestId) {
              window.location.replace(
                `/authorize/callback?id=${encodeURIComponent(requestId)}`
              )
            } else {
              router.replace("/account/")
            }
            return
          }
          setReady(true)
        },
        () => {
          if (controller.signal.aborted) return
          setReady(true)
        }
      )

    const handlePageShow = (event: PageTransitionEvent) => {
      if (!event.persisted) return
      setReady(false)
      setRevision((current) => current + 1)
    }
    window.addEventListener("pageshow", handlePageShow)

    return () => {
      controller.abort()
      window.removeEventListener("pageshow", handlePageShow)
    }
  }, [enabled, requestId, router, revision])

  return { ready: enabled && ready }
}
