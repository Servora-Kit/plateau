<script lang="ts" setup>
import type { CreateOAuthClientSecretResponse } from '@plateau/api/plateau/admin/client/v1';
import type {
  OAuthClient,
  OAuthClientSecret,
  TokenEndpointAuthMethod,
} from '@plateau/api/plateau/iam/client/v1';

import type {
  EditOAuthClientFormValues,
  OAuthClientFormErrors,
} from '../schemas';

import { computed, onUnmounted, ref } from 'vue';
import { useRouter } from 'vue-router';

import { useVbenDrawer, useVbenModal } from '@vben/common-ui';
import { LOGIN_PATH } from '@vben/constants';
import { formatDateTime } from '@vben/utils';

import {
  Alert,
  Avatar,
  Button,
  Card,
  Checkbox,
  Descriptions,
  Divider,
  Input,
  message,
  Popconfirm,
  Select,
  Skeleton,
  Space,
  Switch,
  Tag,
  Tooltip,
} from 'ant-design-vue';

import {
  deleteOAuthClientSecret,
  disableOAuthClient,
  enableOAuthClient,
  getOAuthClient,
  resolveAdminApiError,
  updateOAuthClient,
} from '#/api';
import { useAuthStore } from '#/store';

import {
  buildUpdateOAuthClientRequest,
  countActiveSecrets,
  extractEditOAuthClientFormValues,
  formatSecretPrefixMask,
  getAuthMethodOptions,
  getClientArchitectureBadge,
  getGrantTypeOptions,
  getSourceBadge,
  getStatusBadge,
  getTrustedPolicyBadge,
  isConfidentialOAuthClient,
  isSecretExpired,
  isStaticOAuthClient,
  MAX_OAUTH_CLIENT_SECRETS,
  normalizeAuthMethodChange,
  normalizeSafeHttpImageUrl,
  resolveOAuthClientDisplayName,
  resolveOAuthClientResourceName,
  resolveSecretDeletionState,
  STANDARD_OIDC_SCOPES,
  toggleGrantTypeSelection,
  validateProtocolFormFields,
} from '../schemas';
import CreateSecretModalComp from './create-secret-modal.vue';

export interface OAuthClientDetailDrawerPayload {
  resourceName: string;
}

const emit = defineEmits<{
  requestDelete: [client: OAuthClient];
  secretCreated: [
    payload: {
      client: OAuthClient;
      response: CreateOAuthClientSecretResponse;
    },
  ];
  updated: [client: OAuthClient];
}>();

const Textarea = Input.TextArea;

const router = useRouter();
const authStore = useAuthStore();

const loading = ref(false);
const saving = ref(false);
const statusChanging = ref(false);
const deletingSecretId = ref('');
const loadError = ref('');

const detailClient = ref<null | OAuthClient>(null);
const initialForm = ref<EditOAuthClientFormValues>(
  extractEditOAuthClientFormValues(),
);
const editForm = ref<EditOAuthClientFormValues>(
  extractEditOAuthClientFormValues(),
);
const errors = ref<OAuthClientFormErrors>({});

let activeAbortController: AbortController | null = null;

function cancelActiveRequest() {
  activeAbortController?.abort();
  activeAbortController = null;
}

onUnmounted(() => {
  cancelActiveRequest();
});

const canManageClients = computed(() =>
  authStore.hasPermission('manage_clients'),
);
const canReadAudit = computed(() => authStore.hasPermission('read_audit'));

const isStatic = computed(() => isStaticOAuthClient(detailClient.value));
const canEditClient = computed(
  () =>
    Boolean(detailClient.value) && !isStatic.value && canManageClients.value,
);
const isConfidential = computed(() =>
  isConfidentialOAuthClient(detailClient.value),
);

const displayName = computed(() =>
  resolveOAuthClientDisplayName(detailClient.value),
);
const logoUrl = computed(() =>
  normalizeSafeHttpImageUrl(detailClient.value?.logoUri),
);
const willWipeSecretsOnSave = computed(
  () =>
    isConfidential.value &&
    editForm.value.tokenEndpointAuthMethod ===
      'TOKEN_ENDPOINT_AUTH_METHOD_NONE',
);
const sourceBadge = computed(() => getSourceBadge(detailClient.value?.source));
const statusBadge = computed(() => getStatusBadge(detailClient.value?.status));
const architectureBadge = computed(() =>
  getClientArchitectureBadge(detailClient.value ?? {}),
);
const trustedBadge = computed(() =>
  getTrustedPolicyBadge(detailClient.value ?? {}),
);

