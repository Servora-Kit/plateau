"use client"

import { useEffect, useState } from "react"
import { useRouter } from "next/navigation"

import { createIamApi } from "@/lib/iam-api"

export function useGuestSession() {
  const router = useRouter()
  const [revision, setRevision] = useState(0)
  const [ready, setReady] = useState(false)

  useEffect(() => {
    const controller = new AbortController()
    void createIamApi(controller.signal)
      .account.GetProfile({})
      .then(
        ({ user }) => {
          if (controller.signal.aborted) return
          if (user) {
            router.replace("/account/")
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
  }, [router, revision])

  return { ready }
}
