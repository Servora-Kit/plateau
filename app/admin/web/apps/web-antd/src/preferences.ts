import { defineOverridesPreferences } from '@vben/preferences';

export const overridesPreferences = defineOverridesPreferences({
  app: {
    accessMode: 'frontend',
    defaultHomePath: '/overview',
    enableRefreshToken: false,
    loginExpiredMode: 'page',
    name: import.meta.env.VITE_APP_TITLE,
  },
  copyright: {
    enable: false,
  },
  shortcutKeys: {
    globalLockScreen: false,
    globalSearch: false,
  },
  widget: {
    globalSearch: false,
    languageToggle: false,
    lockScreen: false,
    notification: false,
  },
});
