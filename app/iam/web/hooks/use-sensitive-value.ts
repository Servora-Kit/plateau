"use client"

import { useCallback, useEffect, useRef, useState } from "react"

export function useSensitiveValue() {
  const [value, setState] = useState("")
  const current = useRef("")
  const mounted = useRef(true)

  const setValue = useCallback((next: string) => {
    current.current = next
    if (mounted.current) setState(next)
  }, [])

  const clear = useCallback(() => {
    current.current = ""
    if (mounted.current) setState("")
  }, [])

  useEffect(() => {
    mounted.current = true
    return () => {
      mounted.current = false
      current.current = ""
    }
  }, [])

  return { value, current, setValue, clear }
}
