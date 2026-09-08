"use client"

import { useCallback, useEffect, useRef, useState } from "react"

interface OneTimeValue {
  ready: boolean
  value: string | null
}

export function useOneTimeFragment(): OneTimeValue & { clear(): void } {
  const initialized = useRef(false)
  const [state, setState] = useState<OneTimeValue>({
    ready: false,
    value: null,
  })
  const clear = useCallback(() => setState({ ready: true, value: null }), [])

  useEffect(() => {
    const openNewLink = () => {
      if (window.location.hash) window.location.reload()
    }
    if (!initialized.current) {
      initialized.current = true
      const fragment = window.location.hash.slice(1)
      window.history.replaceState(
        window.history.state,
        "",
        `${window.location.pathname}${window.location.search}`
      )
      setState({ ready: true, value: fragment || null })
    } else {
      openNewLink()
    }
    // 片段导航不会重新挂载组件，新邮件链接需要独立的页面状态。
    window.addEventListener("hashchange", openNewLink)
    return () => window.removeEventListener("hashchange", openNewLink)
  }, [])

  return { ...state, clear }
}

export function useOneTimeRequestId(): OneTimeValue {
  const initialized = useRef(false)
  const [state, setState] = useState<OneTimeValue>({
    ready: false,
    value: null,
  })

  useEffect(() => {
    if (initialized.current) return
    initialized.current = true

    const requestId = new URL(window.location.href).searchParams.get(
      "request_id"
    )
    window.history.replaceState(
      window.history.state,
      "",
      window.location.pathname
    )
    setState({ ready: true, value: requestId?.trim() || null })
  }, [])

  return state
}
