"use client"

import type { CapProgressEvent, CapSolveEvent, CapWidget } from "cap-widget"
import {
  forwardRef,
  useEffect,
  useImperativeHandle,
  useRef,
  useState,
} from "react"

import { StatusMessage } from "@/components/status-message"

interface CapWindow extends Window {
  CAP_DISABLE_WIDGET_REF?: boolean
}

export interface CapVerificationHandle {
  reset(): void
}

interface CapVerificationProps {
  busy: boolean
  onTokenChange(token: string | null): void
}

export const CapVerification = forwardRef<
  CapVerificationHandle,
  CapVerificationProps
>(function CapVerification({ busy, onTokenChange }, forwardedRef) {
  const hostRef = useRef<HTMLDivElement>(null)
  const widgetRef = useRef<CapWidget | null>(null)
  const callbackRef = useRef(onTokenChange)
  const [progress, setProgress] = useState<number | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [generation, setGeneration] = useState(0)

  callbackRef.current = onTokenChange

  useImperativeHandle(
    forwardedRef,
    () => ({
      reset() {
        widgetRef.current?.reset()
      },
    }),
    []
  )

  useEffect(() => {
    let active = true
    const host = hostRef.current
    let removeListeners = () => {}
    if (!host) return

    const capWindow = window as CapWindow
    capWindow.CAP_CUSTOM_WASM_URL = "/cap-assets/cap_wasm_bg.wasm"
    capWindow.CAP_DISABLE_WIDGET_REF = true
    capWindow.CAP_SILENT = true

    void import("cap-widget")
      .then(() => {
        if (!active || !host.isConnected) return

        const widget = document.createElement("cap-widget")
        widget.setAttribute("data-cap-api-endpoint", "/cap/")
        widget.setAttribute("data-cap-i18n-initial-state", "请完成人机验证")
        widget.setAttribute("data-cap-i18n-verifying-label", "正在验证…")
        widget.setAttribute("data-cap-i18n-solved-label", "验证完成")
        widget.setAttribute("data-cap-i18n-error-label", "验证失败，请重试")
        widget.setAttribute("data-cap-i18n-verify-aria-label", "开始人机验证")
        widget.setAttribute(
          "data-cap-i18n-verifying-aria-label",
          "正在进行人机验证，请稍候"
        )
        widget.setAttribute(
          "data-cap-i18n-verified-aria-label",
          "人机验证已完成，可以继续"
        )
        widget.setAttribute(
          "data-cap-i18n-error-aria-label",
          "人机验证失败，请重试"
        )
        widget.setAttribute("required", "")

        const onProgress = (event: CapProgressEvent) => {
          if (!active) return
          setError(null)
          setProgress(
            Math.max(0, Math.min(100, Math.round(event.detail.progress)))
          )
        }
        const onSolve = (event: CapSolveEvent) => {
          if (!active) return
          setProgress(100)
          setError(null)
          callbackRef.current(event.detail.token)
        }
        const onError = () => {
          if (!active) return
          callbackRef.current(null)
          setProgress(null)
          setError("人机验证未完成，请重试")
        }
        const onReset = () => {
          if (!active) return
          callbackRef.current(null)
          setProgress(null)
          setError(null)
          // 固定版本 reset 未恢复读屏标签，重建官方实例以恢复完整初始状态。
          setGeneration((current) => current + 1)
        }

        widget.addEventListener("progress", onProgress)
        widget.addEventListener("solve", onSolve)
        widget.addEventListener("error", onError)
        widget.addEventListener("reset", onReset)
        widgetRef.current = widget
        host.appendChild(widget)

        if (!active) {
          widget.remove()
          return
        }

        removeListeners = () => {
          widget.removeEventListener("progress", onProgress)
          widget.removeEventListener("solve", onSolve)
          widget.removeEventListener("error", onError)
          widget.removeEventListener("reset", onReset)
        }
      })
      .catch(() => {
        if (!active) return
        callbackRef.current(null)
        setError("无法加载人机验证，请刷新页面重试")
      })

    return () => {
      active = false
      removeListeners()
      const widget = widgetRef.current
      widgetRef.current = null
      if (widget) {
        widget.reset()
        widget.remove()
      }
      host.replaceChildren()
    }
  }, [generation])

  return (
    <div className="grid gap-2" aria-busy={progress !== null && progress < 100}>
      <div
        ref={hostRef}
        inert={busy}
        aria-disabled={busy}
        className={busy ? "pointer-events-none opacity-50" : undefined}
      />
      {progress !== null && progress < 100 ? (
        <p
          className="text-sm text-muted-foreground"
          role="status"
          aria-live="polite"
        >
          验证进度 {progress}%
        </p>
      ) : null}
      {error ? <StatusMessage tone="error">{error}</StatusMessage> : null}
    </div>
  )
})
