import type * as VueRouterModule from 'vue-router';

import type { AdminApi } from '#/api/client';

import { useAccessStore, useUserStore } from '@vben/stores';

import { AdminUserErrorReason } from '@plateau/api/plateau/admin/user/v1/user.errors';
import { SecurityErrorReason } from '@plateau/api/plateau/security/errors/v1/errors.errors';
import { ApiError } from '@servora/proto-utils/errors';
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import * as apiModule from '#/api/client';

import { resolveUserDisplayName, useAuthStore } from '../auth';

const getSessionMock = vi.fn();
const logoutMock = vi.fn();
const replaceMock = vi.fn();

vi.mock('vue-router', async (importOriginal) => {
  const actual = await importOriginal<typeof VueRouterModule>();
  return {
    ...actual,
    useRouter: () => ({
      currentRoute: { value: { fullPath: '/users?view=deleted' } },
      replace: replaceMock,
    }),
  };
});

describe('useAuthStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    getSessionMock.mockReset();
    logoutMock.mockReset();
    replaceMock.mockReset();
    vi.spyOn(apiModule, 'createAdminApi').mockReturnValue({
      audit: {} as AdminApi['audit'],
      auth: {
        GetSession: getSessionMock,
        Logout: logoutMock,
      },
      users: {} as AdminApi['users'],
    });
  });

  it('deduplicates concurrent ensureSession calls and populates user + permissions in memory only', async () => {
    const { promise, resolve: resolveCall } = Promise.withResolvers<unknown>();
    getSessionMock.mockReturnValue(promise);

    const authStore = useAuthStore();
    const userStore = useUserStore();
    const accessStore = useAccessStore();

    const p1 = authStore.ensureSession();
    const p2 = authStore.ensureSession();

    resolveCall({
      permissions: ['access', 'manage_users', 'expunge_users'],
      user: {
        email: 'admin@plateau.local',
        profile: {
          name: 'Platform Admin',
          picture: 'https://img.example/a.png',
        },
        status: 'USER_STATUS_ACTIVE',
        userId: 'u_admin',
      },
    });

    const [s1, s2] = await Promise.all([p1, p2]);
    expect(s1).toBe('authenticated');
    expect(s2).toBe('authenticated');
    expect(getSessionMock).toHaveBeenCalledTimes(1);
    expect(authStore.hasPermission('manage_users')).toBe(true);
    expect(authStore.hasPermission('expunge_users')).toBe(true);
    expect(userStore.userInfo?.realName).toBe('Platform Admin');
    expect(accessStore.accessCodes).toEqual([
      'access',
      'manage_users',
      'expunge_users',
    ]);
  });

  it('rejects authenticated user without access permission as forbidden', async () => {
    getSessionMock.mockResolvedValue({
      permissions: [],
      user: { email: 'member@plateau.local', userId: 'u_member' },
    });

    const authStore = useAuthStore();
    const status = await authStore.ensureSession();
    expect(status).toBe('forbidden');
    expect(authStore.currentUser).toBeNull();
  });

  it('keeps unavailable status on 503 downstream failure without marking session unauthenticated, and recovers on retry', async () => {
    getSessionMock.mockRejectedValueOnce(
      new ApiError({
        httpStatus: 503,
        kind: 'http',
        message: 'HTTP 503',
        method: 'GetSession',
        responseBody: {
          code: 503,
          message: 'openfga unavailable',
          reason:
            AdminUserErrorReason.ADMIN_USER_ERROR_REASON_DOWNSTREAM_UNAVAILABLE,
        },
        service: 'AuthService',
      }),
    );

    const authStore = useAuthStore();
    expect(await authStore.ensureSession()).toBe('unavailable');
    expect(authStore.sessionStatus).toBe('unavailable');

    getSessionMock.mockResolvedValueOnce({
      permissions: ['access', 'manage_users'],
      user: { email: 'admin@plateau.local', userId: 'u_admin' },
    });

    expect(await authStore.retrySession()).toBe('authenticated');
    expect(authStore.currentUser?.userId).toBe('u_admin');
  });

  it('marks unauthenticated on 401 and destroys only local session on logout', async () => {
    getSessionMock.mockRejectedValueOnce(
      new ApiError({
        httpStatus: 401,
        kind: 'http',
        message: 'HTTP 401',
        method: 'GetSession',
        responseBody: {
          code: 401,
          message: 'no session',
          reason: SecurityErrorReason.SECURITY_ERROR_REASON_UNAUTHENTICATED,
        },
        service: 'AuthService',
      }),
    );

    const authStore = useAuthStore();
    expect(await authStore.ensureSession()).toBe('unauthenticated');

    logoutMock.mockResolvedValueOnce({});
    await authStore.logout(true);
    expect(logoutMock).toHaveBeenCalledTimes(1);
    expect(replaceMock).toHaveBeenCalledWith({
      path: '/auth/login',
      query: { redirect: '/users?view=deleted' },
    });
  });

  it('invalidates accessStore.isAccessChecked when refreshed session permissions change', async () => {
    getSessionMock.mockResolvedValueOnce({
      permissions: ['access', 'read_audit'],
      user: { email: 'admin@plateau.local', userId: 'u_admin' },
    });

    const authStore = useAuthStore();
    const accessStore = useAccessStore();
    expect(await authStore.ensureSession()).toBe('authenticated');
    accessStore.setIsAccessChecked(true);

    getSessionMock.mockResolvedValueOnce({
      permissions: ['access'],
      user: { email: 'admin@plateau.local', userId: 'u_admin' },
    });
    expect(await authStore.ensureSession(true)).toBe('authenticated');
    expect(authStore.hasPermission('read_audit')).toBe(false);
    expect(accessStore.isAccessChecked).toBe(false);
  });

  it('resolves user display name by priority', () => {
    expect(
      resolveUserDisplayName({
        email: 'a@b.com',
        profile: { name: 'Alice', nickname: 'Ali' },
      }),
    ).toBe('Alice');
    expect(
      resolveUserDisplayName({
        email: 'a@b.com',
        profile: { nickname: 'Ali' },
      }),
    ).toBe('Ali');
    expect(resolveUserDisplayName({ email: 'a@b.com' })).toBe('a@b.com');
  });
});
