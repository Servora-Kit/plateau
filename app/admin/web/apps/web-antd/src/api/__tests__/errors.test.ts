import { AdminAuditErrorReason } from '@plateau/api/plateau/admin/audit/v1/audit.errors';
import { AdminClientErrorReason } from '@plateau/api/plateau/admin/client/v1/client.errors';
import { AdminUserErrorReason } from '@plateau/api/plateau/admin/user/v1/user.errors';
import { CrudErrorReason } from '@servora/plateau-utils/error-reasons/plateau/crud/v1/errors';
import { SecurityErrorReason } from '@servora/plateau-utils/error-reasons/plateau/security/errors/v1/errors';
import { ApiError } from '@servora/proto-utils/errors';
import { describe, expect, it } from 'vitest';

import { buildOidcLoginUrl, createAdminApi, sanitizeReturnTo } from '../client';
import { resolveAdminApiError } from '../errors';
import { createOAuthClientsApi } from '../oauth-clients';

function httpError(
  httpStatus: number,
  reason: string,
  message = 'internal secret detail',
) {
  return new ApiError({
    httpStatus,
    kind: 'http',
    message: `HTTP ${httpStatus}`,
    method: 'TestMethod',
    responseBody: {
      code: httpStatus,
      message,
      metadata: { secret: 'super-secret-token' },
      reason,
    },
    service: 'UserService',
  });
}

describe('oidc login URL builder', () => {
  it('allows valid in-site relative paths and rejects open redirects or auth loops', () => {
    expect(sanitizeReturnTo('/users?view=deleted')).toBe('/users?view=deleted');
    expect(sanitizeReturnTo(undefined)).toBe('/overview');
    expect(sanitizeReturnTo('https://evil.example/phish')).toBe('/overview');
    expect(sanitizeReturnTo('//evil.example/phish')).toBe('/overview');
    expect(sanitizeReturnTo(String.raw`/\evil.example`)).toBe('/overview');
    expect(sanitizeReturnTo('/auth/login?error=access_denied')).toBe(
      '/overview',
    );
    expect(sanitizeReturnTo('/v1/admin/auth/logout')).toBe('/overview');
    expect(buildOidcLoginUrl('/users')).toBe(
      '/v1/admin/auth/login?return_to=%2Fusers',
    );
  });
});

