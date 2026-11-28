<script lang="ts" setup>
import { ref } from 'vue';

import { useVbenModal } from '@vben/common-ui';

import { Alert, Button, Descriptions, message, Space } from 'ant-design-vue';

export interface SecretRevealModalPayload {
  clientId: string;
  displayName?: string;
  mode: 'created_client' | 'created_secret';
  secretName?: string;
  secretPrefix?: string;
  secretValue: string;
}

const activePayload = ref<null | SecretRevealModalPayload>(null);
const copiedSecret = ref(false);
const copiedClientId = ref(false);

async function copyField(
  text: string | undefined,
  label: string,
  kind: 'client_id' | 'secret',
) {
  if (!text) {
    return;
  }
  try {
    await navigator.clipboard.writeText(text);
    if (kind === 'secret') {
      copiedSecret.value = true;
    } else {
      copiedClientId.value = true;
    }
    message.success(`已复制${label}`);
  } catch {
    message.error(`复制${label}失败，请手动选中复制`);
  }
}

const [Modal, modalApi] = useVbenModal<SecretRevealModalPayload>({
  closeOnClickModal: false,
  closeOnPressEscape: false,
  confirmText: '我已妥善保存，关闭窗口',
  onConfirm() {
    modalApi.close();
  },
  onOpenChange(isOpen) {
    if (!isOpen) {
      activePayload.value = null;
      copiedSecret.value = false;
      copiedClientId.value = false;
      return;
    }
    const data = modalApi.getData();
    activePayload.value = data ?? null;
    copiedSecret.value = false;
    copiedClientId.value = false;
    modalApi.setState({
      title:
        data?.mode === 'created_secret'
          ? '新客户端密钥已生成（仅展示一次）'
          : '机密客户端创建成功（请立即保存初始密钥）',
    });
  },
  showCancelButton: false,
});
</script>

<template>
  <Modal class="w-full max-w-[580px]">
    <div v-if="activePayload" class="space-y-4">
      <Alert
        description="平台仅存储单向哈希，关闭窗口后将无法再次查看完整密钥，请立即妥善保存。"
        message="请立即复制并保存完整客户端密钥"
        show-icon
        type="warning"
      />

      <Descriptions bordered :column="1" size="small">
        <Descriptions.Item
          v-if="activePayload.displayName"
          label="客户端显示名称"
        >
          <span class="font-medium">{{ activePayload.displayName }}</span>
        </Descriptions.Item>
        <Descriptions.Item v-if="activePayload.secretName" label="密钥备注名称">
          <span>{{ activePayload.secretName }}</span>
        </Descriptions.Item>
        <Descriptions.Item label="Client ID">
          <div class="flex flex-wrap items-center justify-between gap-2">
            <code class="break-all font-mono text-xs font-semibold">
              {{ activePayload.clientId }}
            </code>
            <Button
              size="small"
              type="link"
              @click="
                copyField(activePayload.clientId, 'Client ID', 'client_id')
              "
            >
              {{ copiedClientId ? '已复制' : '复制 Client ID' }}
            </Button>
          </div>
        </Descriptions.Item>
      </Descriptions>

      <div class="space-y-2">
        <div class="flex items-center justify-between gap-2">
          <span class="text-foreground text-sm font-semibold">
            客户端明文密钥（Client Secret）
          </span>
          <Space :size="8">
            <Button
              size="small"
              type="primary"
              @click="
                copyField(activePayload.secretValue, '客户端密钥', 'secret')
              "
            >
              {{ copiedSecret ? '已复制密钥' : '一键复制密钥' }}
            </Button>
          </Space>
        </div>

        <div
          class="bg-muted/60 border-border select-all break-all rounded-md border p-3 font-mono text-xs leading-5"
        >
          {{ activePayload.secretValue }}
        </div>
      </div>
    </div>
  </Modal>
</template>
