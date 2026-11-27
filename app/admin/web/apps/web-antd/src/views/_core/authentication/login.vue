<script lang="ts" setup>
import { computed, ref } from 'vue';
import { useRoute } from 'vue-router';

import { Alert, Button } from 'ant-design-vue';

import { sanitizeReturnTo } from '#/api';
import { useAuthStore } from '#/store';

defineOptions({ name: 'AdminOidcLogin' });

const route = useRoute();
const authStore = useAuthStore();
const redirecting = ref(false);

interface CallbackNotice {
  description: string;
  message: string;
  type: 'error' | 'info' | 'warning';
}

const callbackNotice = computed<CallbackNotice | null>(() => {
  const rawError = Array.isArray(route.query.error)
    ? route.query.error[0]
    : route.query.error;
  switch (rawError) {
    case 'access_denied': {
      return {
        description:
          '当前 IAM 账号未获授平台管理员访问权限，请前往 IAM 切换具备管理权限的账号后再试。',
        message: '无平台管理访问权限',
        type: 'warning',
      };
    }
    case 'login_cancelled': {
      return {
        description:
          '您已取消本次统一身份认证，如需进入管理台请重新点击下方按钮发起登录。',
        message: '已取消登录',
        type: 'info',
      };
    }
    case 'login_failed': {
      return {
        description: '登录握手已过期或未完成，请重新点击下方按钮发起认证。',
        message: '登录握手未完成',
        type: 'info',
      };
    }
    case 'service_unavailable': {
      return {
        description: '认证服务或权限引擎暂时不可用，请稍后重试。',
        message: '下游服务暂时不可用',
        type: 'error',
      };
    }
    default: {
      return null;
    }
  }
});

const targetReturnTo = computed(() => {
  const rawRedirect = Array.isArray(route.query.redirect)
    ? route.query.redirect[0]
    : route.query.redirect;
  if (!rawRedirect) {
    return '/overview';
  }
  try {
    return sanitizeReturnTo(decodeURIComponent(rawRedirect));
  } catch {
    return '/overview';
  }
});

function handleOidcLogin() {
  redirecting.value = true;
  authStore.startOidcLogin(targetReturnTo.value);
}
</script>

<template>
  <div class="mx-auto flex w-full max-w-md flex-col gap-6 py-4">
    <div class="space-y-2">
      <h2 class="text-foreground text-2xl font-semibold tracking-tight">
        管理员登录
      </h2>
      <p class="text-muted-foreground text-sm">
        仅限配置授权的平台管理员访问，身份凭证由 Plateau IAM 统一验证。
      </p>
    </div>

    <Alert
      v-if="callbackNotice"
      :description="callbackNotice.description"
      :message="callbackNotice.message"
      show-icon
      :type="callbackNotice.type"
    />

    <Button
      block
      :loading="redirecting"
      size="large"
      type="primary"
      @click="handleOidcLogin"
    >
      使用 Plateau IAM 登录
    </Button>
  </div>
</template>