describe('resolveAdminApiError', () => {
  it('treats cancelled, network, and timeout errors without invalidating session', () => {
    const cancelled = resolveAdminApiError(
      new ApiError({
        kind: 'cancelled',
        message: 'aborted',
        method: 'ListUsers',
        service: 'UserService',
      }),
    );
    expect(cancelled).toMatchObject({
      category: 'cancelled',
      preserveSession: true,
      silent: true,
    });

    const network = resolveAdminApiError(
      new ApiError({
        kind: 'network',
        message: 'ECONNREFUSED',
        method: 'ListUsers',
        service: 'UserService',
      }),
    );
    expect(network).toMatchObject({
      category: 'network',
      preserveSession: true,
    });
    expect(network.message).not.toContain('ECONNREFUSED');

    const timeout = resolveAdminApiError(
      new ApiError({
        kind: 'timeout',
        message: 'timed out',
        method: 'ListUsers',
        service: 'UserService',
      }),
    );
    expect(timeout).toMatchObject({
      category: 'timeout',
      preserveSession: true,
    });
  });

  it('marks only local 401 unauthenticated as session-expiring and keeps session on 503 downstream failure', () => {
    const unauth = resolveAdminApiError(
      httpError(401, SecurityErrorReason.SECURITY_ERROR_REASON_UNAUTHENTICATED),
    );
    expect(unauth).toMatchObject({
      category: 'unauthenticated',
      preserveSession: false,
    });

    const downstream = resolveAdminApiError(
      httpError(
        503,
        AdminUserErrorReason.ADMIN_USER_ERROR_REASON_DOWNSTREAM_UNAVAILABLE,
        'client_credentials secret=xyz failed against iam:10001',
      ),
    );
    expect(downstream).toMatchObject({
      category: 'unavailable',
      preserveSession: true,
    });
    expect(downstream.message).not.toContain('secret=xyz');

    const authzUnavailable = resolveAdminApiError(
      httpError(503, SecurityErrorReason.SECURITY_ERROR_REASON_UNAVAILABLE),
    );
    expect(authzUnavailable).toMatchObject({
      category: 'unavailable',
      preserveSession: true,
    });
  });

  it('distinguishes protection, conflict, not-found, and mail failure reasons accurately', () => {
    expect(
      resolveAdminApiError(
        httpError(
          403,
          AdminUserErrorReason.ADMIN_USER_ERROR_REASON_SELF_ACTION_FORBIDDEN,
        ),
      ),
    ).toMatchObject({
      category: 'self_protected',
      preserveSession: true,
    });

    expect(
      resolveAdminApiError(
        httpError(
          403,
          AdminUserErrorReason.ADMIN_USER_ERROR_REASON_TARGET_IS_ADMIN,
        ),
      ),
    ).toMatchObject({
      category: 'target_admin',
      preserveSession: true,
    });

    const deleteMissing = resolveAdminApiError(
      httpError(
        404,
        AdminUserErrorReason.ADMIN_USER_ERROR_REASON_USER_NOT_FOUND,
      ),
      'delete',
    );
    expect(deleteMissing).toMatchObject({
      category: 'not_found',
      message: '目标用户不存在或已被删除',
      shouldRefreshRecord: true,
    });

    const createConflict = resolveAdminApiError(
      httpError(
        409,
        AdminUserErrorReason.ADMIN_USER_ERROR_REASON_USER_ALREADY_EXISTS,
      ),
      'create',
    );
    expect(createConflict.message).toContain('含已软删除账号占用');

    const undeleteConflict = resolveAdminApiError(
      httpError(
        409,
        AdminUserErrorReason.ADMIN_USER_ERROR_REASON_USER_ALREADY_EXISTS,
      ),
      'undelete',
    );
    expect(undeleteConflict.message).toBe('该用户已处于正常状态');

    expect(
      resolveAdminApiError(
        httpError(
          409,
          AdminUserErrorReason.ADMIN_USER_ERROR_REASON_ETAG_MISMATCH,
        ),
      ),
    ).toMatchObject({
      category: 'etag_mismatch',
      shouldRefreshRecord: true,
    });

    expect(
      resolveAdminApiError(
        httpError(
          503,
          AdminUserErrorReason.ADMIN_USER_ERROR_REASON_VERIFICATION_EMAIL_FAILED,
        ),
      ),
    ).toMatchObject({
      category: 'email_failed',
      preserveSession: true,
    });

    expect(
      resolveAdminApiError(
        httpError(400, CrudErrorReason.CRUD_ERROR_REASON_INVALID_FILTER),
      ),
    ).toMatchObject({
      category: 'invalid_argument',
      preserveSession: true,
    });
  });

  it('maps AdminAuditErrorReason codes while preserving admin session on 503 downstream faults', () => {
    const auditDownstream = resolveAdminApiError(
      httpError(
        503,
        AdminAuditErrorReason.ADMIN_AUDIT_ERROR_REASON_DOWNSTREAM_UNAVAILABLE,
        'clickhouse dial 127.0.0.1:19000 password=topsecret',
      ),
    );
    expect(auditDownstream).toMatchObject({
      category: 'unavailable',
      preserveSession: true,
    });
    expect(auditDownstream.message).not.toContain('topsecret');

    expect(
      resolveAdminApiError(
        httpError(
          404,
          AdminAuditErrorReason.ADMIN_AUDIT_ERROR_REASON_EVENT_NOT_FOUND,
        ),
      ),
    ).toMatchObject({
      category: 'not_found',
      preserveSession: true,
    });

    expect(
      resolveAdminApiError(
        httpError(
          400,
          AdminAuditErrorReason.ADMIN_AUDIT_ERROR_REASON_INVALID_ARGUMENT,
        ),
      ),
    ).toMatchObject({
      category: 'invalid_argument',
      preserveSession: true,
      shouldRefreshRecord: false,
    });

    expect(
      resolveAdminApiError(
        httpError(
          400,
          AdminAuditErrorReason.ADMIN_AUDIT_ERROR_REASON_INVALID_PAGE_TOKEN,
        ),
      ),
    ).toMatchObject({
      category: 'invalid_argument',
      preserveSession: true,
      shouldRefreshRecord: true,
    });
  });

  it('maps all AdminClientErrorReason codes while preserving session and redacting internals', () => {
    const api = createAdminApi();
    expect(typeof api.oauthClients.ListOAuthClients).toBe('function');
    expect(typeof createOAuthClientsApi().CreateOAuthClient).toBe('function');

    expect(
      resolveAdminApiError(
        httpError(
          409,
          AdminClientErrorReason.ADMIN_CLIENT_ERROR_REASON_ALREADY_EXISTS,
        ),
        'create',
      ),
    ).toMatchObject({
      category: 'already_exists',
      preserveSession: true,
      shouldRefreshRecord: false,
    });

    const staticErr = resolveAdminApiError(
      httpError(
        403,
        AdminClientErrorReason.ADMIN_CLIENT_ERROR_REASON_STATIC_CLIENT_IMMUTABLE,
      ),
      'update',
    );
    expect(staticErr).toMatchObject({
      category: 'static_immutable',
      preserveSession: true,
    });
    expect(staticErr.message).toContain('oidc.yaml');

    expect(
      resolveAdminApiError(
        httpError(
          400,
          AdminClientErrorReason.ADMIN_CLIENT_ERROR_REASON_INVALID_CONFIG,
        ),
      ),
    ).toMatchObject({
      category: 'invalid_argument',
      preserveSession: true,
    });

    expect(
      resolveAdminApiError(
        httpError(
          409,
          AdminClientErrorReason.ADMIN_CLIENT_ERROR_REASON_INVALID_STATE,
        ),
      ),
    ).toMatchObject({
      category: 'invalid_state',
      preserveSession: true,
      shouldRefreshRecord: true,
    });

    expect(
      resolveAdminApiError(
        httpError(
          409,
          AdminClientErrorReason.ADMIN_CLIENT_ERROR_REASON_ETAG_MISMATCH,
        ),
      ),
    ).toMatchObject({
      category: 'etag_mismatch',
      preserveSession: true,
      shouldRefreshRecord: true,
    });

    const limitErr = resolveAdminApiError(
      httpError(
        409,
        AdminClientErrorReason.ADMIN_CLIENT_ERROR_REASON_SECRET_LIMIT_EXCEEDED,
      ),
    );
    expect(limitErr).toMatchObject({
      category: 'invalid_state',
      preserveSession: true,
      shouldRefreshRecord: true,
    });
    expect(limitErr.message).toContain('5 把');

    const lastSecretErr = resolveAdminApiError(
      httpError(
        409,
        AdminClientErrorReason.ADMIN_CLIENT_ERROR_REASON_LAST_ACTIVE_SECRET,
      ),
    );
    expect(lastSecretErr).toMatchObject({
      category: 'invalid_state',
      preserveSession: true,
      shouldRefreshRecord: true,
    });
    expect(lastSecretErr.message).toContain('至少需保留一把有效密钥');

    expect(
      resolveAdminApiError(
        httpError(
          404,
          AdminClientErrorReason.ADMIN_CLIENT_ERROR_REASON_NOT_FOUND,
        ),
        'delete',
      ),
    ).toMatchObject({
      category: 'not_found',
      message: '目标 OAuth 客户端不存在或已被删除',
      preserveSession: true,
      shouldRefreshRecord: true,
    });

    const clientDownstream = resolveAdminApiError(
      httpError(
        503,
        AdminClientErrorReason.ADMIN_CLIENT_ERROR_REASON_DOWNSTREAM_UNAVAILABLE,
        'iam grpc dial failed client_secret=pcs_super_secret',
      ),
    );
    expect(clientDownstream).toMatchObject({
      category: 'unavailable',
      preserveSession: true,
    });
    expect(clientDownstream.message).not.toContain('pcs_super_secret');
  });

  it('redacts unknown exceptions without echoing raw message or stack', () => {
    const unknown = resolveAdminApiError(
      new Error('password=PlainSecret123! stack trace'),
    );
    expect(unknown).toEqual({
      category: 'unknown',
      message: '服务处理异常，请稍后重试',
      preserveSession: true,
      shouldRefreshRecord: false,
      silent: false,
    });
  });
});
