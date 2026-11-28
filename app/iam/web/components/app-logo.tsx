"use client"

import { useState } from "react"

import { extractInitial, normalizeHttpImageUrl } from "@/lib/oidc-display"
import { cn } from "@/lib/utils"

type AppLogoSize = "xs" | "sm" | "md" | "lg"
type UserAvatarSize = "xs" | "sm" | "md" | "lg"

const appLogoSizeClasses: Record<AppLogoSize, string> = {
  xs: "size-7 rounded-lg text-xs",
  sm: "size-9 rounded-xl text-sm",
  md: "size-11 rounded-2xl text-base",
  lg: "size-14 rounded-2xl text-xl",
}

const userAvatarSizeClasses: Record<UserAvatarSize, string> = {
  xs: "size-6 text-[11px]",
  sm: "size-8 text-xs",
  md: "size-10 text-sm",
  lg: "size-16 text-xl",
}

export interface AppLogoProps {
  src?: string | null
  name: string
  size?: AppLogoSize
  alt?: string
  className?: string
}

export function AppLogo({
  src,
  name,
  size = "md",
  alt,
  className,
}: AppLogoProps) {
  const [failedSrc, setFailedSrc] = useState<string | null>(null)
  const validUrl = normalizeHttpImageUrl(src)
  const showImage = Boolean(validUrl && failedSrc !== validUrl)
  const initial = extractInitial(name, "A")
  const decorative = !alt

  return (
    <span
      aria-hidden={decorative ? "true" : undefined}
      className={cn(
        "inline-flex shrink-0 items-center justify-center overflow-hidden font-semibold select-none ring-1",
        appLogoSizeClasses[size],
        showImage
          ? "bg-background ring-border/80"
          : "bg-primary/10 text-primary ring-primary/15 dark:bg-primary/15",
        className
      )}
    >
      {showImage && validUrl ? (
        // eslint-disable-next-line @next/next/no-img-element
        <img
          src={validUrl}
          alt={alt ?? ""}
          referrerPolicy="no-referrer"
          decoding="async"
          onError={() => setFailedSrc(validUrl)}
          className="size-full object-cover"
        />
      ) : (
        <span aria-hidden="true">{initial}</span>
      )}
    </span>
  )
}

export interface UserAvatarProps {
  src?: string | null
  name: string
  size?: UserAvatarSize
  alt?: string
  className?: string
}

export function UserAvatar({
  src,
  name,
  size = "sm",
  alt,
  className,
}: UserAvatarProps) {
  const [failedSrc, setFailedSrc] = useState<string | null>(null)
  const validUrl = normalizeHttpImageUrl(src)
  const showImage = Boolean(validUrl && failedSrc !== validUrl)
  const initial = extractInitial(name, "P")
  const decorative = !alt

  return (
    <span
      aria-hidden={decorative ? "true" : undefined}
      className={cn(
        "inline-flex shrink-0 items-center justify-center overflow-hidden rounded-full font-semibold select-none ring-1",
        userAvatarSizeClasses[size],
        showImage
          ? "bg-muted/40 ring-border/80"
          : "bg-primary text-primary-foreground ring-primary/20",
        className
      )}
    >
      {showImage && validUrl ? (
        // eslint-disable-next-line @next/next/no-img-element
        <img
          src={validUrl}
          alt={alt ?? ""}
          referrerPolicy="no-referrer"
          decoding="async"
          onError={() => setFailedSrc(validUrl)}
          className="size-full object-cover"
        />
      ) : (
        <span aria-hidden="true">{initial}</span>
      )}
    </span>
  )
}
