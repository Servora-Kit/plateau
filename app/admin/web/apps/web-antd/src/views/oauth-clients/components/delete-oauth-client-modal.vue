<script lang="ts" setup>
import type { OAuthClient } from '@plateau/api/plateau/iam/client/v1';

import { computed, ref, watch } from 'vue';
import { useRouter } from 'vue-router';

import { useVbenModal } from '@vben/common-ui';
import { LOGIN_PATH } from '@vben/constants';

import { Alert, Descriptions, Input, message } from 'ant-design-vue';

import { deleteOAuthClient, resolveAdminApiError } from '#/api';
import { useAuthStore } from '#/store';

import {
  isDeleteClientIdConfirmed,
  resolveOAuthClientResourceName,
} from '../schemas';

export interface DeleteOAuthClientModalPayload {
  client: OAuthClient;
}

const emit = defineEmits<{
  deleted: [client: OAuthClient];
}>();

const router = useRouter();
const authStore = useAuthStore();

const targetClient = ref<null | OAuthClient>(null);
const confirmClientId = ref('');

const clientIdMatched = computed(() =>
  isDeleteClientIdConfirmed(
    confirmClientId.value,
    targetClient.value?.clientId,
  ),
);

const [Modal, modalApi] = useVbenModal<DeleteOAuthClientModalPayload>({
  confirmText: '确认永久删除',
  onCancel() {
    modalApi.close();
  },
  async onConfirm() {
    if (!targetClient.value || !clientIdMatched.value) {
      return;
    }
    const snapshot = targetClient.value;
    const name = resolveOAuthClientResourceName(snapshot);

    modalApi.setState({ confirmLoading: true });
    try {
      await deleteOAuthClient({
        ...(snapshot.etag ? { etag: snapshot.etag } : {}),
        name,
      });
      modalApi.close();
      message.success('OAuth 客户端已永久删除，关联活跃令牌与授权记录已清理');
      emit('deleted', snapshot);
    } catch (error) {
      const resolved = resolveAdminApiError(error, 'delete');
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
      if (resolved.category === 'not_found') {
        modalApi.close();
        message.warning(resolved.message);
        emit('deleted', snapshot);
        return;
      }
      message.error(resolved.message);
    } finally {
      modalApi.setState({ confirmLoading: false });
    }
  },
  onOpenChange(isOpen) {
    if (!isOpen) {
      targetClient.value = null;
      confirmClientId.value = '';
      return;
    }
    const payload = modalApi.getData();
    targetClient.value = payload?.client ?? null;
    confirmClientId.value = '';
    modalApi.setState({ confirmDisabled: true });
  },
  title: '永久物理删除 OAuth 客户端',
});

watch(clientIdMatched, (matched) => {
  modalApi.setState({ confirmDisabled: !matched });
});
</script>

<template>
  <Modal class="w-full max-w-[540px]">
    <div class="space-y-4">
      <Alert
        description="永久删除该客户端将立即撤销其所有活跃访问令牌、刷新令牌、在途登录授权流及全部用户已授权同意记录，并物理清除客户端配置与密钥记录，此操作不可恢复。"
        message="不可逆高危操作警告"
        show-icon
        type="error"
      />

      <Descriptions v-if="targetClient" bordered :column="1" size="small">
        <Descriptions.Item label="客户端名称">
          <span class="font-semibold">{{
            targetClient.displayName || '—'
          }}</span>
        </Descriptions.Item>
        <Descriptions.Item label="待删除 Client ID">
          <code class="font-mono text-xs font-semibold">{{
            targetClient.clientId || '—'
          }}</code>
        </Descriptions.Item>
      </Descriptions>

      <div class="space-y-2">
        <label class="text-foreground block text-sm font-medium">
          请输入上方完整 Client ID 以解锁永久删除：
        </label>
        <Input
          v-model:value="confirmClientId"
          allow-clear
          autocomplete="off"
          :placeholder="targetClient?.clientId || '输入完整 Client ID 确认'"
        />
      </div>
    </div>
  </Modal>
</template>
