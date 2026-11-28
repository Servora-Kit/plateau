import assert from "node:assert/strict"
import test from "node:test"

import {
  dedupeScopes,
  describeOidcScope,
  extractInitial,
  formatDateTime,
  normalizeHttpImageUrl,
  resolveOidcClientName,
} from "./oidc-display.ts"

test("normalizeHttpImageUrl accepts http/https URLs and rejects unsafe schemes", () => {
  assert.equal(
    normalizeHttpImageUrl("  https://cdn.example.com/logo.png "),
    "https://cdn.example.com/logo.png"
  )
  assert.equal(
    normalizeHttpImageUrl("http://localhost:10012/icon.svg"),
    "http://localhost:10012/icon.svg"
  )
  assert.equal(normalizeHttpImageUrl(""), null)
  assert.equal(normalizeHttpImageUrl("   "), null)
  assert.equal(normalizeHttpImageUrl("javascript:alert(1)"), null)
  assert.equal(normalizeHttpImageUrl("data:image/png;base64,AAAA"), null)
  assert.equal(normalizeHttpImageUrl("not-a-valid-url"), null)
})

test("extractInitial handles ASCII, CJK, and fallback labels", () => {
  assert.equal(extractInitial("blog-web"), "B")
  assert.equal(extractInitial("  Plateau 控制台 "), "P")
  assert.equal(extractInitial("博客应用"), "博")
  assert.equal(extractInitial("", "U"), "U")
  assert.equal(extractInitial(undefined, "A"), "A")
})

test("resolveOidcClientName prefers clientName over clientId", () => {
  assert.equal(
    resolveOidcClientName({ clientId: "cli_1", clientName: "Plateau 博客" }),
    "Plateau 博客"
  )
  assert.equal(
    resolveOidcClientName({ clientId: "cli_1", clientName: "   " }),
    "cli_1"
  )
  assert.equal(resolveOidcClientName(null, "默认应用"), "默认应用")
})

test("describeOidcScope maps standard OIDC scopes and falls back gracefully", () => {
  assert.deepEqual(describeOidcScope("openid"), {
    scope: "openid",
    title: "基础身份标识",
    description: "验证您的基础身份标识",
    known: true,
  })
  assert.deepEqual(describeOidcScope("profile"), {
    scope: "profile",
    title: "公开个人资料",
    description: "读取您的公开个人资料（姓名、昵称、头像、语言）",
    known: true,
  })
  assert.deepEqual(describeOidcScope("email"), {
    scope: "email",
    title: "电子邮箱地址",
    description: "读取您的已验证电子邮箱地址",
    known: true,
  })
  assert.deepEqual(describeOidcScope("offline_access"), {
    scope: "offline_access",
    title: "离线访问凭证",
    description: "在您离线时保持授权并刷新访问凭证",
    known: true,
  })
  assert.deepEqual(describeOidcScope("blog:write"), {
    scope: "blog:write",
    title: "blog:write",
    description: "blog:write",
    known: false,
  })
})

test("dedupeScopes trims and preserves first-seen order", () => {
  assert.deepEqual(
    dedupeScopes([" openid ", "profile", "openid", "", "  ", "email"]),
    ["openid", "profile", "email"]
  )
  assert.deepEqual(dedupeScopes(undefined), [])
})

test("formatDateTime formats valid RFC 3339 timestamps and rejects invalid input", () => {
  const formatted = formatDateTime("2026-11-28T06:30:00Z")
  assert.equal(typeof formatted, "string")
  assert.ok((formatted ?? "").length > 0)
  assert.equal(formatDateTime(""), null)
  assert.equal(formatDateTime("invalid-date"), null)
})
