<script lang="ts" setup>
import type { CreateOAuthClientSecretResponse } from '@plateau/api/plateau/admin/client/v1';
import type { OAuthClient } from '@plateau/api/plateau/iam/client/v1';
import type { Dayjs } from 'dayjs';

import { ref } from 'vue';
import { useRouter } from 'vue-router';

import { useVbenModal } from '@vben/common-ui';
import { LOGIN_PATH } from '@vben/constants';

import {
  Alert,
  DatePicker,
  Descriptions,
  Input,
  message,
  RadioGroup,
} from 'ant-design-vue';
import dayjs from 'dayjs';

import { createOAuthClientSecret, resolveAdminApiError } from '#/api';
import { useAuthStore } from '#/store';

import { resolveOAuthClientResourceName } from '../schemas';

export interface CreateSecretModalPayload {
  client: OAuthClient;
}

const emit = defineEmits<{
  created: [
    payload: {
      client: OAuthClient;
      response: CreateOAuthClientSecretResponse;
    },
  ];
}>();

type ExpiryMode = '30d' | '90d' | 'custom' | 'never';

const router = useRouter();
const authStore = useAuthStore();

const targetClient = ref<null | OAuthClient>(null);
const secretName = ref('');
const expiryMode = ref<ExpiryMode>('never');
const customExpiresAt = ref<Dayjs>();
const fieldError = ref('');

function computeExpiresTimeIso(): { error?: string; iso?: string } {
  switch (expiryMode.value) {
    case '30d': {
      return { iso: dayjs().add(30, 'day').toDate().toISOString() };
    }
    case '90d': {
      return { iso: dayjs().add(90, 'day').toDate().toISOString() };
    }
    case 'custom': {
      if (!customExpiresAt.value || !customExpiresAt.value.isValid()) {
        return { error: '请选择有效的自定义密钥过期时间' };
      }
      if (customExpiresAt.value.valueOf() <= Date.now() + 60_000) {
        return { error: '密钥过期时间必须晚于当前时间至少 1 分钟' };
      }
      return { iso: customExpiresAt.value.toDate().toISOString() };
    }
    default: {
      return {};
    }
  }
}

const [Modal, modalApi] = useVbenModal<CreateSecretModalPayload>({
  confirmText: '生成新密钥',
  onCancel() {
    modalApi.close();
  },
  async onConfirm() {
    if (!targetClient.value) {
      return;
    }
    const { error: expiryError, iso } = computeExpiresTimeIso();
    if (expiryError) {
      fieldError.value = expiryError;
      return;
    }
    fieldError.value = '';

    const clientSnapshot = targetClient.value;
    const name = resolveOAuthClientResourceName(clientSnapshot);
    const trimmedSecretName = secretName.value.trim();

    modalApi.setState({ confirmLoading: true });
    try {
      const response = await createOAuthClientSecret({
        ...(iso ? { expiresTime: iso } : {}),
        name,
        ...(trimmedSecretName ? { secretName: trimmedSecretName } : {}),
      });
      modalApi.close();
      emit('created', { client: clientSnapshot, response });
    } catch (error) {
      const resolved = resolveAdminApiError(error, 'create');
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
      targetClient.value = null;
      secretName.value = '';
      expiryMode.value = 'never';
      customExpiresAt.value = undefined;
      fieldError.value = '';
      return;
    }
    const payload = modalApi.getData();
    targetClient.value = payload?.client ?? null;
    secretName.value = '';
    expiryMode.value = 'never';
    customExpiresAt.value = undefined;
    fieldError.value = '';
  },
  title: '新增客户端密钥（零停机轮换）',
});
</script>

<template>
  <Modal class="w-full max-w-[520px]">
    <div class="space-y-4">
      <Alert
        description="新增密钥后新旧密钥可同时有效换取令牌；完成下游服务配置切换后，再删除旧密钥即可实现零停机平滑轮换（每个动态客户端最多同时持有 5 把密钥）。"
        message="零停机轮换指引"
        show-icon
        type="info"
      />

      <Descriptions v-if="targetClient" bordered :column="1" size="small">
        <Descriptions.Item label="目标客户端">
          <span class="font-medium">{{
            targetClient.displayName || targetClient.clientId || '—'
          }}</span>
        </Descriptions.Item>
        <Descriptions.Item label="Client ID">
          <span class="font-mono text-xs">{{
            targetClient.clientId || '—'
          }}</span>
        </Descriptions.Item>
      </Descriptions>

      <div class="space-y-1.5">
        <label class="text-foreground block text-sm font-medium">
          密钥备注名称（选填）
        </label>
        <Input
          v-model:value="secretName"
          allow-clear
          :maxlength="64"
          placeholder="例如：2026Q4 生产环境轮换密钥（留空使用默认名称）"
        />
      </div>

      <div class="space-y-2">
        <label class="text-foreground block text-sm font-medium">
          有效期策略
        </label>
        <RadioGroup
          v-model:value="expiryMode"
          :options="[
            { label: '永不过期', value: 'never' },
            { label: '30 天后过期', value: '30d' },
            { label: '90 天后过期', value: '90d' },
            { label: '自定义过期时间', value: 'custom' },
          ]"
          @change="fieldError = ''"
        />

        <div v-if="expiryMode === 'custom'" class="pt-1">
          <DatePicker
            v-model:value="customExpiresAt"
            class="w-full"
            placeholder="选择密钥过期日期与时间"
            show-time
            @change="fieldError = ''"
          />
        </div>

        <p v-if="fieldError" class="text-xs text-red-500">
          {{ fieldError }}
        </p>
      </div>
    </div>
  </Modal>
</template>
