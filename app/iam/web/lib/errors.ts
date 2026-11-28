import {
  AuthnErrorReason,
  isAuthnErrorReason,
} from "@plateau/api/plateau/iam/authn/v1/authn.errors"
import { ApiError, parseKratosError } from "@servora/proto-utils/errors"

export function authnErrorReason(error: unknown): AuthnErrorReason | null {
  if (!(error instanceof ApiError)) return null
  const reason = parseKratosError(error)?.reason
  return isAuthnErrorReason(reason) ? reason : null
}
const invalidTokenReasons: Record<string, true> = {
  ACCOUNT_ERROR_REASON_INVALID_TOKEN: true,
  ACCOUNT_ERROR_REASON_TOKEN_EXPIRED: true,
}

export function isUnauthenticated(error: unknown): boolean {
  if (!(error instanceof ApiError)) return false
  return (
    error.httpStatus === 401 ||
    parseKratosError(error)?.reason === "ACCOUNT_ERROR_REASON_UNAUTHENTICATED"
  )
}

export function isConflict(error: unknown): boolean {
  return error instanceof ApiError && error.httpStatus === 409
}

export function isInvalidOneTimeToken(error: unknown): boolean {
  if (!(error instanceof ApiError)) return false
  const reason = parseKratosError(error)?.reason
  return reason !== undefined && invalidTokenReasons[reason] === true
}

export function isInvalidPassword(error: unknown): boolean {
  if (!(error instanceof ApiError)) return false
  const reason = parseKratosError(error)?.reason
  return (
    reason === "ACCOUNT_ERROR_REASON_INVALID_PASSWORD" ||
    reason === AuthnErrorReason.AUTHN_ERROR_REASON_INVALID_PASSWORD
  )
}

export function isAccountInvalidState(error: unknown): boolean {
  return (
    error instanceof ApiError &&
    parseKratosError(error)?.reason === "ACCOUNT_ERROR_REASON_INVALID_STATE"
  )
}

export function isAlreadyRegistered(error: unknown): boolean {
  return (
    error instanceof ApiError &&
    parseKratosError(error)?.reason ===
      "ACCOUNT_ERROR_REASON_EMAIL_ALREADY_REGISTERED"
  )
}

export function isConsentRequestInvalid(error: unknown): boolean {
  if (!(error instanceof ApiError)) return false
  if (
    error.httpStatus === 400 ||
    error.httpStatus === 404 ||
    error.httpStatus === 410
  ) {
    return true
  }
  const reason = authnErrorReason(error)
  return (
    reason === AuthnErrorReason.AUTHN_ERROR_REASON_OIDC_REQUEST_INVALID ||
    reason === AuthnErrorReason.AUTHN_ERROR_REASON_FLOW_EXPIRED ||
    reason === AuthnErrorReason.AUTHN_ERROR_REASON_FLOW_NOT_FOUND ||
    reason === AuthnErrorReason.AUTHN_ERROR_REASON_FLOW_INVALIDATED
  )
}

export function requestErrorMessage(
  error: unknown,
  fallback = "操作未完成，请重试"
): string {
  if (!(error instanceof ApiError)) return fallback
  if (error.kind === "network") return "无法连接服务，请检查网络后重试"
  if (error.kind === "timeout") return "请求超时，请确认网络状态后重试"
  if (error.kind === "cancelled") return "请求已取消"
  if (error.httpStatus === 403) {
    return "请求被拒绝，请检查访问地址或权限配置"
  }
  if (error.httpStatus !== undefined && error.httpStatus >= 500) {
    return "服务暂时不可用，请稍后重试"
  }
  return fallback
}
