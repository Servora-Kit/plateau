<script lang="ts" setup>
import type { User } from '@plateau/api/iam/user/v1';

import type { CreateUserFormValues } from '../schemas';

import { useRouter } from 'vue-router';

import { useVbenModal } from '@vben/common-ui';
import { LOGIN_PATH } from '@vben/constants';

import { Alert, message } from 'ant-design-vue';

import { useVbenForm } from '#/adapter/form';
import { createAdminApi, resolveAdminApiError } from '#/api';
import { useAuthStore } from '#/store';

import { buildCreateUserRequest, getCreateUserFormSchema } from '../schemas';

const emit = defineEmits<{
  created: [payload: { emailSent: boolean; user: User }];
}>();

const router = useRouter();
const authStore = useAuthStore();

const [CreateForm, formApi] = useVbenForm({
  commonConfig: {
    componentProps: {
      class: 'w-full',
    },
  },
  layout: 'vertical',
  schema: getCreateUserFormSchema(),
  showDefaultActions: false,
});

const [Modal, modalApi] = useVbenModal({
  confirmText: '创建用户',
  onCancel() {
    modalApi.close();
  },
  async onConfirm() {
    const { valid } = await formApi.validate();
    if (!valid) {
      return;
    }
    const rawValues = (await formApi.getValues()) as CreateUserFormValues;
    const request = buildCreateUserRequest(rawValues);

    modalApi.setState({ confirmLoading: true });
    try {
      const response = await createAdminApi().users.CreateUser(request);
      const createdUser = response.user ?? {
        email: request.user?.email ?? rawValues.email.trim(),
      };
      const emailSent = response.verificationEmailSent === true;
      modalApi.close();
      emit('created', { emailSent, user: createdUser });
      if (emailSent) {
        message.success('用户已创建，验证邮件已发送');
      }
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
  async onOpenChange(isOpen) {
    if (isOpen) {
      await formApi.resetForm();
      await formApi.setValues({
        email: '',
        name: '',
        nickname: '',
        password: '',
        requirePasswordChange: true,
      });
    }
  },
  title: '创建 IAM 用户',
});
</script>

<template>
  <Modal class="w-full max-w-[520px]">
    <div class="space-y-4">
      <Alert
        description="新建账号初始处于“待邮箱验证”状态，完成邮箱验证后方可登录 IAM。若邮件服务暂时故障，账号仍会完成创建并支持后续手动补发验证信。"
        message="创建与验证说明"
        show-icon
        type="info"
      />
      <CreateForm />
    </div>
  </Modal>
</template>