const secretsList = computed(() => detailClient.value?.secrets ?? []);
const activeSecretsCount = computed(() =>
  countActiveSecrets(secretsList.value),
);

const addSecretState = computed(() => {
  if (!canManageClients.value) {
    return {
      disabled: true,
      reason: '当前账号缺少 manage_clients 权限',
    };
  }
  if (isStatic.value) {
    return {
      disabled: true,
      reason:
        '该客户端由 IAM 静态配置文件（oidc.yaml）托管，不支持在线新增密钥',
    };
  }
  if (secretsList.value.length >= MAX_OAUTH_CLIENT_SECRETS) {
    return {
      disabled: true,
      reason: `单个动态客户端最多同时持有 ${MAX_OAUTH_CLIENT_SECRETS} 把密钥，请先删除旧密钥后再新增`,
    };
  }
  return {
    disabled: false,
    reason: undefined,
  };
});

const hasAuthCode = computed(() =>
  editForm.value.allowedGrantTypes.includes('authorization_code'),
);
const hasClientCredentials = computed(() =>
  editForm.value.allowedGrantTypes.includes('client_credentials'),
);

const authMethodOptions = computed(() => {
  const base = getAuthMethodOptions(editForm.value.applicationType);
  const persistedPublic =
    detailClient.value?.tokenEndpointAuthMethod ===
      'TOKEN_ENDPOINT_AUTH_METHOD_NONE' && secretsList.value.length === 0;
  if (!persistedPublic) {
    return base;
  }
  return base.map((item) => ({
    ...item,
    disabled: item.disabled || item.value !== 'TOKEN_ENDPOINT_AUTH_METHOD_NONE',
  }));
});

const grantTypeOptions = computed(() =>
  getGrantTypeOptions(
    editForm.value.applicationType,
    editForm.value.tokenEndpointAuthMethod,
  ),
);

const [CreateSecretModal, createSecretModalApi] = useVbenModal({
  connectedComponent: CreateSecretModalComp,
});

async function handleUnauthenticatedRedirect(msg: string) {
  message.warning(msg);
  authStore.markUnauthenticated();
  await router.replace({
    path: LOGIN_PATH,
    query: { redirect: router.currentRoute.value.fullPath },
  });
}

async function copyText(text: string | undefined, label: string) {
  if (!text) {
    return;
  }
  try {
    await navigator.clipboard.writeText(text);
    message.success(`已复制${label}`);
  } catch {
    message.error(`复制${label}失败，请手动选中复制`);
  }
}

function syncFormFromClient(client: OAuthClient) {
  const extracted = extractEditOAuthClientFormValues(client);
  initialForm.value = extracted;
  editForm.value = {
    ...extracted,
    allowedGrantTypes: [...extracted.allowedGrantTypes],
    allowedScopes: [...extracted.allowedScopes],
  };
  errors.value = {};
}

