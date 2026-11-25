<script lang="ts" setup>
import type { User } from '@plateau/api/iam/user/v1';

import type {
  CursorPaginationState,
  UsersSearchFormValues,
  UsersViewMode,
} from './schemas';

import { computed, nextTick, onUnmounted, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';

import { Page, useVbenDrawer, useVbenModal } from '@vben/common-ui';
import { LOGIN_PATH } from '@vben/constants';

import {
  Alert,
  Modal as AntModal,
  Avatar,
  Button,
  Descriptions,
  message,
  Popconfirm,
  Segmented,
  Select,
  Space,
  Tag,
  Tooltip,
} from 'ant-design-vue';

import { useVbenVxeGrid } from '#/adapter/vxe-table';
import { createAdminApi, resolveAdminApiError } from '#/api';
import { resolveUserDisplayName, useAuthStore } from '#/store';

import CreateUserModalComp from './components/create-user-modal.vue';
import ExpungeUserModalComp from './components/expunge-user-modal.vue';
import SetPasswordModalComp from './components/set-password-modal.vue';
import UserDetailDrawerComp from './components/user-detail-drawer.vue';
import {
  applyCursorPageResult,
  buildUsersListRequest,
  createInitialCursorState,
  getUsersGridColumns,
  getUsersSearchFormSchema,
  resolveUserResourceName,
  stepCursorNext,
  stepCursorPrev,
} from './schemas';

defineOptions({ name: 'AdminUsersPage' });

const route = useRoute();
const router = useRouter();
const authStore = useAuthStore();

const initialView: UsersViewMode =
  route.query.view === 'deleted' ? 'deleted' : 'active';
const viewMode = ref<UsersViewMode>(initialView);

const cursorState = ref<CursorPaginationState>(createInitialCursorState(20));
const listLoading = ref(false);
const lastQuerySignature = ref('');
const preserveCursorOnce = ref(false);
const busyUserMap = ref<Record<string, boolean>>({});

const degradedCreatedUser = ref<null | User>(null);
const degradedModalOpen = ref(false);
const resendingDegradedMail = ref(false);

let listAbortController: AbortController | null = null;

function cancelListRequest() {
  listAbortController?.abort();
  listAbortController = null;
}

onUnmounted(() => {
  cancelListRequest();
});

const canManageUsers = computed(() => authStore.hasPermission('manage_users'));
const canExpungeUsers = computed(() =>
  authStore.hasPermission('expunge_users'),
);

function isSelfUser(row: User): boolean {
  return Boolean(row.userId && row.userId === authStore.currentUser?.userId);
}

function isRowBusy(row: User): boolean {
  const key = row.userId || row.name || '';
  return Boolean(key && busyUserMap.value[key]);
}

function setRowBusy(row: User, busy: boolean) {
  const key = row.userId || row.name || '';
  if (!key) {
    return;
  }
  busyUserMap.value = {
    ...busyUserMap.value,
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

const [DetailDrawer, detailDrawerApi] = useVbenDrawer({
  connectedComponent: UserDetailDrawerComp,
});

const [CreateUserModal, createUserModalApi] = useVbenModal({
  connectedComponent: CreateUserModalComp,
});

const [SetPasswordModal, setPasswordModalApi] = useVbenModal({
  connectedComponent: SetPasswordModalComp,
});

const [ExpungeUserModal, expungeUserModalApi] = useVbenModal({
  connectedComponent: ExpungeUserModalComp,
});

const [Grid, gridApi] = useVbenVxeGrid<User, UsersSearchFormValues>({
  formOptions: {
    collapsed: false,
    schema: getUsersSearchFormSchema(initialView),
    showCollapseButton: false,
    submitOnChange: false,
    submitOnEnter: true,
    wrapperClass: 'grid-cols-1 md:grid-cols-2 lg:grid-cols-4',
  },
  gridOptions: {
    columns: getUsersGridColumns(initialView),
    height: 'auto',
    keepSource: false,
    pagerConfig: {
      enabled: false,
    },
    proxyConfig: {
      ajax: {
        query: async (
          _params: unknown,
          rawFormValues?: UsersSearchFormValues,
        ) => {
          const formValues: UsersSearchFormValues = rawFormValues ?? {};
          const signaturePayload = buildUsersListRequest(
            viewMode.value,
            formValues,
            cursorState.value.pageSize,
            '',
          );
          const nextSignature = JSON.stringify({
            filter: signaturePayload.filter ?? '',
            orderBy: signaturePayload.orderBy ?? '',
            pageSize: cursorState.value.pageSize,
            viewMode: viewMode.value,
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
      keyField: 'userId',
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
  formValues: UsersSearchFormValues,
): Promise<{ items: User[]; total: number }> {
  cancelListRequest();
  const controller = new AbortController();
  listAbortController = controller;
  listLoading.value = true;

  try {
    const activeToken =
      cursorState.value.cursorStack[cursorState.value.currentPageIndex] ?? '';
    const request = buildUsersListRequest(
      viewMode.value,
      formValues,
      cursorState.value.pageSize,
      activeToken,
    );
    const response = await createAdminApi(controller.signal).users.ListUsers(
      request,
    );
    const users = response.users ?? [];
    const applied = applyCursorPageResult(
      cursorState.value,
      users.length,
      response.nextPageToken,
      response.totalSize,
    );
    cursorState.value = applied.state;

    if (applied.fallbackToPreviousPage) {
      return await executeListQuery(formValues);
    }

    return {
      items: users,
      total: cursorState.value.totalSize,
    };
  } catch (error) {
    const resolved = resolveAdminApiError(error);
    if (!resolved.silent) {
      if (resolved.preserveSession) {
        message.error(resolved.message);
      } else {
        await handleUnauthenticatedRedirect(resolved.message);
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

async function handleSwitchView(nextValue: number | string) {
  const nextMode: UsersViewMode =
    nextValue === 'deleted' ? 'deleted' : 'active';
  if (nextMode === viewMode.value) {
    return;
  }
  cancelListRequest();
  viewMode.value = nextMode;
  cursorState.value = createInitialCursorState(cursorState.value.pageSize);
  lastQuerySignature.value = '';

  await router.replace({
    path: '/users',
    query: nextMode === 'deleted' ? { view: 'deleted' } : {},
  });

  gridApi.setState({
    formOptions: {
      collapsed: false,
      schema: getUsersSearchFormSchema(nextMode),
      showCollapseButton: false,
      submitOnChange: false,
      submitOnEnter: true,
      wrapperClass: 'grid-cols-1 md:grid-cols-2 lg:grid-cols-4',
    },
  });
  gridApi.setGridOptions({
    columns: getUsersGridColumns(nextMode),
    data: [],
  });
  await nextTick();
  await gridApi.formApi.resetForm();
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

function openCreateModal() {
  if (!canManageUsers.value) {
    return;
  }
  createUserModalApi.open();
}

function openDetailDrawer(row: User) {
  detailDrawerApi
    .setData({ resourceName: resolveUserResourceName(row) })
    .open();
}

function openSetPasswordModal(row: User) {
  setPasswordModalApi.setData({ user: row }).open();
}

function openExpungeModal(row: User) {
  if (isSelfUser(row) || !canExpungeUsers.value) {
    return;
  }
  expungeUserModalApi.setData({ user: row }).open();
}

async function handleUserCreated(payload: { emailSent: boolean; user: User }) {
  cursorState.value = createInitialCursorState(cursorState.value.pageSize);
  lastQuerySignature.value = '';
  await gridApi.query();
  if (!payload.emailSent) {
    degradedCreatedUser.value = payload.user;
    degradedModalOpen.value = true;
  }
}

async function handleResendVerificationEmail(
  row: User,
  fromDegradedModal = false,
) {
  const name = resolveUserResourceName(row);
  if (fromDegradedModal) {
    resendingDegradedMail.value = true;
  } else {
    setRowBusy(row, true);
  }
  try {
    await createAdminApi().users.SendUserVerificationEmail({ name });
    message.success(`已向 ${row.email || name} 发送验证邮件`);
    if (fromDegradedModal) {
      degradedModalOpen.value = false;
      degradedCreatedUser.value = null;
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
    message.error(resolved.message);
    if (resolved.shouldRefreshRecord) {
      await reloadKeepingCurrentCursor();
    }
  } finally {
    if (fromDegradedModal) {
      resendingDegradedMail.value = false;
    } else {
      setRowBusy(row, false);
    }
  }
}

function handleConfirmRevokeIamSessions(row: User) {
  const name = resolveUserResourceName(row);
  AntModal.confirm({
    cancelText: '取消',
    content:
      '仅撤销该账号在 IAM 的登录会话与刷新令牌，使其无法通过 IAM 继续签发新令牌；不会自动终止 Admin 或其他应用已建立的本地会话。确认执行？',
    okText: '确认强制登出 IAM',
    okType: 'primary',
    async onOk() {
      setRowBusy(row, true);
      try {
        await createAdminApi().users.RevokeUserSessions({ name });
        message.success('已撤销该账号在 IAM 的全部登录与刷新会话');
      } catch (error) {
        const resolved = resolveAdminApiError(error);
        if (!resolved.silent) {
          if (resolved.preserveSession) {
            message.error(resolved.message);
          } else {
            await handleUnauthenticatedRedirect(resolved.message);
          }
        }
      } finally {
        setRowBusy(row, false);
      }
    },
    title: `强制登出 IAM：${row.email || row.userId}`,
  });
}

async function handleDisableUser(row: User) {
  if (isSelfUser(row)) {
    return;
  }
  const name = resolveUserResourceName(row);
  setRowBusy(row, true);
  try {
    await createAdminApi().users.DisableUser({
      etag: row.etag ?? '',
      name,
    });
    message.success('账号已封禁');
    await reloadKeepingCurrentCursor();
  } catch (error) {
    const resolved = resolveAdminApiError(error);
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

async function handleEnableUser(row: User) {
  const name = resolveUserResourceName(row);
  setRowBusy(row, true);
  try {
    await createAdminApi().users.EnableUser({
      etag: row.etag ?? '',
      name,
    });
    message.success('账号已解禁恢复正常');
    await reloadKeepingCurrentCursor();
  } catch (error) {
    const resolved = resolveAdminApiError(error);
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

async function handleSoftDeleteUser(row: User) {
  if (isSelfUser(row)) {
    return;
  }
  const name = resolveUserResourceName(row);
  setRowBusy(row, true);
  try {
    await createAdminApi().users.DeleteUser({ name });
    message.success('账号已移入回收站（无限期保留，原邮箱继续占用）');
    await reloadKeepingCurrentCursor();
  } catch (error) {
    const resolved = resolveAdminApiError(error, 'delete');
    if (!resolved.silent) {
      if (resolved.preserveSession) {
        if (resolved.category === 'not_found') {
          message.warning(resolved.message);
        } else {
          message.error(resolved.message);
        }
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

async function handleUndeleteUser(row: User) {
  const name = resolveUserResourceName(row);
  setRowBusy(row, true);
  try {
    await createAdminApi().users.UndeleteUser({ name });
    message.success('账号已从回收站恢复');
    await reloadKeepingCurrentCursor();
  } catch (error) {
    const resolved = resolveAdminApiError(error, 'undelete');
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

function getStatusBadge(row: User) {
  switch (row.status) {
    case 'USER_STATUS_ACTIVE': {
      return { color: 'success', text: '正常' };
    }
    case 'USER_STATUS_DISABLED': {
      return { color: 'error', text: '已封禁' };
    }
    case 'USER_STATUS_PENDING_EMAIL_VERIFICATION': {
      return { color: 'warning', text: '待邮箱验证' };
    }
    default: {
      return { color: 'default', text: '未知' };
    }
  }
}

watch(
  () => route.query.view,
  async (nextQueryView) => {
    const desired: UsersViewMode =
      nextQueryView === 'deleted' ? 'deleted' : 'active';
    if (desired !== viewMode.value) {
      await handleSwitchView(desired);
    }
  },
);

watch(
  () => route.query.action,
  async (action) => {
    if (action !== 'create') {
      return;
    }
    const nextQuery = { ...route.query };
    delete nextQuery.action;
    await router.replace({ path: '/users', query: nextQuery });
    await nextTick();
    if (canManageUsers.value) {
      createUserModalApi.open();
    }
  },
  { flush: 'post', immediate: true },
);

watch(
  () => route.query.userId,
  async (rawUserId) => {
    const cleanUserId = Array.isArray(rawUserId)
      ? rawUserId[0]?.trim()
      : rawUserId?.trim();
    if (!cleanUserId) {
      return;
    }
    const nextQuery = { ...route.query };
    delete nextQuery.userId;
    await router.replace({ path: '/users', query: nextQuery });
    await nextTick();
    detailDrawerApi
      .setData({
        resourceName: resolveUserResourceName({ userId: cleanUserId }),
      })
      .open();
  },
  { flush: 'post', immediate: true },
);
</script>

<template>
  <Page auto-content-height title="IAM 全局用户管理">
    <div class="flex h-full flex-col gap-3">
      <Alert
        v-if="viewMode === 'deleted'"
        description="已软删除账号将无限期保留（无自动清理倒计时），且继续占用原登录邮箱；可随时恢复账号，或在确认无需保留后输入完整邮箱显式永久删除以释放邮箱。"
        message="回收站保留与邮箱占用规则"
        show-icon
        type="info"
      />

      <div class="min-h-0 flex-1">
        <Grid>
          <template #toolbar-actions>
            <Space wrap>
              <Segmented
                :options="[
                  { label: '正常用户', value: 'active' },
                  { label: '已删除用户（回收站）', value: 'deleted' },
                ]"
                :value="viewMode"
                @change="handleSwitchView"
              />
              <Tooltip
                v-if="viewMode === 'active'"
                :title="
                  canManageUsers ? undefined : '当前账号缺少 manage_users 权限'
                "
              >
                <Button
                  :disabled="!canManageUsers"
                  type="primary"
                  @click="openCreateModal"
                >
                  创建用户
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

          <template #userIdentity="{ row }">
            <div class="flex items-center gap-2.5 py-1 text-left">
              <Avatar
                class="shrink-0"
                :size="32"
                :src="row.profile?.picture?.trim() || undefined"
              >
                {{ resolveUserDisplayName(row).slice(0, 1).toUpperCase() }}
              </Avatar>
              <div class="flex min-w-0 flex-1 flex-col gap-0.5">
                <div class="flex items-center gap-1.5 leading-5">
                  <span
                    class="text-foreground truncate font-medium"
                    :title="row.email || undefined"
                  >
                    {{ row.email || '—' }}
                  </span>
                  <Tag v-if="isSelfUser(row)" class="m-0 shrink-0" color="blue">
                    当前管理员
                  </Tag>
                </div>
                <div
                  class="text-muted-foreground truncate text-xs leading-4"
                  :title="
                    row.profile?.name || row.profile?.nickname || undefined
                  "
                >
                  {{
                    row.profile?.name ||
                    row.profile?.nickname ||
                    '未设置显示姓名'
                  }}
                </div>
              </div>
            </div>
          </template>

          <template #userId="{ row }">
            <span class="font-mono text-xs">{{ row.userId || '—' }}</span>
          </template>

          <template #status="{ row }">
            <Space v-if="row.deleteTime" size="small" wrap>
              <Tag color="volcano">已软删除</Tag>
              <Tag color="default">无限期保留 · 邮箱占用中</Tag>
            </Space>
            <Space v-else size="small" wrap>
              <Tag :color="getStatusBadge(row).color">
                {{ getStatusBadge(row).text }}
              </Tag>
              <Tag :color="row.emailVerified ? 'processing' : 'default'">
                {{ row.emailVerified ? '已验证' : '未验证' }}
              </Tag>
              <Tag
                v-if="
                  row.requiredActions?.includes(
                    'USER_REQUIRED_ACTION_UPDATE_PASSWORD',
                  )
                "
                color="gold"
              >
                待首次改密
              </Tag>
            </Space>
          </template>

          <template #actions="{ row }">
            <Space
              v-if="viewMode === 'active'"
              class="user-row-actions justify-end"
              :size="4"
              wrap
            >
              <Button size="small" type="link" @click="openDetailDrawer(row)">
                {{ canManageUsers ? '详情 / 编辑' : '查看详情' }}
              </Button>

              <template v-if="canManageUsers">
                <Button
                  v-if="row.status === 'USER_STATUS_PENDING_EMAIL_VERIFICATION'"
                  :loading="isRowBusy(row)"
                  size="small"
                  type="link"
                  @click="handleResendVerificationEmail(row)"
                >
                  补发验证信
                </Button>

                <Button
                  :disabled="isRowBusy(row)"
                  size="small"
                  type="link"
                  @click="openSetPasswordModal(row)"
                >
                  设密
                </Button>

                <Button
                  :loading="isRowBusy(row)"
                  size="small"
                  type="link"
                  @click="handleConfirmRevokeIamSessions(row)"
                >
                  强制登出 IAM
                </Button>

                <template v-if="row.status !== 'USER_STATUS_DISABLED'">
                  <Tooltip
                    v-if="isSelfUser(row)"
                    title="安全保护：不能封禁当前登录管理员自身"
                  >
                    <Button danger disabled size="small" type="link">
                      封禁
                    </Button>
                  </Tooltip>
                  <Popconfirm
                    v-else
                    cancel-text="取消"
                    ok-text="确认封禁"
                    title="封禁后该账号将无法登录 IAM 且其当前 IAM 会话将被撤销，确认封禁？"
                    @confirm="handleDisableUser(row)"
                  >
                    <Button
                      danger
                      :loading="isRowBusy(row)"
                      size="small"
                      type="link"
                    >
                      封禁
                    </Button>
                  </Popconfirm>
                </template>

                <Popconfirm
                  v-else
                  cancel-text="取消"
                  ok-text="确认解禁"
                  title="确认解除封禁并恢复该账号的正常登录状态？"
                  @confirm="handleEnableUser(row)"
                >
                  <Button :loading="isRowBusy(row)" size="small" type="link">
                    解禁
                  </Button>
                </Popconfirm>

                <Tooltip
                  v-if="isSelfUser(row)"
                  title="安全保护：不能软删除当前登录管理员自身"
                >
                  <Button danger disabled size="small" type="link">
                    软删除
                  </Button>
                </Tooltip>
                <Popconfirm
                  v-else
                  cancel-text="取消"
                  ok-text="移入回收站"
                  title="软删除后账号将移入回收站无限期保留，原邮箱继续占用，确认软删除？"
                  @confirm="handleSoftDeleteUser(row)"
                >
                  <Button
                    danger
                    :loading="isRowBusy(row)"
                    size="small"
                    type="link"
                  >
                    软删除
                  </Button>
                </Popconfirm>
              </template>
            </Space>

            <Space v-else class="user-row-actions justify-end" :size="4" wrap>
              <Button size="small" type="link" @click="openDetailDrawer(row)">
                查看详情
              </Button>

              <Tooltip
                v-if="!canManageUsers"
                title="需要 manage_users 权限才能恢复账号"
              >
                <Button disabled size="small" type="link">恢复账号</Button>
              </Tooltip>
              <Popconfirm
                v-else
                cancel-text="取消"
                ok-text="确认恢复"
                title="恢复后账号将回到原生命周期状态（若需登录可由管理员重设密码），确认恢复？"
                @confirm="handleUndeleteUser(row)"
              >
                <Button :loading="isRowBusy(row)" size="small" type="link">
                  恢复账号
                </Button>
              </Popconfirm>

              <Tooltip
                v-if="isSelfUser(row)"
                title="安全保护：不能永久删除当前登录管理员自身"
              >
                <Button danger disabled size="small" type="link">
                  永久删除
                </Button>
              </Tooltip>
              <Tooltip
                v-else-if="!canExpungeUsers"
                title="需要 expunge_users 权限才能执行显式永久删除"
              >
                <Button danger disabled size="small" type="link">
                  永久删除
                </Button>
              </Tooltip>
              <Button
                v-else
                danger
                :disabled="isRowBusy(row)"
                size="small"
                type="link"
                @click="openExpungeModal(row)"
              >
                永久删除
              </Button>
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

    <DetailDrawer @updated="reloadKeepingCurrentCursor" />
    <CreateUserModal @created="handleUserCreated" />
    <SetPasswordModal />
    <ExpungeUserModal @expunged="reloadKeepingCurrentCursor" />

    <AntModal
      v-model:open="degradedModalOpen"
      cancel-text="稍后在列表中补发"
      :confirm-loading="resendingDegradedMail"
      ok-text="立即补发验证邮件"
      title="账号已创建，但验证邮件发送失败"
      @ok="
        degradedCreatedUser &&
        handleResendVerificationEmail(degradedCreatedUser, true)
      "
    >
      <div class="space-y-4 py-2">
        <Alert
          description="该 IAM 账号已在数据库中创建完成（状态为待邮箱验证），但邮件服务暂时不可用导致验证信未能发出。请勿再次点击“创建用户”重复提交同一邮箱！"
          message="请勿重复创建账号"
          show-icon
          type="warning"
        />
        <Descriptions
          v-if="degradedCreatedUser"
          bordered
          :column="1"
          size="small"
        >
          <Descriptions.Item label="已创建邮箱">
            <span class="font-semibold">{{
              degradedCreatedUser.email || '—'
            }}</span>
          </Descriptions.Item>
          <Descriptions.Item label="用户 ID">
            <span class="font-mono text-xs">{{
              degradedCreatedUser.userId || '—'
            }}</span>
          </Descriptions.Item>
          <Descriptions.Item label="资源名称">
            <span class="font-mono text-xs">{{
              degradedCreatedUser.name || '—'
            }}</span>
          </Descriptions.Item>
        </Descriptions>
      </div>
    </AntModal>
  </Page>
</template>

<style scoped>
:deep(.vxe-body--column > .vxe-cell > .vxe-cell--wrapper) {
  min-width: 0;
  max-width: 100%;
}

:deep(.user-row-actions .ant-btn-link) {
  padding-inline: 4px;
}
</style>
