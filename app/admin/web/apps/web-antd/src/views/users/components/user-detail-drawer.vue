<script lang="ts" setup>
import type { User } from '@plateau/api/plateau/iam/user/v1';

import type { ProfileFormValues } from '../schemas';

import { computed, nextTick, onUnmounted, ref } from 'vue';
import { useRouter } from 'vue-router';

import { useVbenDrawer } from '@vben/common-ui';
import { LOGIN_PATH } from '@vben/constants';
import { formatDateTime } from '@vben/utils';

import {
  Alert,
  Avatar,
  Button,
  Descriptions,
  Divider,
  message,
  Skeleton,
  Space,
  Tag,
} from 'ant-design-vue';

import { useVbenForm } from '#/adapter/form';
import { createAdminApi, resolveAdminApiError } from '#/api';
import { resolveUserDisplayName, useAuthStore } from '#/store';

import {
  buildProfileUpdateRequest,
  extractProfileFormValues,
  getProfileFormSchema,
  resolveUserResourceName,
} from '../schemas';

export interface UserDetailDrawerPayload {
  resourceName: string;
}

const emit = defineEmits<{
  updated: [user: User];
}>();

const router = useRouter();
const authStore = useAuthStore();

const loading = ref(false);
const saving = ref(false);
const loadError = ref('');
const detailUser = ref<null | User>(null);
const initialProfile = ref<ProfileFormValues>(extractProfileFormValues());

let activeAbortController: AbortController | null = null;

function cancelActiveRequest() {
  activeAbortController?.abort();
  activeAbortController = null;
}

onUnmounted(() => {
  cancelActiveRequest();
});

const isDeleted = computed(() => Boolean(detailUser.value?.deleteTime));
const canEditProfile = computed(
  () =>
    Boolean(detailUser.value) &&
    !isDeleted.value &&
    authStore.hasPermission('manage_users'),
);
const canReadAudit = computed(() => authStore.hasPermission('read_audit'));

const displayName = computed(() => resolveUserDisplayName(detailUser.value));
const avatarUrl = computed(
  () => detailUser.value?.profile?.picture?.trim() || '',
);

const statusDisplay = computed(() => {
  if (isDeleted.value) {
    return { color: 'volcano', text: '已软删除（无限期保留）' };
  }
  switch (detailUser.value?.status) {
    case 'USER_STATUS_ACTIVE': {
      return { color: 'success', text: '正常（ACTIVE）' };
    }
    case 'USER_STATUS_DISABLED': {
      return { color: 'error', text: '已封禁（DISABLED）' };
    }
    case 'USER_STATUS_PENDING_EMAIL_VERIFICATION': {
      return { color: 'warning', text: '待邮箱验证' };
    }
    default: {
      return { color: 'default', text: '未知状态' };
    }
  }
});

const requiresPasswordChange = computed(() =>
  Boolean(
    detailUser.value?.requiredActions?.includes(
      'USER_REQUIRED_ACTION_UPDATE_PASSWORD',
    ),
  ),
);

const [ProfileForm, profileFormApi] = useVbenForm({
  commonConfig: {
    componentProps: {
      class: 'w-full',
    },
  },
  layout: 'vertical',
  schema: getProfileFormSchema(),
  showDefaultActions: false,
});

async function handleUnauthenticatedRedirect(msg: string) {
  message.warning(msg);
  authStore.markUnauthenticated();
  await router.replace({
    path: LOGIN_PATH,
    query: { redirect: router.currentRoute.value.fullPath },
  });
}

async function fetchUserDetail(resourceName: string) {
  cancelActiveRequest();
  const controller = new AbortController();
  activeAbortController = controller;
  loading.value = true;
  loadError.value = '';
  try {
    const user = await createAdminApi(controller.signal).users.GetUser({
      name: resourceName,
    });
    if (activeAbortController !== controller) {
      return;
    }
    detailUser.value = user;
    const nextProfile = extractProfileFormValues(user.profile);
    initialProfile.value = nextProfile;
    loading.value = false;
    if (canEditProfile.value) {
      await nextTick();
      await profileFormApi.setValues(nextProfile);
    }
  } catch (error) {
    const resolved = resolveAdminApiError(error);
    if (resolved.silent) {
      return;
    }
    if (!resolved.preserveSession) {
      await handleUnauthenticatedRedirect(resolved.message);
      return;
    }
    loadError.value = resolved.message;
  } finally {
    if (activeAbortController === controller) {
      loading.value = false;
    }
  }
}