async function fetchClientDetail(resourceName: string) {
  cancelActiveRequest();
  const controller = new AbortController();
  activeAbortController = controller;
  loading.value = true;
  loadError.value = '';
  try {
    const client = await getOAuthClient(
      { name: resourceName },
      controller.signal,
    );
    if (activeAbortController !== controller) {
      return;
    }
    detailClient.value = client;
    syncFormFromClient(client);
    drawerApi.setState({
      showConfirmButton: !isStaticOAuthClient(client) && canManageClients.value,
      title: isStaticOAuthClient(client)
        ? 'OAuth 客户端详情（静态只读）'
        : 'OAuth 客户端详情与配置管理',
    });
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

function handleAuthMethodChange(nextValue: unknown) {
  const method = nextValue as TokenEndpointAuthMethod;
  editForm.value = normalizeAuthMethodChange(editForm.value, method);
  errors.value = {
    ...errors.value,
    allowedGrantTypes: undefined,
    tokenEndpointAuthMethod: undefined,
  };
}

function handleToggleGrantType(grantType: string, checked: boolean) {
  const nextGrants = toggleGrantTypeSelection(
    editForm.value.allowedGrantTypes,
    grantType,
    checked,
  );
  const nextScopes =
    nextGrants.includes('authorization_code') &&
    !editForm.value.allowedScopes.includes('openid')
      ? ['openid', ...editForm.value.allowedScopes]
      : [...editForm.value.allowedScopes];
  editForm.value = {
    ...editForm.value,
    allowedGrantTypes: nextGrants,
    allowedScopes: nextScopes,
  };
  errors.value = {
    ...errors.value,
    allowedGrantTypes: undefined,
  };
}

function handleToggleScope(scopeValue: string, checked: boolean) {
  if (scopeValue === 'openid' && hasAuthCode.value) {
    return;
  }
  const set = new Set(editForm.value.allowedScopes);
  if (checked) {
    set.add(scopeValue);
  } else {
    set.delete(scopeValue);
  }
  if (hasAuthCode.value) {
    set.add('openid');
  }
  editForm.value = {
    ...editForm.value,
    allowedScopes: [...set],
  };
  errors.value = {
    ...errors.value,
    allowedScopes: undefined,
  };
}

function clearFieldError(key: keyof OAuthClientFormErrors) {
  if (errors.value[key]) {
    errors.value = {
      ...errors.value,
      [key]: undefined,
    };
  }
}

function openCreateSecretModal() {
  if (!detailClient.value || addSecretState.value.disabled) {
    return;
  }
  createSecretModalApi.setData({ client: detailClient.value }).open();
}

async function handleSecretCreated(payload: {
  client: OAuthClient;
  response: CreateOAuthClientSecretResponse;
}) {
  const resourceName = resolveOAuthClientResourceName(payload.client);
  await fetchClientDetail(resourceName);
  if (detailClient.value) {
    emit('updated', detailClient.value);
  }
  emit('secretCreated', payload);
}

async function handleDeleteSecret(secret: OAuthClientSecret) {
  if (!detailClient.value || !secret.secretId) {
    return;
  }
  const resourceName = resolveOAuthClientResourceName(detailClient.value);
  deletingSecretId.value = secret.secretId;
  try {
    await deleteOAuthClientSecret({
      name: resourceName,
      secretId: secret.secretId,
    });
    message.success('客户端密钥已删除');
    await fetchClientDetail(resourceName);
    if (detailClient.value) {
      emit('updated', detailClient.value);
    }
  } catch (error) {
    const resolved = resolveAdminApiError(error, 'delete');
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
      await fetchClientDetail(resourceName);
    }
  } finally {
    deletingSecretId.value = '';
  }
}

async function handleToggleClientStatus() {
  if (!detailClient.value || isStatic.value) {
    return;
  }
  const resourceName = resolveOAuthClientResourceName(detailClient.value);
  const etag = detailClient.value.etag ?? '';
  const isDisabling = detailClient.value.status === 'CLIENT_STATUS_ACTIVE';

  statusChanging.value = true;
  try {
    const updated = isDisabling
      ? await disableOAuthClient({ etag, name: resourceName })
      : await enableOAuthClient({ etag, name: resourceName });
    detailClient.value = updated;
    syncFormFromClient(updated);
    message.success(
      isDisabling
        ? '客户端已禁用，活跃会话与在途授权已撤销'
        : '客户端已重新启用',
    );
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
      await fetchClientDetail(resourceName);
    }
  } finally {
    statusChanging.value = false;
  }
}

function handleRequestDelete() {
  if (!detailClient.value || isStatic.value) {
    return;
  }
  const snapshot = detailClient.value;
  drawerApi.close();
  emit('requestDelete', snapshot);
}

