"use client"

import { Eye, EyeOff, LoaderCircle } from "lucide-react"
import { useId, useState, type ComponentProps } from "react"

import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"

interface TextFieldProps extends Omit<ComponentProps<"input">, "id"> {
  label: string
  error?: string | null
  hint?: string
}

export function TextField({
  label,
  error,
  hint,
  className,
  ...props
}: TextFieldProps) {
  const id = useId()
  const messageId = `${id}-message`

  return (
    <div className="grid gap-2">
      <label htmlFor={id} className="text-sm font-medium">
        {label}
      </label>
      <input
        id={id}
        aria-invalid={Boolean(error)}
        aria-describedby={error || hint ? messageId : undefined}
        className={cn(
          "h-10 w-full rounded-lg border border-input bg-background px-3 text-base outline-none placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50 aria-invalid:border-destructive aria-invalid:ring-destructive/20 sm:text-sm",
          className
        )}
        {...props}
      />
      {error ? (
        <p id={messageId} className="text-sm text-destructive" role="alert">
          {error}
        </p>
      ) : hint ? (
        <p id={messageId} className="text-sm text-muted-foreground">
          {hint}
        </p>
      ) : null}
    </div>
  )
}

type PasswordFieldProps = Omit<TextFieldProps, "type">

export function PasswordField({
  label,
  error,
  hint,
  className,
  ...props
}: PasswordFieldProps) {
  const id = useId()
  const messageId = `${id}-message`
  const [visible, setVisible] = useState(false)

  return (
    <div className="grid gap-2">
      <label htmlFor={id} className="text-sm font-medium">
        {label}
      </label>
      <div className="relative">
        <input
          id={id}
          type={visible ? "text" : "password"}
          aria-invalid={Boolean(error)}
          aria-describedby={error || hint ? messageId : undefined}
          className={cn(
            "h-10 w-full rounded-lg border border-input bg-background px-3 pr-11 text-base outline-none placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50 aria-invalid:border-destructive aria-invalid:ring-destructive/20 sm:text-sm",
            className
          )}
          {...props}
        />
        <button
          type="button"
          className="absolute inset-y-0 right-0 flex w-11 items-center justify-center rounded-r-lg text-muted-foreground outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring disabled:pointer-events-none disabled:opacity-50"
          aria-label={visible ? "隐藏密码" : "显示密码"}
          aria-pressed={visible}
          disabled={props.disabled}
          onClick={() => setVisible((current) => !current)}
        >
          {visible ? <EyeOff aria-hidden="true" /> : <Eye aria-hidden="true" />}
        </button>
      </div>
      {error ? (
        <p id={messageId} className="text-sm text-destructive" role="alert">
          {error}
        </p>
      ) : hint ? (
        <p id={messageId} className="text-sm text-muted-foreground">
          {hint}
        </p>
      ) : null}
    </div>
  )
}

export function SubmitButton({
  pending,
  children,
  className,
  ...props
}: ComponentProps<typeof Button> & { pending: boolean }) {
  return (
    <Button
      {...props}
      type="submit"
      size="lg"
      className={cn("h-10 w-full", className)}
      disabled={pending || props.disabled}
    >
      {pending ? (
        <LoaderCircle className="animate-spin" aria-hidden="true" />
      ) : null}
      {children}
    </Button>
  )
}
