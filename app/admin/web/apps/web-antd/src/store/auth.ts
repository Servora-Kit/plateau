import type { User } from '@plateau/api/plateau/iam/user/v1';

import { computed, ref } from 'vue';
import { useRouter } from 'vue-router';

import { LOGIN_PATH } from '@vben/constants';
import { resetAllStores, useAccessStore, useUserStore } from '@vben/stores';

import { defineStore } from 'pinia';

import {
  buildOidcLoginUrl,
  createAdminApi,
  resolveAdminApiError,
  sanitizeReturnTo,
} from '#/api';

export type SessionStatus =
  | 'authenticated'
  | 'forbidden'
  | 'unauthenticated'
  | 'unavailable'
  | 'unknown';

export function resolveUserDisplayName(user?: null | User): string {
  const profile = user?.profile;
  return (
    profile?.name?.trim() ||
    profile?.nickname?.trim() ||
    profile?.preferredUsername?.trim() ||
    user?.email?.trim() ||
    user?.userId ||
    '管理员'
  );
}

export const useAuthStore = defineStore('auth', () => {
  const accessStore = useAccessStore();
  const userStore = useUserStore();
  const router = useRouter();

  const sessionStatus = ref<SessionStatus>('unknown');
  const currentUser = ref<null | User>(null);
  const permissions = ref<string[]>([]);
  const sessionErrorMessage = ref('');
  const logoutLoading = ref(false);

  let inflightSessionPromise: null | Promise<SessionStatus> = null;

  const permissionSet = computed(() => new Set(permissions.value));

  function hasPermission(code: string): boolean {
    return permissionSet.value.has(code);
  }

  function haveSamePermissions(a: string[], b: string[]): boolean {
    if (a.length !== b.length) {
      return false;
    }
    const left = [...a].toSorted();
    const right = [...b].toSorted();
    return left.every((code, index) => code === right[index]);
  }

  function syncUserSnapshot(user: User, nextPermissions?: string[]) {
    currentUser.value = user;
    if (nextPermissions !== undefined) {
      const changed = !haveSamePermissions(permissions.value, nextPermissions);
      permissions.value = [...nextPermissions];
      accessStore.setAccessCodes([...nextPermissions]);
      if (changed && accessStore.isAccessChecked) {
        accessStore.setIsAccessChecked(false);
      }
    }
    const displayName = resolveUserDisplayName(user);
    userStore.setUserInfo({
      avatar: user.profile?.picture?.trim() || '',
      homePath: '/overview',
      realName: displayName,
      roles: ['admin'],
      userId: user.userId || '',
      username: user.email || '',
    });
  }

  function clearMemorySession(nextStatus: SessionStatus, errorMsg = '') {
    currentUser.value = null;
    permissions.value = [];
    sessionErrorMessage.value = errorMsg;
    sessionStatus.value = nextStatus;
    accessStore.setAccessCodes([]);
    accessStore.setAccessToken(null);
    accessStore.setIsAccessChecked(false);
    accessStore.setLoginExpired(false);
    userStore.setUserInfo(null);
  }

  function markUnauthenticated() {
    clearMemorySession('unauthenticated');
  }

  async function ensureSession(force = false): Promise<SessionStatus> {
    if (!force && sessionStatus.value === 'authenticated') {
      return 'authenticated';
    }
    if (!force && inflightSessionPromise) {
      return inflightSessionPromise;
    }

    let activePromise: null | Promise<SessionStatus> = null;
    activePromise = (async (): Promise<SessionStatus> => {
      try {
        const response = await createAdminApi().auth.GetSession({});
        const granted = response.permissions ?? [];
        if (!response.user || !granted.includes('access')) {
          clearMemorySession(
            'forbidden',
            '当前 IAM 账号未获授平台管理员访问权限，请切换管理员账号后再试。',
          );
          return 'forbidden';
        }
        sessionErrorMessage.value = '';
        sessionStatus.value = 'authenticated';
        syncUserSnapshot(response.user, granted);
        return 'authenticated';
      } catch (error) {
        const resolved = resolveAdminApiError(error);
        if (resolved.category === 'unauthenticated') {
          markUnauthenticated();
          return 'unauthenticated';
        }
        if (resolved.category === 'forbidden') {
          clearMemorySession('forbidden', resolved.message);
          return 'forbidden';
        }
        sessionErrorMessage.value = resolved.message;
        sessionStatus.value = 'unavailable';
        return 'unavailable';
      } finally {
        if (inflightSessionPromise === activePromise) {
          inflightSessionPromise = null;
        }
      }
    })();

    inflightSessionPromise = activePromise;
    return activePromise;
  }

  async function retrySession(): Promise<SessionStatus> {
    sessionStatus.value = 'unknown';
    return ensureSession(true);
  }

  function startOidcLogin(
    returnTo?: null | string,
    navigate: (url: string) => void = (url) => window.location.assign(url),
  ) {
    navigate(buildOidcLoginUrl(returnTo));
  }

  async function logout(withRedirect = false) {
    logoutLoading.value = true;
    const currentFullPath = router?.currentRoute?.value?.fullPath;
    try {
      await createAdminApi().auth.Logout({});
    } catch {
      // 本地会话即使已在服务端失效，仍正常清理前端内存态
    } finally {
      logoutLoading.value = false;
    }
    resetAllStores();
    clearMemorySession('unauthenticated');
    if (router) {
      const safeRedirect = sanitizeReturnTo(currentFullPath);
      await router.replace({
        path: LOGIN_PATH,
        query:
          withRedirect && safeRedirect !== '/overview'
            ? { redirect: safeRedirect }
            : {},
      });
    }
  }

  function $reset() {
    inflightSessionPromise = null;
    logoutLoading.value = false;
    sessionStatus.value = 'unknown';
    currentUser.value = null;
    permissions.value = [];
    sessionErrorMessage.value = '';
  }

  return {
    $reset,
    currentUser,
    ensureSession,
    hasPermission,
    logout,
    logoutLoading,
    markUnauthenticated,
    permissions,
    permissionSet,
    retrySession,
    sessionErrorMessage,
    sessionStatus,
    startOidcLogin,
    syncUserSnapshot,
  };
});
