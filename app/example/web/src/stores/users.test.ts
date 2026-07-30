import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { ApiError, type ApiErrorKind } from '@servora/proto-utils/errors'

import { UserErrorReason, isUserErrorReason } from '@/api/generated/example/service/v1/user.errors'
import { useUsersStore } from '@/stores/users'

const { getUser } = vi.hoisted(() => ({ getUser: vi.fn() }))

vi.mock('@/api/userApi', () => ({
  userApi: { GetUser: getUser },
}))

beforeEach(() => {
  setActivePinia(createPinia())
  getUser.mockReset()
})

function rejectedRequest(kind: ApiErrorKind, reason = '', message = 'request failed'): ApiError {
  return new ApiError({
    kind,
    message: 'request failed',
    httpStatus: kind === 'http' ? 409 : undefined,
    responseBody:
      kind === 'http'
        ? {
            code: 409,
            reason,
            message,
          }
        : undefined,
    service: 'UserHTTPService',
    method: 'GetUser',
  })
}

async function readFailure(error: ApiError): Promise<string> {
  getUser.mockRejectedValueOnce(error)
  const store = useUsersStore()

  await store.getUser('tenants/demo/users/ada')

  return store.error
}

describe('generated UserErrorReason contract', () => {
  it('exposes every reason and rejects non-members', () => {
    expect(UserErrorReason).toEqual({
      USER_ERROR_REASON_UNSPECIFIED: 'USER_ERROR_REASON_UNSPECIFIED',
      USER_ERROR_REASON_NOT_FOUND: 'USER_ERROR_REASON_NOT_FOUND',
      USER_ERROR_REASON_ALREADY_EXISTS: 'USER_ERROR_REASON_ALREADY_EXISTS',
      USER_ERROR_REASON_ETAG_MISMATCH: 'USER_ERROR_REASON_ETAG_MISMATCH',
    })

    for (const reason of Object.values(UserErrorReason)) {
      expect(isUserErrorReason(reason)).toBe(true)
    }
    for (const reason of [
      'USER_ERROR_REASON_FUTURE',
      'user_error_reason_not_found',
      'toString',
      null,
      404,
      {},
    ]) {
      expect(isUserErrorReason(reason)).toBe(false)
    }
  })
})

describe('User error presentation', () => {
  it.each([
    [UserErrorReason.USER_ERROR_REASON_ALREADY_EXISTS, '用户已存在'],
    [UserErrorReason.USER_ERROR_REASON_ETAG_MISMATCH, '用户数据已更新，请刷新后重试'],
    [UserErrorReason.USER_ERROR_REASON_NOT_FOUND, '用户不存在'],
  ])('maps %s to application-owned copy', async (reason, expected) => {
    await expect(readFailure(rejectedRequest('http', reason, 'backend detail'))).resolves.toBe(
      expected,
    )
  })

  it('uses readable backend copy for a valid but unmapped reason', async () => {
    await expect(
      readFailure(
        rejectedRequest('http', UserErrorReason.USER_ERROR_REASON_UNSPECIFIED, '后端拒绝了请求'),
      ),
    ).resolves.toBe('后端拒绝了请求')
  })

  it.each([
    [UserErrorReason.USER_ERROR_REASON_UNSPECIFIED, 'USER_ERROR_REASON_UNSPECIFIED'],
    ['USER_ERROR_REASON_FUTURE', 'USER_ERROR_REASON_FUTURE'],
  ])('does not display machine-code fallback %s', async (reason, message) => {
    await expect(readFailure(rejectedRequest('http', reason, message))).resolves.toBe('请求失败')
  })

  it.each([
    ['network' as const, '网络连接失败，请检查网络设置'],
    ['timeout' as const, '请求超时，请稍后重试'],
  ])('keeps the %s fallback', async (kind, expected) => {
    await expect(readFailure(rejectedRequest(kind))).resolves.toBe(expected)
  })
})
