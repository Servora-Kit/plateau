import { AlertCircle, CheckCircle2, Info } from "lucide-react"

import { cn } from "@/lib/utils"

interface StatusMessageProps {
  children: React.ReactNode
  tone?: "error" | "success" | "info"
}

export function StatusMessage({ children, tone = "info" }: StatusMessageProps) {
  const Icon =
    tone === "error" ? AlertCircle : tone === "success" ? CheckCircle2 : Info

  return (
    <div
      role={tone === "error" ? "alert" : "status"}
      aria-live={tone === "error" ? "assertive" : "polite"}
      className={cn(
        "flex items-start gap-2 rounded-lg border p-3 text-sm",
        tone === "error" &&
          "border-destructive/30 bg-destructive/5 text-destructive",
        tone === "success" && "border-border bg-muted/60 text-foreground",
        tone === "info" && "border-border bg-muted/60 text-muted-foreground"
      )}
    >
      <Icon className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
      <div className="min-w-0 leading-5">{children}</div>
    </div>
  )
}
