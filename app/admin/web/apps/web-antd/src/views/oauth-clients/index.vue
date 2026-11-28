<script lang="ts" setup>
import type { CreateOAuthClientSecretResponse } from '@plateau/api/plateau/admin/client/v1';
import type { OAuthClient } from '@plateau/api/plateau/iam/client/v1';

import type {
  CursorPaginationState,
  OAuthClientsSearchFormValues,
} from './schemas';

import { computed, nextTick, onUnmounted, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';

import { Page, useVbenDrawer, useVbenModal } from '@vben/common-ui';
import { LOGIN_PATH } from '@vben/constants';

import {
  Avatar,
  Button,
  message,
  Popconfirm,
  Select,
  Space,
  Tag,
  Tooltip,
} from 'ant-design-vue';

import { useVbenVxeGrid } from '#/adapter/vxe-table';
import {
  disableOAuthClient,
  enableOAuthClient,
  listOAuthClients,
  resolveAdminApiError,
} from '#/api';
import { useAuthStore } from '#/store';

import CreateOAuthClientDrawerComp from './components/create-oauth-client-drawer.vue';
import DeleteOAuthClientModalComp from './components/delete-oauth-client-modal.vue';
import OAuthClientDetailDrawerComp from './components/oauth-client-detail-drawer.vue';
import SecretRevealModalComp from './components/secret-reveal-modal.vue';
import {
  applyCursorPageResult,
  buildOAuthClientsListRequest,
  createInitialCursorState,
  getClientArchitectureBadge,
  getOAuthClientsGridColumns,
  getOAuthClientsSearchFormSchema,
  getSourceBadge,
  getStatusBadge,
  getTrustedPolicyBadge,
  isConfidentialOAuthClient,
  isStaticOAuthClient,
  normalizeSafeHttpImageUrl,
  resolveOAuthClientDisplayName,
  resolveOAuthClientResourceName,
  stepCursorNext,
  stepCursorPrev,
} from './schemas';

defineOptions({ name: 'AdminOAuthClientsPage' });

const route = useRoute();
const router = useRouter();
const authStore = useAuthStore();

const cursorState = ref<CursorPaginationState>(createInitialCursorState(20));
const listLoading = ref(false);
const lastQuerySignature = ref('');
const preserveCursorOnce = ref(false);
const busyClientMap = ref<Record<string, boolean>>({});

let listAbortController: AbortController | null = null;

function cancelListRequest() {
  listAbortController?.abort();
  listAbortController = null;
}

onUnmounted(() => {
  cancelListRequest();
});

const canManageClients = computed(() =>
  authStore.hasPermission('manage_clients'),
);

function isRowBusy(row: OAuthClient): boolean {
  const key = row.clientId || row.name || '';
  return Boolean(key && busyClientMap.value[key]);
}

function setRowBusy(row: OAuthClient, busy: boolean) {
  const key = row.clientId || row.name || '';
  if (!key) {
    return;
  }
  busyClientMap.value = {
    ...busyClientMap.value,
    [key]: busy,
  };
}

async function handleUnauthenticatedRedirect(msg: string) {
  message.warning(msg);
  authStore.markUnauthenticated();
  await router.replace({
    path: LOGIN_PATH,
    query: { redirect: router.currentRoute.value.fullPath },
  });
}

async function copyClientId(clientId?: string) {
  if (!clientId) {
    return;
  }
  try {
    await navigator.clipboard.writeText(clientId);
    message.success('已复制 Client ID');
  } catch {
    message.error('复制 Client ID 失败，请手动选中复制');
  }
}

const [CreateDrawer, createDrawerApi] = useVbenDrawer({
  connectedComponent: CreateOAuthClientDrawerComp,
});

const [DetailDrawer, detailDrawerApi] = useVbenDrawer({
  connectedComponent: OAuthClientDetailDrawerComp,
});

const [SecretRevealModal, secretRevealModalApi] = useVbenModal({
  connectedComponent: SecretRevealModalComp,
});

const [DeleteClientModal, deleteClientModalApi] = useVbenModal({
  connectedComponent: DeleteOAuthClientModalComp,
});

const [Grid, gridApi] = useVbenVxeGrid<
  OAuthClient,
  OAuthClientsSearchFormValues
>({
  formOptions: {
    collapsed: false,
    schema: getOAuthClientsSearchFormSchema(),
    showCollapseButton: false,
    submitOnChange: false,
    submitOnEnter: true,
    wrapperClass: 'grid-cols-1 md:grid-cols-2 lg:grid-cols-5',
  },
  gridOptions: {
    columns: getOAuthClientsGridColumns(),
    height: 'auto',
    keepSource: false,
    pagerConfig: {
      enabled: false,
    },
    proxyConfig: {
      ajax: {
        query: async (
          _params: unknown,
          rawFormValues?: OAuthClientsSearchFormValues,
        ) => {
          const formValues: OAuthClientsSearchFormValues = rawFormValues ?? {};
          const signaturePayload = buildOAuthClientsListRequest(
            formValues,
            cursorState.value.pageSize,
            '',
          );
          const nextSignature = JSON.stringify({
            filter: signaturePayload.filter ?? '',
            orderBy: signaturePayload.orderBy ?? '',
            pageSize: cursorState.value.pageSize,
          });

          if (
            !preserveCursorOnce.value &&
            nextSignature !== lastQuerySignature.value
          ) {
            cursorState.value = {
              ...createInitialCursorState(cursorState.value.pageSize),
            };
          }
          preserveCursorOnce.value = false;
          lastQuerySignature.value = nextSignature;

          return await executeListQuery(formValues);
        },
      },
      autoLoad: true,
      enabled: true,
    },
    rowConfig: {
      isHover: true,
      keyField: 'clientId',
    },
    showHeaderOverflow: 'tooltip',
    showOverflow: false,
    toolbarConfig: {
      custom: false,
      refresh: false,
      search: true,
      zoom: false,
    },
  },
});

async function executeListQuery(
  formValues: OAuthClientsSearchFormValues,
  retriedInvalidToken = false,
): Promise<{ items: OAuthClient[]; total: number }> {
  cancelListRequest();
  const controller = new AbortController();
  listAbortController = controller;
  listLoading.value = true;

  try {
    const activeToken =
      cursorState.value.cursorStack[cursorState.value.currentPageIndex] ?? '';
    const request = buildOAuthClientsListRequest(
      formValues,
      cursorState.value.pageSize,
      activeToken,
    );
    const response = await listOAuthClients(request, controller.signal);
    const clients = response.clients ?? [];
    const applied = applyCursorPageResult(
      cursorState.value,
      clients.length,
      response.nextPageToken,
      response.totalSize,
    );
    cursorState.value = applied.state;

    if (applied.fallbackToPreviousPage) {
      return await executeListQuery(formValues, retriedInvalidToken);
    }

    return {
      items: clients,
      total: cursorState.value.totalSize,
    };
  } catch (error) {
    const resolved = resolveAdminApiError(error);
    if (!resolved.silent) {
      if (!resolved.preserveSession) {
        await handleUnauthenticatedRedirect(resolved.message);
      } else if (
        resolved.category === 'invalid_argument' &&
        cursorState.value.currentPageIndex > 0 &&
        !retriedInvalidToken
      ) {
        cursorState.value = createInitialCursorState(
          cursorState.value.pageSize,
        );
        return await executeListQuery(formValues, true);
      } else {
        message.error(resolved.message);
      }
    }
    return {
      items: [],
      total: cursorState.value.totalSize,
    };
  } finally {
    if (listAbortController === controller) {
      listLoading.value = false;
    }
  }
}

async function reloadKeepingCurrentCursor() {
  preserveCursorOnce.value = true;
  await gridApi.query();
}

async function handleFirstPage() {
  if (cursorState.value.currentPageIndex === 0) {
    return;
  }
  cursorState.value = createInitialCursorState(cursorState.value.pageSize);
  await reloadKeepingCurrentCursor();
}

async function handlePrevPage() {
  const prev = stepCursorPrev(cursorState.value);
  if (!prev) {
    return;
  }
  cursorState.value = prev;
  await reloadKeepingCurrentCursor();
}

async function handleNextPage() {
  const next = stepCursorNext(cursorState.value);
  if (!next) {
    return;
  }
  cursorState.value = next;
  await reloadKeepingCurrentCursor();
}

async function handlePageSizeChange(nextSize: unknown) {
  const parsed = Number(nextSize) || 20;
  cursorState.value = createInitialCursorState(parsed);
  lastQuerySignature.value = '';
  await gridApi.query();
}

function openCreateDrawer() {
  if (!canManageClients.value) {
    return;
  }
  createDrawerApi.open();
}

function openDetailDrawer(row: OAuthClient) {
  detailDrawerApi
    .setData({ resourceName: resolveOAuthClientResourceName(row) })
    .open();
}

function openDeleteModal(row: OAuthClient) {
  if (isStaticOAuthClient(row) || !canManageClients.value) {
    return;
  }
  deleteClientModalApi.setData({ client: row }).open();
}

async function handleClientCreated(payload: {
  client: OAuthClient;
  clientSecret?: string;
}) {
  cursorState.value = createInitialCursorState(cursorState.value.pageSize);
  lastQuerySignature.value = '';
  await gridApi.query();

  if (payload.clientSecret) {
    secretRevealModalApi
      .setData({
        clientId: payload.client.clientId ?? '',
        displayName: payload.client.displayName,
        mode: 'created_client',
        secretValue: payload.clientSecret,
      })
      .open();
  }
}

function handleSecretCreatedFromDetail(payload: {
  client: OAuthClient;
  response: CreateOAuthClientSecretResponse;
}) {
  const secretValue = payload.response.secretValue?.trim();
  if (!secretValue) {
    return;
  }
  secretRevealModalApi
    .setData({
      clientId: payload.client.clientId ?? '',
      displayName: payload.client.displayName,
      mode: 'created_secret',
      secretName: payload.response.secret?.name,
      secretPrefix: payload.response.secret?.secretPrefix,
      secretValue,
    })
    .open();
}

async function handleDisableClient(row: OAuthClient) {
  if (isStaticOAuthClient(row)) {
    return;
  }
  const name = resolveOAuthClientResourceName(row);
  setRowBusy(row, true);
  try {
    await disableOAuthClient({
      etag: row.etag ?? '',
      name,
    });
    message.success('客户端已禁用，活跃会话与在途授权已撤销');
    await reloadKeepingCurrentCursor();
  } catch (error) {
    const resolved = resolveAdminApiError(error, 'update');
    if (!resolved.silent) {
      if (resolved.preserveSession) {
        message.error(resolved.message);
        if (resolved.shouldRefreshRecord) {
          await reloadKeepingCurrentCursor();
        }
      } else {
        await handleUnauthenticatedRedirect(resolved.message);
      }
    }
  } finally {
    setRowBusy(row, false);
  }
}

async function handleEnableClient(row: OAuthClient) {
  if (isStaticOAuthClient(row)) {
    return;
  }
  const name = resolveOAuthClientResourceName(row);
  setRowBusy(row, true);
  try {
    await enableOAuthClient({
      etag: row.etag ?? '',
      name,
    });
    message.success('客户端已重新启用');
    await reloadKeepingCurrentCursor();
  } catch (error) {
    const resolved = resolveAdminApiError(error, 'update');
    if (!resolved.silent) {
      if (resolved.preserveSession) {
        message.error(resolved.message);
        if (resolved.shouldRefreshRecord) {
          await reloadKeepingCurrentCursor();
        }
      } else {
        await handleUnauthenticatedRedirect(resolved.message);
      }
    }
  } finally {
    setRowBusy(row, false);
  }
}

function getDetailActionLabel(row: OAuthClient): string {
  if (isStaticOAuthClient(row) || !canManageClients.value) {
    return '查看详情';
  }
  if (isConfidentialOAuthClient(row)) {
    return '详情 / 密钥';
  }
  return '详情';
}

watch(
  () => route.query.action,
  async (action) => {
    if (action !== 'create') {
      return;
    }
    const nextQuery = { ...route.query };
    delete nextQuery.action;
    await router.replace({ path: '/oauth-clients', query: nextQuery });
    await nextTick();
    if (canManageClients.value) {
      createDrawerApi.open();
    }
  },
  { flush: 'post', immediate: true },
);

watch(
  () => route.query.clientId,
  async (rawClientId) => {
    const cleanClientId = Array.isArray(rawClientId)
      ? rawClientId[0]?.trim()
      : rawClientId?.trim();
    if (!cleanClientId) {
      return;
    }
    const nextQuery = { ...route.query };
    delete nextQuery.clientId;
    await router.replace({ path: '/oauth-clients', query: nextQuery });
    await nextTick();
    detailDrawerApi
      .setData({
        resourceName: resolveOAuthClientResourceName({
          clientId: cleanClientId,
        }),
      })
      .open();
  },
  { flush: 'post', immediate: true },
);
</script>

<template>
  <Page auto-content-height title="OAuth 2.0 / OIDC 客户端管理">
    <div class="flex h-full flex-col gap-3">
      <div class="min-h-0 flex-1">
        <Grid>
          <template #toolbar-actions>
            <Space wrap>
              <Tooltip
                :title="
                  canManageClients
                    ? undefined
                    : '当前账号缺少 manage_clients 权限'
                "
              >
                <Button
                  :disabled="!canManageClients"
                  type="primary"
                  @click="openCreateDrawer"
                >
                  新建客户端
                </Button>
              </Tooltip>
              <Button
                :loading="listLoading"
                @click="reloadKeepingCurrentCursor"
              >
                刷新列表
              </Button>
            </Space>
          </template>

          <template #clientIdentity="{ row }">
            <div class="flex items-center gap-2.5 py-1 text-left">
              <Avatar
                class="shrink-0"
                shape="square"
                :size="32"
                :src="normalizeSafeHttpImageUrl(row.logoUri) || undefined"
              >
                {{
                  resolveOAuthClientDisplayName(row).slice(0, 1).toUpperCase()
                }}
              </Avatar>
              <div class="flex min-w-0 flex-1 flex-col gap-0.5">
                <span
                  class="text-foreground truncate font-medium leading-5"
                  :title="resolveOAuthClientDisplayName(row)"
                >
                  {{ resolveOAuthClientDisplayName(row) }}
                </span>
                <span
                  class="text-muted-foreground truncate text-xs leading-4"
                  :title="row.description || undefined"
                >
                  {{ row.description || '未设置备注说明' }}
                </span>
              </div>
            </div>
          </template>

          <template #clientId="{ row }">
            <div class="flex items-center gap-1 text-left">
              <span
                class="truncate font-mono text-xs"
                :title="row.clientId || undefined"
              >
                {{ row.clientId || '—' }}
              </span>
              <Button
                v-if="row.clientId"
                size="small"
                type="link"
                @click.stop="copyClientId(row.clientId)"
              >
                复制
              </Button>
            </div>
          </template>

          <template #source="{ row }">
            <Tooltip
              :title="
                isStaticOAuthClient(row)
                  ? '由 IAM oidc.yaml 静态引导配置托管，仅支持只读查看'
                  : '由管理 API 动态注册托管'
              "
            >
              <Tag :color="getSourceBadge(row.source).color">
                {{ getSourceBadge(row.source).text }}
              </Tag>
            </Tooltip>
          </template>

          <template #architecture="{ row }">
            <Space :size="4" wrap>
              <Tag :color="getClientArchitectureBadge(row).color">
                {{ getClientArchitectureBadge(row).modeLabel }}
              </Tag>
              <span class="text-muted-foreground text-xs">
                {{ getClientArchitectureBadge(row).authLabel }}
              </span>
            </Space>
          </template>

          <template #trusted="{ row }">
            <Tag :color="getTrustedPolicyBadge(row).color">
              {{ getTrustedPolicyBadge(row).text }}
            </Tag>
          </template>

          <template #status="{ row }">
            <Tag :color="getStatusBadge(row.status).color">
              {{ getStatusBadge(row.status).text }}
            </Tag>
          </template>

          <template #actions="{ row }">
            <Space class="client-row-actions justify-end" :size="4" wrap>
              <Button size="small" type="link" @click="openDetailDrawer(row)">
                {{ getDetailActionLabel(row) }}
              </Button>

              <template v-if="canManageClients">
                <Tooltip
                  v-if="isStaticOAuthClient(row)"
                  title="该客户端由 IAM 静态配置文件（oidc.yaml）引导托管，不支持在线编辑"
                >
                  <Button disabled size="small" type="link">编辑</Button>
                </Tooltip>
                <Button
                  v-else
                  :disabled="isRowBusy(row)"
                  size="small"
                  type="link"
                  @click="openDetailDrawer(row)"
                >
                  编辑
                </Button>

                <Tooltip
                  v-if="isStaticOAuthClient(row)"
                  title="该客户端由 IAM 静态配置文件（oidc.yaml）引导托管，不支持在线启停"
                >
                  <Button disabled size="small" type="link">禁用</Button>
                </Tooltip>
                <Popconfirm
                  v-else-if="row.status !== 'CLIENT_STATUS_DISABLED'"
                  cancel-text="取消"
                  ok-text="确认禁用"
                  title="禁用后将立即阻断该客户端令牌签发，并撤销全部活跃会话与在途登录请求，确认禁用？"
                  @confirm="handleDisableClient(row)"
                >
                  <Button
                    danger
                    :loading="isRowBusy(row)"
                    size="small"
                    type="link"
                  >
                    禁用
                  </Button>
                </Popconfirm>
                <Popconfirm
                  v-else
                  cancel-text="取消"
                  ok-text="确认启用"
                  title="确认重新启用该 OAuth 客户端？"
                  @confirm="handleEnableClient(row)"
                >
                  <Button :loading="isRowBusy(row)" size="small" type="link">
                    启用
                  </Button>
                </Popconfirm>

                <Tooltip
                  v-if="isStaticOAuthClient(row)"
                  title="该客户端由 IAM 静态配置文件（oidc.yaml）引导托管，不支持在线删除"
                >
                  <Button danger disabled size="small" type="link">删除</Button>
                </Tooltip>
                <Button
                  v-else
                  danger
                  :disabled="isRowBusy(row)"
                  size="small"
                  type="link"
                  @click="openDeleteModal(row)"
                >
                  删除
                </Button>
              </template>
            </Space>
          </template>
        </Grid>
      </div>

      <div
        class="bg-card border-border flex flex-wrap items-center justify-between gap-3 rounded-md border px-4 py-2.5 text-sm"
      >
        <div class="text-muted-foreground">
          共
          <span class="text-foreground font-medium">{{
            cursorState.totalSize
          }}</span>
          条记录 · 当前第
          <span class="text-foreground font-medium">{{
            cursorState.currentPageIndex + 1
          }}</span>
          页
        </div>

        <Space wrap>
          <Select
            :disabled="listLoading"
            :options="[
              { label: '10 条 / 页', value: 10 },
              { label: '20 条 / 页', value: 20 },
              { label: '50 条 / 页', value: 50 },
            ]"
            size="small"
            style="width: 112px"
            :value="cursorState.pageSize"
            @change="handlePageSizeChange"
          />
          <Button
            :disabled="cursorState.currentPageIndex === 0 || listLoading"
            size="small"
            @click="handleFirstPage"
          >
            首页
          </Button>
          <Button
            :disabled="cursorState.currentPageIndex === 0 || listLoading"
            size="small"
            @click="handlePrevPage"
          >
            上一页
          </Button>
          <Button
            :disabled="!cursorState.nextPageToken || listLoading"
            size="small"
            type="primary"
            @click="handleNextPage"
          >
            下一页
          </Button>
        </Space>
      </div>
    </div>

    <CreateDrawer @created="handleClientCreated" />
    <DetailDrawer
      @request-delete="openDeleteModal"
      @secret-created="handleSecretCreatedFromDetail"
      @updated="reloadKeepingCurrentCursor"
    />
    <SecretRevealModal />
    <DeleteClientModal @deleted="reloadKeepingCurrentCursor" />
  </Page>
</template>

<style scoped>
:deep(.vxe-body--column > .vxe-cell > .vxe-cell--wrapper) {
  min-width: 0;
  max-width: 100%;
}

:deep(.client-row-actions .ant-btn-link) {
  padding-inline: 4px;
}
</style>