const [Drawer, drawerApi] = useVbenDrawer<OAuthClientDetailDrawerPayload>({
  confirmText: '保存配置修改',
  onCancel() {
    drawerApi.close();
  },
  async onConfirm() {
    if (!canEditClient.value || !detailClient.value) {
      drawerApi.close();
      return;
    }
    const nextErrors = validateProtocolFormFields({
      ...editForm.value,
      isCreate: false,
    });
    errors.value = nextErrors;
    if (Object.keys(nextErrors).length > 0) {
      message.warning('请先修正表单中标红的配置项');
      return;
    }

    const resourceName = resolveOAuthClientResourceName(detailClient.value);
    const etag = detailClient.value.etag ?? '';
    const request = buildUpdateOAuthClientRequest(
      resourceName,
      etag,
      initialForm.value,
      editForm.value,
    );
    if (!request) {
      message.info('配置字段未发生变更');
      return;
    }

    saving.value = true;
    drawerApi.setState({ confirmLoading: true });
    try {
      const updated = await updateOAuthClient(request);
      detailClient.value = updated;
      syncFormFromClient(updated);
      message.success('客户端配置已保存');
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
        await fetchClientDetail(resourceName);
      }
    } finally {
      saving.value = false;
      drawerApi.setState({ confirmLoading: false });
    }
  },
  async onOpenChange(isOpen) {
    if (!isOpen) {
      cancelActiveRequest();
      detailClient.value = null;
      loadError.value = '';
      errors.value = {};
      return;
    }
    const payload = drawerApi.getData();
    if (!payload?.resourceName) {
      return;
    }
    await fetchClientDetail(payload.resourceName);
  },
  title: 'OAuth 客户端详情',
});

async function jumpToResourceAudit() {
  if (!detailClient.value) {
    return;
  }
  const resourceName = resolveOAuthClientResourceName(detailClient.value);
  drawerApi.close();
  await router.push({
    path: '/audit',
    query: { resourceName },
  });
}

async function jumpToServiceActorAudit() {
  const clientId = detailClient.value?.clientId?.trim();
  if (!clientId) {
    return;
  }
  drawerApi.close();
  await router.push({
    path: '/audit',
    query: { actorId: clientId, actorType: 'service' },
  });
}
</script>

