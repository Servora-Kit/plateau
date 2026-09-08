const emailPattern = /^[^\s@]+@[^\s@]+\.[^\s@]+$/u

export function validateEmail(value: string): string | null {
  const email = value.trim()
  if (!email) return "请输入邮箱地址"
  if (email.length > 254 || !emailPattern.test(email))
    return "请输入有效的邮箱地址"
  return null
}

export function validatePassword(value: string, label = "密码"): string | null {
  if (!value) return `请输入${label}`
  if (value.length < 8 || value.length > 128)
    return `${label}长度须为 8 至 128 个字符`
  return null
}

export function validateOptionalURL(value: string): string | null {
  if (!value) return null
  try {
    const url = new URL(value)
    if (url.protocol !== "http:" && url.protocol !== "https:")
      return "请输入 http 或 https 地址"
  } catch {
    return "请输入有效的网址"
  }
  return null
}