const [Drawer, drawerApi] = useVbenDrawer<UserDetailDrawerPayload>({
  confirmText: '保存资料修改',
  onCancel() {
    drawerApi.close();
  },
  async onConfirm() {
    if (!canEditProfile.value || !detailUser.value) {
      drawerApi.close();
      return;
    }
    const { valid } = await profileFormApi.validate();
    if (!valid) {
      return;
    }
    const values = (await profileFormApi.getValues()) as ProfileFormValues;
    const resourceName = resolveUserResourceName(detailUser.value);
    const etag = detailUser.value.etag ?? '';
    const request = buildProfileUpdateRequest(
      resourceName,
      etag,
      initialProfile.value,
      values,
    );
    if (!request) {
      message.info('资料字段未发生变更');
      return;
    }

    saving.value = true;
    drawerApi.setState({ confirmLoading: true });
    try {
      const updated = await createAdminApi().users.UpdateUser(request);
      detailUser.value = updated;
      const nextProfile = extractProfileFormValues(updated.profile);
      initialProfile.value = nextProfile;
      await profileFormApi.setValues(nextProfile);
      if (updated.userId && updated.userId === authStore.currentUser?.userId) {
        authStore.syncUserSnapshot(updated);
      }
      message.success('用户资料已保存');
      emit('updated', updated);
    } catch (error) {
      const resolved = resolveAdminApiError(error, 'update');
      if (resolved.silent) {
        return;
      }
      if (!resolved.preserveSession) {
        drawerApi.close();
        await handleUnauthenticatedRedirect(resolved.message);
        return;
      }
      message.error(resolved.message);
      if (resolved.shouldRefreshRecord) {
        await fetchUserDetail(resourceName);
      }
    } finally {
      saving.value = false;
      drawerApi.setState({ confirmLoading: false });
    }
  },
  async onOpenChange(isOpen) {
    if (!isOpen) {
      cancelActiveRequest();
      detailUser.value = null;
      loadError.value = '';
      return;
    }
    const payload = drawerApi.getData();
    if (!payload?.resourceName) {
      return;
    }
    await fetchUserDetail(payload.resourceName);
    drawerApi.setState({
      showConfirmButton: canEditProfile.value,
      title: isDeleted.value ? '已删除用户详情（只读）' : '用户详情与资料编辑',
    });
  },
});

async function jumpToResourceAudit() {
  if (!detailUser.value) {
    return;
  }
  const resourceName = resolveUserResourceName(detailUser.value);
  drawerApi.close();
  await router.push({
    path: '/audit',
    query: { resourceName },
  });
}

async function jumpToActorAudit() {
  const userId = detailUser.value?.userId?.trim();
  if (!userId) {
    return;
  }
  drawerApi.close();
  await router.push({
    path: '/audit',
    query: { actorId: userId, actorType: 'human' },
  });
}
</script>