<template>
  <Drawer class="w-full max-w-[700px]">
    <template #extra>
      <Button
        v-if="detailClient"
        :disabled="loading || saving"
        :loading="loading"
        size="small"
        @click="fetchClientDetail(resolveOAuthClientResourceName(detailClient))"
      >
        刷新详情
      </Button>
    </template>

    <Skeleton v-if="loading && !detailClient" active :paragraph="{ rows: 9 }" />

    <Alert v-else-if="loadError" :message="loadError" show-icon type="error" />

    <div v-else-if="detailClient" class="space-y-5">
      <Alert
        v-if="isStatic"
        description="该客户端由 IAM 静态配置文件（oidc.yaml）引导托管，仅支持只读查看。"
        message="系统静态托管客户端"
        show-icon
        type="info"
      />

      <div class="flex flex-wrap items-center justify-between gap-3">
        <div class="flex items-center gap-3">
          <Avatar shape="square" :size="48" :src="logoUrl || undefined">
            {{ displayName.slice(0, 1).toUpperCase() }}
          </Avatar>
          <div class="space-y-1">
            <div class="flex flex-wrap items-center gap-1.5">
              <span class="text-foreground text-base font-semibold">
                {{ displayName }}
              </span>
              <Tag :color="sourceBadge.color">{{ sourceBadge.text }}</Tag>
              <Tag :color="statusBadge.color">{{ statusBadge.text }}</Tag>
            </div>
            <div class="flex flex-wrap items-center gap-2 text-xs">
              <code class="text-muted-foreground font-mono">
                {{ detailClient.clientId || '—' }}
              </code>
              <Button
                v-if="detailClient.clientId"
                size="small"
                type="link"
                @click="copyText(detailClient.clientId, 'Client ID')"
              >
                复制 ID
              </Button>
            </div>
          </div>
        </div>

        <Space v-if="canReadAudit" size="small" wrap>
          <Button size="small" @click="jumpToResourceAudit">
            查看变更审计
          </Button>
          <Button
            v-if="detailClient.clientId"
            size="small"
            @click="jumpToServiceActorAudit"
          >
            查看服务调用审计
          </Button>
        </Space>
      </div>

      <Descriptions bordered :column="2" size="small" title="协议身份元数据">
        <Descriptions.Item label="Client ID">
          <span class="font-mono text-xs">{{
            detailClient.clientId || '—'
          }}</span>
        </Descriptions.Item>
        <Descriptions.Item label="资源名称">
          <span class="font-mono text-xs">{{ detailClient.name || '—' }}</span>
        </Descriptions.Item>
        <Descriptions.Item label="架构与认证模式">
          <Space :size="4" wrap>
            <Tag :color="architectureBadge.color">
              {{ architectureBadge.modeLabel }}
            </Tag>
            <Tag color="default">{{ architectureBadge.authLabel }}</Tag>
          </Space>
        </Descriptions.Item>
        <Descriptions.Item label="受信策略">
          <Tag :color="trustedBadge.color">{{ trustedBadge.text }}</Tag>
        </Descriptions.Item>
        <Descriptions.Item label="创建时间">
          {{
            detailClient.createTime
              ? formatDateTime(detailClient.createTime)
              : '—'
          }}
        </Descriptions.Item>
        <Descriptions.Item label="最后更新时间">
          {{
            detailClient.updateTime
              ? formatDateTime(detailClient.updateTime)
              : '—'
          }}
        </Descriptions.Item>
      </Descriptions>

      <Card
        v-if="isConfidential"
        size="small"
        :title="`客户端密钥（已配置 ${secretsList.length} / ${MAX_OAUTH_CLIENT_SECRETS} 把 · 有效 ${activeSecretsCount} 把）`"
      >
        <template #extra>
          <Tooltip :title="addSecretState.reason">
            <Button
              :disabled="addSecretState.disabled"
              size="small"
              type="primary"
              @click="openCreateSecretModal"
            >
              新增密钥
            </Button>
          </Tooltip>
        </template>

        <div class="space-y-2.5">
          <p class="text-muted-foreground m-0 text-xs">
            平台仅持久化密钥 SHA-256
            单向哈希与前缀掩码；如需轮换，请先新增密钥并更新下游服务，验证无误后再删除旧密钥。
          </p>

          <div
            v-if="secretsList.length === 0"
            class="text-muted-foreground rounded-md border border-dashed px-3 py-3 text-center text-xs"
          >
            当前暂无客户端密钥记录
          </div>

          <div
            v-else
            class="divide-border border-border divide-y rounded-md border"
          >
            <div
              v-for="sec in secretsList"
              :key="sec.secretId || sec.secretPrefix"
              class="flex flex-wrap items-center justify-between gap-3 px-3 py-2.5 text-xs"
            >
              <div class="min-w-0 flex-1 space-y-1">
                <div class="flex flex-wrap items-center gap-2">
                  <span class="text-foreground font-medium">
                    {{ sec.name || '默认密钥' }}
                  </span>
                  <code
                    class="bg-muted rounded px-1.5 py-0.5 font-mono text-xs"
                  >
                    {{ formatSecretPrefixMask(sec.secretPrefix) }}
                  </code>
                  <Tag v-if="isSecretExpired(sec)" class="m-0" color="error">
                    已过期
                  </Tag>
                  <Tag v-else class="m-0" color="success">有效</Tag>
                </div>

                <div
                  class="text-muted-foreground flex flex-wrap items-center gap-x-4 gap-y-1"
                >
                  <span>
                    创建：{{
                      sec.createTime ? formatDateTime(sec.createTime) : '—'
                    }}
                  </span>
                  <span
                    :class="
                      isSecretExpired(sec) ? 'font-medium text-red-500' : ''
                    "
                  >
                    过期：{{
                      sec.expiresTime
                        ? formatDateTime(sec.expiresTime)
                        : '永不过期'
                    }}
                  </span>
                  <span>
                    最近使用：{{
                      sec.lastUsedTime
                        ? formatDateTime(sec.lastUsedTime)
                        : '尚未使用'
                    }}
                  </span>
                </div>
              </div>

              <div>
                <Tooltip
                  v-if="
                    !resolveSecretDeletionState(
                      detailClient,
                      sec,
                      canManageClients,
                    ).deletable
                  "
                  :title="
                    resolveSecretDeletionState(
                      detailClient,
                      sec,
                      canManageClients,
                    ).reason
                  "
                >
                  <Button danger disabled size="small" type="link">
                    删除
                  </Button>
                </Tooltip>
                <Popconfirm
                  v-else
                  cancel-text="取消"
                  ok-text="确认删除"
                  title="删除后使用该密钥的下游服务将立即无法换取新令牌（已建立的用户会话不受影响），确认删除该密钥？"
                  @confirm="handleDeleteSecret(sec)"
                >
                  <Button
                    danger
                    :loading="deletingSecretId === sec.secretId"
                    size="small"
                    type="link"
                  >
                    删除
                  </Button>
                </Popconfirm>
              </div>
            </div>
          </div>
        </div>
      </Card>

      <Divider class="!my-2" />

      <template v-if="canEditClient">
        <div class="space-y-4">
          <div>
            <h4 class="text-foreground m-0 text-sm font-semibold">
              编辑客户端配置（保存时仅提交变更字段 FieldMask 与最新 ETag）
            </h4>
            <p class="text-muted-foreground mt-0.5 mb-0 text-xs">
              Client ID
              与应用架构类型属于不可变协议属性，仅支持修改展示元数据、认证方法与白名单配置。
            </p>
          </div>

          <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <div class="space-y-1.5">
              <label class="text-foreground block text-sm font-medium">
                客户端显示名称 <span class="text-red-500">*</span>
              </label>
              <Input
                v-model:value="editForm.displayName"
                allow-clear
                :maxlength="128"
                :status="errors.displayName ? 'error' : undefined"
                @change="clearFieldError('displayName')"
              />
              <p v-if="errors.displayName" class="m-0 text-xs text-red-500">
                {{ errors.displayName }}
              </p>
            </div>

            <div class="space-y-1.5">
              <label class="text-foreground block text-sm font-medium">
                应用图标 URL
              </label>
              <Input
                v-model:value="editForm.logoUri"
                allow-clear
                :maxlength="512"
                placeholder="https://example.com/logo.png"
                :status="errors.logoUri ? 'error' : undefined"
                @change="clearFieldError('logoUri')"
              />
              <p v-if="errors.logoUri" class="m-0 text-xs text-red-500">
                {{ errors.logoUri }}
              </p>
            </div>
          </div>

          <div class="space-y-1.5">
            <label class="text-foreground block text-sm font-medium">
              备注说明
            </label>
            <Input
              v-model:value="editForm.description"
              allow-clear
              :maxlength="256"
              placeholder="选填，清空后保存将清除原说明"
            />
          </div>

          <div class="space-y-1.5">
            <label class="text-foreground block text-sm font-medium">
              令牌端点认证方式 <span class="text-red-500">*</span>
            </label>
            <Select
              class="w-full"
              :options="authMethodOptions"
              :status="errors.tokenEndpointAuthMethod ? 'error' : undefined"
              :value="editForm.tokenEndpointAuthMethod"
              @change="handleAuthMethodChange"
            />
            <p
              v-if="errors.tokenEndpointAuthMethod"
              class="m-0 text-xs text-red-500"
            >
              {{ errors.tokenEndpointAuthMethod }}
            </p>
            <Alert
              v-if="willWipeSecretsOnSave"
              class="mt-2"
              description="保存为无密钥公共客户端（NONE）后将立即清空当前全部客户端密钥，且清空后无法直接切回机密客户端认证方式。"
              message="切换为公共客户端将永久清除现有密钥"
              show-icon
              type="warning"
            />
          </div>

          <div class="space-y-2">
            <label class="text-foreground block text-sm font-medium">
              允许的授权类型 <span class="text-red-500">*</span>
            </label>
            <div class="flex flex-col gap-2">
              <Checkbox
                v-for="opt in grantTypeOptions"
                :key="opt.value"
                :checked="editForm.allowedGrantTypes.includes(opt.value)"
                :disabled="opt.disabled"
                @change="
                  (e) =>
                    handleToggleGrantType(opt.value, Boolean(e.target.checked))
                "
              >
                {{ opt.label }}
              </Checkbox>
            </div>
            <p v-if="errors.allowedGrantTypes" class="m-0 text-xs text-red-500">
              {{ errors.allowedGrantTypes }}
            </p>
          </div>

          <template v-if="hasAuthCode">
            <div class="space-y-1.5">
              <label class="text-foreground block text-sm font-medium">
                登录回调地址白名单（redirect_uris）
                <span class="text-red-500">*</span>
              </label>
              <Textarea
                v-model:value="editForm.redirectUrisText"
                class="font-mono text-xs"
                placeholder="每行填写一个完整回调地址"
                :rows="3"
                :status="errors.redirectUrisText ? 'error' : undefined"
                @change="clearFieldError('redirectUrisText')"
              />
              <p
                v-if="errors.redirectUrisText"
                class="m-0 text-xs text-red-500"
              >
                {{ errors.redirectUrisText }}
              </p>
            </div>

            <div class="space-y-1.5">
              <label class="text-foreground block text-sm font-medium">
                登出跳回地址白名单（post_logout_redirect_uris）
              </label>
              <Textarea
                v-model:value="editForm.postLogoutRedirectUrisText"
                class="font-mono text-xs"
                placeholder="每行填写一个登出跳回地址（选填）"
                :rows="2"
                :status="
                  errors.postLogoutRedirectUrisText ? 'error' : undefined
                "
                @change="clearFieldError('postLogoutRedirectUrisText')"
              />
              <p
                v-if="errors.postLogoutRedirectUrisText"
                class="m-0 text-xs text-red-500"
              >
                {{ errors.postLogoutRedirectUrisText }}
              </p>
            </div>

            <div class="space-y-2">
              <label class="text-foreground block text-sm font-medium">
                允许的权限范围（Allowed Scopes）
                <span class="text-red-500">*</span>
              </label>
              <div class="grid grid-cols-1 gap-2 sm:grid-cols-2">
                <div
                  v-for="scope in STANDARD_OIDC_SCOPES"
                  :key="scope.value"
                  class="border-border rounded-md border px-3 py-2"
                >
                  <Checkbox
                    :checked="editForm.allowedScopes.includes(scope.value)"
                    :disabled="scope.value === 'openid'"
                    @change="
                      (e) =>
                        handleToggleScope(
                          scope.value,
                          Boolean(e.target.checked),
                        )
                    "
                  >
                    <span class="text-sm font-medium">{{ scope.label }}</span>
                  </Checkbox>
                </div>
              </div>
              <p v-if="errors.allowedScopes" class="m-0 text-xs text-red-500">
                {{ errors.allowedScopes }}
              </p>
            </div>

            <div
              class="border-border bg-muted/30 flex items-center justify-between gap-4 rounded-md border px-3.5 py-3"
            >
              <div class="space-y-0.5">
                <div class="text-foreground text-sm font-medium">
                  第一方受信应用（免用户授权同意页）
                </div>
                <div class="text-muted-foreground text-xs">
                  开启后跳过 <code class="font-mono">/consent/</code>
                  确认页直接签发授权码。
                </div>
              </div>
              <Switch v-model:checked="editForm.trusted" />
            </div>
          </template>

          <div v-if="hasClientCredentials" class="space-y-1.5">
            <label class="text-foreground block text-sm font-medium">
              目标服务受众（Audiences） <span class="text-red-500">*</span>
            </label>
            <Textarea
              v-model:value="editForm.audiencesText"
              class="font-mono text-xs"
              placeholder="每行填写一个目标服务受众标识"
              :rows="2"
              :status="errors.audiencesText ? 'error' : undefined"
              @change="clearFieldError('audiencesText')"
            />
            <p v-if="errors.audiencesText" class="m-0 text-xs text-red-500">
              {{ errors.audiencesText }}
            </p>
          </div>

          <Divider class="!my-3" />

          <Card size="small" title="危险操作区">
            <div class="space-y-3 text-xs">
              <div class="flex flex-wrap items-center justify-between gap-3">
                <div class="space-y-0.5">
                  <div class="text-foreground font-medium">
                    {{
                      detailClient.status === 'CLIENT_STATUS_ACTIVE'
                        ? '禁用该客户端'
                        : '重新启用该客户端'
                    }}
                  </div>
                  <div class="text-muted-foreground">
                    {{
                      detailClient.status === 'CLIENT_STATUS_ACTIVE'
                        ? '禁用后将立即阻断新令牌签发，并级联撤销当前全部活跃令牌与在途登录请求。'
                        : '重新恢复客户端正常令牌签发能力（此前已撤销的会话不会自动恢复）。'
                    }}
                  </div>
                </div>

                <Popconfirm
                  v-if="detailClient.status === 'CLIENT_STATUS_ACTIVE'"
                  cancel-text="取消"
                  ok-text="确认禁用"
                  title="禁用后将立即撤销该客户端所有活跃令牌与在途授权流程，确认禁用？"
                  @confirm="handleToggleClientStatus"
                >
                  <Button danger :loading="statusChanging" size="small">
                    禁用客户端
                  </Button>
                </Popconfirm>
                <Popconfirm
                  v-else
                  cancel-text="取消"
                  ok-text="确认启用"
                  title="确认重新启用该 OAuth 客户端？"
                  @confirm="handleToggleClientStatus"
                >
                  <Button :loading="statusChanging" size="small" type="primary">
                    启用客户端
                  </Button>
                </Popconfirm>
              </div>

              <Divider class="!my-2" />

              <div class="flex flex-wrap items-center justify-between gap-3">
                <div class="space-y-0.5">
                  <div class="text-foreground font-medium">
                    永久物理删除客户端
                  </div>
                  <div class="text-muted-foreground">
                    彻底移除客户端配置、全部密钥、用户授权记忆及活跃会话，操作不可恢复。
                  </div>
                </div>
                <Button danger size="small" @click="handleRequestDelete">
                  永久删除...
                </Button>
              </div>
            </div>
          </Card>
        </div>
      </template>

      <template v-else>
        <Descriptions
          bordered
          :column="1"
          size="small"
          title="协议与白名单配置（只读）"
        >
          <Descriptions.Item label="备注说明">
            {{ detailClient.description || '—' }}
          </Descriptions.Item>
          <Descriptions.Item label="图标 URL">
            <span class="break-all font-mono text-xs">
              {{ detailClient.logoUri || '—' }}
            </span>
          </Descriptions.Item>
          <Descriptions.Item label="允许的授权类型">
            <Space v-if="detailClient.allowedGrantTypes?.length" wrap>
              <Tag
                v-for="grant in detailClient.allowedGrantTypes"
                :key="grant"
                color="default"
              >
                {{ grant }}
              </Tag>
            </Space>
            <span v-else>—</span>
          </Descriptions.Item>
          <Descriptions.Item label="允许的 Scopes">
            <Space v-if="detailClient.allowedScopes?.length" wrap>
              <Tag
                v-for="scope in detailClient.allowedScopes"
                :key="scope"
                color="blue"
              >
                {{ scope }}
              </Tag>
            </Space>
            <span v-else>—</span>
          </Descriptions.Item>
          <Descriptions.Item label="登录回调地址（redirect_uris）">
            <div v-if="detailClient.redirectUris?.length" class="space-y-1">
              <div
                v-for="uri in detailClient.redirectUris"
                :key="uri"
                class="break-all font-mono text-xs"
              >
                {{ uri }}
              </div>
            </div>
            <span v-else>—</span>
          </Descriptions.Item>
          <Descriptions.Item label="登出跳回地址（post_logout_redirect_uris）">
            <div
              v-if="detailClient.postLogoutRedirectUris?.length"
              class="space-y-1"
            >
              <div
                v-for="uri in detailClient.postLogoutRedirectUris"
                :key="uri"
                class="break-all font-mono text-xs"
              >
                {{ uri }}
              </div>
            </div>
            <span v-else>—</span>
          </Descriptions.Item>
          <Descriptions.Item label="目标受众（audiences）">
            <Space v-if="detailClient.audiences?.length" wrap>
              <Tag
                v-for="aud in detailClient.audiences"
                :key="aud"
                color="geekblue"
              >
                {{ aud }}
              </Tag>
            </Space>
            <span v-else>—</span>
          </Descriptions.Item>
        </Descriptions>
      </template>
    </div>

    <CreateSecretModal @created="handleSecretCreated" />
  </Drawer>
</template>
