<script lang="ts" setup>
import type { User } from '@plateau/api/plateau/iam/user/v1';

import { nextTick, ref } from 'vue';
import { useRouter } from 'vue-router';

import { useVbenModal } from '@vben/common-ui';
import { LOGIN_PATH } from '@vben/constants';

import { Alert, Descriptions, message } from 'ant-design-vue';

import { useVbenForm } from '#/adapter/form';
import { createAdminApi, resolveAdminApiError } from '#/api';
import { useAuthStore } from '#/store';

import { getSetPasswordFormSchema, resolveUserResourceName } from '../schemas';

export interface SetPasswordModalPayload {
  user: User;
}

const router = useRouter();
const authStore = useAuthStore();
const targetUser = ref<null | User>(null);

const [PasswordForm, formApi] = useVbenForm({
  commonConfig: {
    componentProps: {
      class: 'w-full',
    },
  },
  layout: 'vertical',
  schema: getSetPasswordFormSchema(),
  showDefaultActions: false,
});

const [Modal, modalApi] = useVbenModal<SetPasswordModalPayload>({
  confirmText: '确认重置密码',
  onCancel() {
    modalApi.close();
  },
  async onConfirm() {
    if (!targetUser.value) {
      return;
    }
    const { valid } = await formApi.validate();
    if (!valid) {
      return;
    }
    const values = (await formApi.getValues()) as { password: string };
    const name = resolveUserResourceName(targetUser.value);

    modalApi.setState({ confirmLoading: true });
    try {
      await createAdminApi().users.SetUserPassword({
        name,
        newPassword: values.password,
      });
      modalApi.close();
      message.success('已重置该账号的 IAM 登录密码，并撤销其当前 IAM 会话');
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
  async onOpenChange(isOpen) {
    if (!isOpen) {
      targetUser.value = null;
      if (formApi.isMounted) {
        await formApi.resetForm();
      }
      return;
    }
    const payload = modalApi.getData();
    targetUser.value = payload?.user ?? null;
    await nextTick();
    await formApi.resetForm();
  },
  title: '管理设置密码',
});
</script>

<template>
  <Modal class="w-full max-w-[520px]">
    <div class="space-y-4">
      <Alert
        description="直接重置该账号的 IAM 登录密码，并立即撤销其当前全部 IAM 登录与刷新会话。此操作不发送密码通知邮件、不添加下次强制改密标记，也不会自动终止其他应用已建立的本地会话。"
        message="操作范围说明"
        show-icon
        type="warning"
      />
      <Descriptions v-if="targetUser" bordered :column="1" size="small">
        <Descriptions.Item label="目标邮箱">
          {{ targetUser.email || '—' }}
        </Descriptions.Item>
        <Descriptions.Item label="用户 ID">
          <span class="font-mono text-xs">{{ targetUser.userId || '—' }}</span>
        </Descriptions.Item>
      </Descriptions>
      <PasswordForm />
    </div>
  </Modal>
</template>