<template>
  <Drawer class="w-full max-w-[640px]">
    <Skeleton v-if="loading" active :paragraph="{ rows: 8 }" />

    <Alert v-else-if="loadError" :message="loadError" show-icon type="error" />

    <div v-else-if="detailUser" class="space-y-5">
      <Alert
        v-if="isDeleted"
        description="该账号处于软删除状态，无限期保留且继续占用原登录邮箱；如需编辑资料，请先恢复账号。"
        message="回收站只读快照"
        show-icon
        type="warning"
      />

      <div class="flex flex-wrap items-center justify-between gap-3">
        <div class="flex items-center gap-3">
          <Avatar :size="48" :src="avatarUrl || undefined">
            {{ displayName.slice(0, 1).toUpperCase() }}
          </Avatar>
          <div>
            <div class="text-foreground text-base font-semibold">
              {{ displayName }}
            </div>
            <div class="text-muted-foreground text-xs">
              {{ detailUser.email || '—' }}
            </div>
          </div>
        </div>
        <Space v-if="canReadAudit" size="small" wrap>
          <Button size="small" @click="jumpToResourceAudit">
            查看账号变更审计
          </Button>
          <Button
            v-if="detailUser.userId"
            size="small"
            @click="jumpToActorAudit"
          >
            查看该账号操作审计
          </Button>
        </Space>
      </div>

      <Descriptions bordered :column="1" size="small" title="身份与状态元数据">
        <Descriptions.Item label="用户 ID">
          <span class="font-mono text-xs">{{ detailUser.userId || '—' }}</span>
        </Descriptions.Item>
        <Descriptions.Item label="资源名称">
          <span class="font-mono text-xs">{{ detailUser.name || '—' }}</span>
        </Descriptions.Item>
        <Descriptions.Item label="登录邮箱（只读）">
          <span>{{ detailUser.email || '—' }}</span>
        </Descriptions.Item>
        <Descriptions.Item label="生命周期状态">
          <Space wrap>
            <Tag :color="statusDisplay.color">{{ statusDisplay.text }}</Tag>
            <Tag v-if="isDeleted" color="default">原邮箱占用中</Tag>
          </Space>
        </Descriptions.Item>
        <Descriptions.Item label="邮箱验证状态">
          <Space wrap>
            <Tag :color="detailUser.emailVerified ? 'processing' : 'default'">
              {{ detailUser.emailVerified ? '已验证' : '未验证' }}
            </Tag>
            <span
              v-if="detailUser.emailVerifiedTime"
              class="text-muted-foreground text-xs"
            >
              验证时间：{{ formatDateTime(detailUser.emailVerifiedTime) }}
            </span>
          </Space>
        </Descriptions.Item>
        <Descriptions.Item label="首次登录待办">
          <Tag v-if="requiresPasswordChange" color="gold">
            待首次登录修改密码（UPDATE_PASSWORD）
          </Tag>
          <span v-else class="text-muted-foreground text-xs">无待办要求</span>
        </Descriptions.Item>
        <Descriptions.Item label="创建时间">
          {{
            detailUser.createTime ? formatDateTime(detailUser.createTime) : '—'
          }}
        </Descriptions.Item>
        <Descriptions.Item label="最后更新时间">
          {{
            detailUser.updateTime ? formatDateTime(detailUser.updateTime) : '—'
          }}
        </Descriptions.Item>
        <Descriptions.Item v-if="detailUser.deleteTime" label="软删除时间">
          {{ formatDateTime(detailUser.deleteTime) }}
        </Descriptions.Item>
      </Descriptions>

      <template v-if="canEditProfile">
        <Divider class="!my-3" />
        <div class="space-y-3">
          <div>
            <h4 class="text-foreground text-sm font-semibold">
              编辑标准资料（共 7 项，保存时仅提交变更字段掩码与最新 ETag）
            </h4>
            <p class="text-muted-foreground text-xs">
              登录邮箱属于不可变登录标识；头像仅支持填写公开 HTTP(S) 图片 URL。
            </p>
          </div>
          <ProfileForm />
        </div>
      </template>

      <template v-else>
        <Descriptions
          bordered
          :column="1"
          size="small"
          title="标准资料（只读）"
        >
          <Descriptions.Item label="显示姓名">
            {{ detailUser.profile?.name || '—' }}
          </Descriptions.Item>
          <Descriptions.Item label="昵称">
            {{ detailUser.profile?.nickname || '—' }}
          </Descriptions.Item>
          <Descriptions.Item label="名（Given Name）">
            {{ detailUser.profile?.givenName || '—' }}
          </Descriptions.Item>
          <Descriptions.Item label="姓（Family Name）">
            {{ detailUser.profile?.familyName || '—' }}
          </Descriptions.Item>
          <Descriptions.Item label="偏好用户名">
            {{ detailUser.profile?.preferredUsername || '—' }}
          </Descriptions.Item>
          <Descriptions.Item label="头像 URL">
            <span class="break-all font-mono text-xs">
              {{ detailUser.profile?.picture || '—' }}
            </span>
          </Descriptions.Item>
          <Descriptions.Item label="语言区域（Locale）">
            {{ detailUser.profile?.locale || '—' }}
          </Descriptions.Item>
        </Descriptions>
      </template>
    </div>
  </Drawer>
</template>
