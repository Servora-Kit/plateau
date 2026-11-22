import type { Router } from 'vue-router';

import { LOGIN_PATH } from '@vben/constants';
import { preferences } from '@vben/preferences';
import { useAccessStore } from '@vben/stores';
import { startProgress, stopProgress } from '@vben/utils';

import { sanitizeReturnTo } from '#/api';
import { accessRoutes, coreRouteNames } from '#/router/routes';
import { useAuthStore } from '#/store';

import { generateAccess } from './access';

function setupCommonGuard(router: Router) {
  const loadedPaths = new Set<string>();

  router.beforeEach((to) => {
    to.meta.loaded = loadedPaths.has(to.path);
    if (!to.meta.loaded && preferences.transition.progress) {
      startProgress();
    }
    return true;
  });

  router.afterEach((to) => {
    loadedPaths.add(to.path);
    if (preferences.transition.progress) {
      stopProgress();
    }
  });
}

function setupAccessGuard(router: Router) {
  router.beforeEach(async (to, from) => {
    const accessStore = useAccessStore();
    const authStore = useAuthStore();

    if (coreRouteNames.includes(to.name as string)) {
      if (to.path === LOGIN_PATH) {
        // 携带后端 OIDC 回调错误参数时直接展示登录页提示，避免重探测死循环
        if (to.query.error) {
          return true;
        }
        if (authStore.sessionStatus === 'authenticated') {
          const rawRedirect = Array.isArray(to.query.redirect)
            ? to.query.redirect[0]
            : to.query.redirect;
          return sanitizeReturnTo(rawRedirect);
        }
      }
      return true;
    }

    if (to.meta.ignoreAccess) {
      return true;
    }

    const status = await authStore.ensureSession();

    if (status === 'unauthenticated') {
      return {
        path: LOGIN_PATH,
        query:
          to.fullPath === preferences.app.defaultHomePath
            ? {}
            : { redirect: to.fullPath },
        replace: true,
      };
    }

    if (status === 'forbidden') {
      return {
        path: '/forbidden',
        replace: true,
      };
    }

    if (status === 'unavailable') {
      return {
        path: '/service-unavailable',
        query: { redirect: to.fullPath },
        replace: true,
      };
    }

    if (accessStore.isAccessChecked) {
      return true;
    }

    const { accessibleMenus, accessibleRoutes } = await generateAccess({
      roles: ['admin'],
      router,
      routes: accessRoutes,
    });

    accessStore.setAccessMenus(accessibleMenus);
    accessStore.setAccessRoutes(accessibleRoutes);
    accessStore.setIsAccessChecked(true);

    const rawFromRedirect = Array.isArray(from.query.redirect)
      ? from.query.redirect[0]
      : from.query.redirect;
    let redirectPath =
      to.path === '/' ? preferences.app.defaultHomePath : to.fullPath;
    if (rawFromRedirect) {
      redirectPath = sanitizeReturnTo(rawFromRedirect);
    }

    return {
      ...router.resolve(redirectPath),
      replace: true,
    };
  });
}

function createRouterGuard(router: Router) {
  setupCommonGuard(router);
  setupAccessGuard(router);
}

export { createRouterGuard };
