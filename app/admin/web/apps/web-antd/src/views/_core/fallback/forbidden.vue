<script lang="ts" setup>
import { computed } from 'vue';
import { useRouter } from 'vue-router';

import { LOGIN_PATH } from '@vben/constants';

import { Button, Result } from 'ant-design-vue';

import { useAuthStore } from '#/store';

defineOptions({ name: 'AdminForbiddenPage' });

const router = useRouter();
const authStore = useAuthStore();

const subtitle = computed(
  () =>
    authStore.sessionErrorMessage ||
    '当前账号未获授平台管理员访问权限，无法进入管理工作台。',
);

async function handleLogoutAndSwitch() {
  await authStore.logout(false);
}

function handleBackToLogin() {
  router.replace(LOGIN_PATH);
}
</script>

<template>
  <div class="flex min-h-full items-center justify-center p-6">
    <Result status="403" :sub-title="subtitle" title="无管理访问权限">
      <template #extra>
        <div class="flex flex-wrap justify-center gap-3">
          <Button
            :loading="authStore.logoutLoading"
            type="primary"
            @click="handleLogoutAndSwitch"
          >
            退出当前管理会话并重新登录
          </Button>
          <Button @click="handleBackToLogin">返回登录页</Button>
        </div>
      </template>
    </Result>
  </div>
</template>
