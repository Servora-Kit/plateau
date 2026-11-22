<script lang="ts" setup>
import { computed, watch } from 'vue';

import { useWatermark } from '@vben/hooks';
import { BasicLayout, UserDropdown } from '@vben/layouts';
import { preferences, usePreferences } from '@vben/preferences';
import { useUserStore } from '@vben/stores';

import { resolveUserDisplayName, useAuthStore } from '#/store';

const userStore = useUserStore();
const authStore = useAuthStore();
const { destroyWatermark, updateWatermark } = useWatermark();
const { isDark } = usePreferences();

const displayName = computed(
  () =>
    userStore.userInfo?.realName ||
    resolveUserDisplayName(authStore.currentUser),
);

const userEmail = computed(
  () => authStore.currentUser?.email || userStore.userInfo?.username || '',
);

const avatar = computed(
  () =>
    authStore.currentUser?.profile?.picture?.trim() ||
    userStore.userInfo?.avatar ||
    preferences.app.defaultAvatar,
);

async function handleLogout() {
  await authStore.logout(false);
}

watch(
  () => ({
    content: preferences.app.watermarkContent,
    email: userEmail.value,
    enable: preferences.app.watermark,
    isDark: isDark.value,
    name: displayName.value,
  }),
  async ({ content, email, enable, isDark: isDarkValue, name }) => {
    if (enable) {
      const watermarkColor = isDarkValue
        ? 'rgba(255, 255, 255, 0.12)'
        : 'rgba(0, 0, 0, 0.12)';
      await updateWatermark({
        advancedStyle: {
          colorStops: [
            { color: watermarkColor, offset: 0 },
            { color: watermarkColor, offset: 1 },
          ],
          type: 'linear',
        },
        content: content || [name, email].filter(Boolean).join(' · '),
      });
    } else {
      destroyWatermark();
    }
  },
  { immediate: true },
);
</script>

<template>
  <BasicLayout
    :avatar="avatar"
    :text="displayName"
    @clear-preferences-and-logout="handleLogout"
    @logout="handleLogout"
  >
    <template #user-dropdown>
      <UserDropdown
        :avatar="avatar"
        :description="userEmail"
        :menus="[]"
        tag-text="管理员"
        :text="displayName"
        @clear-preferences-and-logout="handleLogout"
        @logout="handleLogout"
      />
    </template>
  </BasicLayout>
</template>
