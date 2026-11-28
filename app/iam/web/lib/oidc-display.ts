export interface OidcScopeDescriptor {
  scope: string
  title: string
  description: string
  known: boolean
}

const standardScopeMap: Record<
  string,
  { title: string; description: string }
> = {
  openid: {
    title: "基础身份标识",
    description: "验证您的基础身份标识",
  },
  profile: {
    title: "公开个人资料",
    description: "读取您的公开个人资料（姓名、昵称、头像、语言）",
  },
  email: {
    title: "电子邮箱地址",
    description: "读取您的已验证电子邮箱地址",
  },
  offline_access: {
    title: "离线访问凭证",
    description: "在您离线时保持授权并刷新访问凭证",
  },
}

export function normalizeHttpImageUrl(raw?: string | null): string | null {
  const trimmed = raw?.trim()
  if (!trimmed) return null
  try {
    const parsed = new URL(trimmed)
    if (parsed.protocol !== "https:" && parsed.protocol !== "http:") {
      return null
    }
    return parsed.toString()
  } catch {
    return null
  }
}

export function extractInitial(label?: string | null, fallback = "P"): string {
  const trimmed = label?.trim()
  if (!trimmed) return fallback
  const [first] = Array.from(trimmed)
  return first ? first.toUpperCase() : fallback
}

export function resolveOidcClientName(
  client?: { clientId?: string; clientName?: string } | null,
  fallback = "第三方应用"
): string {
  const name = client?.clientName?.trim()
  if (name) return name
  const id = client?.clientId?.trim()
  if (id) return id
  return fallback
}

export function describeOidcScope(rawScope: string): OidcScopeDescriptor {
  const scope = rawScope.trim()
  const matched = standardScopeMap[scope]
  if (matched) {
    return {
      scope,
      title: matched.title,
      description: matched.description,
      known: true,
    }
  }
  return {
    scope,
    title: scope,
    description: scope,
    known: false,
  }
}

export function dedupeScopes(scopes?: readonly string[] | null): string[] {
  if (!scopes || scopes.length === 0) return []
  const seen = new Set<string>()
  const result: string[] = []
  for (const item of scopes) {
    const normalized = item.trim()
    if (!normalized || seen.has(normalized)) continue
    seen.add(normalized)
    result.push(normalized)
  }
  return result
}

export function formatDateTime(iso?: string | null): string | null {
  const trimmed = iso?.trim()
  if (!trimmed) return null
  const date = new Date(trimmed)
  if (Number.isNaN(date.getTime())) return null
  return new Intl.DateTimeFormat("zh-CN", {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(date)
}
