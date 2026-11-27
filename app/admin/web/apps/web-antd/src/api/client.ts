import type { AuditService } from '@plateau/api/plateau/admin/audit/v1';
import type { AuthService } from '@plateau/api/plateau/admin/auth/v1';
import type { UserService } from '@plateau/api/plateau/admin/user/v1';

import { createAuditServiceClient } from '@plateau/api/plateau/admin/audit/v1';
import { createAuthServiceClient } from '@plateau/api/plateau/admin/auth/v1';
import { createUserServiceClient } from '@plateau/api/plateau/admin/user/v1';
import {
  createHttpClient,
  createTransport,
} from '@servora/plateau-utils/client';

const DEFAULT_RETURN_PATH = '/overview';

export interface AdminApi {
  audit: AuditService;
  auth: AuthService;
  users: UserService;
}

export const adminHttpClient = createHttpClient({
  baseURL: '/',
  credentials: 'same-origin',
  retry: 0,
  service: 'admin',
  timeout: 10_000,
});

export function createAdminApi(signal?: AbortSignal): AdminApi {
  const transport = createTransport(adminHttpClient, { signal });
  return {
    audit: createAuditServiceClient(transport),
    auth: createAuthServiceClient(transport),
    users: createUserServiceClient(transport),
  };
}

export function sanitizeReturnTo(returnTo?: null | string): string {
  if (!returnTo) {
    return DEFAULT_RETURN_PATH;
  }
  const trimmed = returnTo.trim();
  if (
    !trimmed.startsWith('/') ||
    trimmed.startsWith('//') ||
    trimmed.includes('\\') ||
    trimmed === '/auth' ||
    trimmed.startsWith('/auth/') ||
    trimmed.startsWith('/v1/')
  ) {
    return DEFAULT_RETURN_PATH;
  }
  return trimmed;
}

export function buildOidcLoginUrl(returnTo?: null | string): string {
  const target = sanitizeReturnTo(returnTo);
  return `/v1/admin/auth/login?return_to=${encodeURIComponent(target)}`;
}
