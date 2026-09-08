import { ShieldCheck } from "lucide-react"
import Link from "next/link"

interface AuthShellProps {
  title: string
  description: string
  children: React.ReactNode
  footer?: React.ReactNode
}

export function AuthShell({
  title,
  description,
  children,
  footer,
}: AuthShellProps) {
  return (
    <main
      id="main-content"
      className="flex min-h-dvh items-center justify-center px-4 py-10 sm:px-6"
    >
      <div className="w-full max-w-md">
        <Link
          href="/"
          className="mx-auto mb-8 flex w-fit items-center gap-2 rounded-md text-sm font-semibold outline-none focus-visible:ring-3 focus-visible:ring-ring"
        >
          <ShieldCheck className="size-5" aria-hidden="true" />
          Plateau 账户
        </Link>
        <section
          className="rounded-xl border bg-card p-6 text-card-foreground shadow-sm sm:p-8"
          aria-labelledby="page-title"
        >
          <header className="mb-6 space-y-2">
            <h1
              id="page-title"
              className="text-2xl font-semibold tracking-tight"
            >
              {title}
            </h1>
            <p className="text-sm leading-6 text-muted-foreground">
              {description}
            </p>
          </header>
          {children}
        </section>
        {footer ? (
          <div className="mt-6 text-center text-sm leading-6 text-muted-foreground">
            {footer}
          </div>
        ) : null}
      </div>
    </main>
  )
}
