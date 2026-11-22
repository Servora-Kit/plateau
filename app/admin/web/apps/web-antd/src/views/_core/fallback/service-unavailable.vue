<script lang="ts" setup>
import { computed, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';

import { Button, Result } from 'ant-design-vue';

import { sanitizeReturnTo } from '#/api';
import { useAuthStore } from '#/store';

defineOptions({ name: 'AdminServiceUnavailablePage' });

const route = useRoute();
const router = useRouter();
const authStore = useAuthStore();
const retrying = ref(false);

const subtitle = computed(
  () =>
    authStore.sessionErrorMessage ||
    '下游身份或权限服务暂时不可用，当前本地管理会话仍保留，恢复后可直接重试。',
);

const targetPath = computed(() => {
  const raw = Array.isArray(route.query.redirect)
    ? route.query.redirect[0]
    : route.query.redirect;
  return sanitizeReturnTo(raw);
});

async function handleRetry() {
  retrying.value = true;
  try {
    const status = await authStore.retrySession();
    if (status === 'authenticated') {
      await router.replace(targetPath.value);
    } else if (status === 'unauthenticated') {
      await router.replace({
        path: '/auth/login',
        query:
          targetPath.value === '/overview'
            ? {}
            : { redirect: targetPath.value },
      });
    } else if (status === 'forbidden') {
      await router.replace('/forbidden');
    }
  } finally {
    retrying.value = false;
  }
}
</script>

<template>
  <div class="flex min-h-full items-center justify-center p-6">
    <Result status="500" :sub-title="subtitle" title="服务暂时不可用">
      <template #extra>
        <Button :loading="retrying" type="primary" @click="handleRetry">
          重试连接
        </Button>
      </template>
    </Result>
  </div>
</template>
