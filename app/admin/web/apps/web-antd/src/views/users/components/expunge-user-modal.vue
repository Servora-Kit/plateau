<script lang="ts" setup>
import type { User } from '@plateau/api/plateau/iam/user/v1';

import { computed, ref, watch } from 'vue';
import { useRouter } from 'vue-router';

import { useVbenModal } from '@vben/common-ui';
import { LOGIN_PATH } from '@vben/constants';

import { Alert, Descriptions, Input, message } from 'ant-design-vue';

import { createAdminApi, resolveAdminApiError } from '#/api';
import { useAuthStore } from '#/store';

import { isExpungeEmailConfirmed, resolveUserResourceName } from '../schemas';

export interface ExpungeUserModalPayload {
  user: User;
}

const emit = defineEmits<{
  expunged: [user: User];
}>();

const router = useRouter();
const authStore = useAuthStore();

const targetUser = ref<null | User>(null);
const confirmEmail = ref('');

const emailMatched = computed(() =>
  isExpungeEmailConfirmed(confirmEmail.value, targetUser.value?.email),
);

const [Modal, modalApi] = useVbenModal<ExpungeUserModalPayload>({
  confirmText: '确认永久删除',
  onCancel() {
    modalApi.close();
  },
  async onConfirm() {
    if (!targetUser.value || !emailMatched.value) {
      return;
    }
    const userSnapshot = targetUser.value;
    const name = resolveUserResourceName(userSnapshot);

    modalApi.setState({ confirmLoading: true });
    try {
      await createAdminApi().users.ExpungeUser({ name });
      modalApi.close();
      message.success('账号已永久物理删除，原登录邮箱已释放');
      emit('expunged', userSnapshot);
    } catch (error) {
      const resolved = resolveAdminApiError(error);
      if (resolved.silent) {
        return;
      }
      if (!resolved.preserveSession) {
        modalApi.close();
        message.warning(resolved.message);
        authStore.markUnauthenticated();
        await router.replace({
          path: LOGIN_PATH,
          query: { redirect: router.currentRoute.value.fullPath },
        });
        return;
      }
      message.error(resolved.message);
    } finally {
      modalApi.setState({ confirmLoading: false });
    }
  },
  onOpenChange(isOpen) {
    if (!isOpen) {
      targetUser.value = null;
      confirmEmail.value = '';
      return;
    }
    const payload = modalApi.getData();
    targetUser.value = payload?.user ?? null;
    confirmEmail.value = '';
    modalApi.setState({ confirmDisabled: true });
  },
  title: '显式永久删除已软删除用户',
});

watch(emailMatched, (matched) => {
  modalApi.setState({ confirmDisabled: !matched });
});
</script>

<template>
  <Modal class="w-full max-w-[520px]">
    <div class="space-y-4">
      <Alert
        description="永久删除将从 IAM 物理抹除该账号的身份记录、密码凭证与全部历史授权会话，此操作不可恢复，原占用的登录邮箱将立即释放供重新注册或创建。"
        message="不可逆高危操作警告"
        show-icon
        type="error"
      />

      <Descriptions v-if="targetUser" bordered :column="1" size="small">
        <Descriptions.Item label="待永久删除邮箱">
          <span class="font-semibold">{{ targetUser.email || '—' }}</span>
        </Descriptions.Item>
        <Descriptions.Item label="用户 ID">
          <span class="font-mono text-xs">{{ targetUser.userId || '—' }}</span>
        </Descriptions.Item>
      </Descriptions>

      <div class="space-y-2">
        <label class="text-foreground block text-sm font-medium">
          请输入上方完整邮箱地址以解锁永久删除：
        </label>
        <Input
          v-model:value="confirmEmail"
          allow-clear
          autocomplete="off"
          :placeholder="targetUser?.email || '输入完整邮箱确认'"
        />
      </div>
    </div>
  </Modal>
</template>
