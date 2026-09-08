"use client"

import { useCallback, useEffect, useRef, useState } from "react"

type FlightResult<T> =
  | { status: "success"; value: T }
  | { status: "failure"; error: unknown }
  | { status: "busy" | "stale" }

export function useSingleFlight() {
  const mounted = useRef(true)
  const active = useRef<AbortController | null>(null)
  const [pending, setPending] = useState(false)

  useEffect(() => {
    mounted.current = true
    return () => {
      mounted.current = false
      active.current?.abort(new DOMException("页面已离开", "AbortError"))
      active.current = null
    }
  }, [])

  const run = useCallback(
    async <T>(
      task: (signal: AbortSignal) => Promise<T>
    ): Promise<FlightResult<T>> => {
      if (active.current) return { status: "busy" }

      const controller = new AbortController()
      active.current = controller
      setPending(true)

      try {
        const value = await task(controller.signal)
        if (!mounted.current || active.current !== controller)
          return { status: "stale" }
        return { status: "success", value }
      } catch (error: unknown) {
        if (!mounted.current || active.current !== controller)
          return { status: "stale" }
        return { status: "failure", error }
      } finally {
        if (active.current === controller) {
          active.current = null
          if (mounted.current) setPending(false)
        }
      }
    },
    []
  )

  return { pending, run }
}
