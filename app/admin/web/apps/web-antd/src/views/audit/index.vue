<script lang="ts" setup>
import type { Event } from '@plateau/api/audit/event/v1';

import type {
  AuditCategoryTab,
  AuditCursorState,
  AuditSearchFormValues,
} from './schemas';

import {
  computed,
  nextTick,
  onMounted,
  onUnmounted,
  ref,
  toRaw,
  watch,
} from 'vue';
import { useRoute, useRouter } from 'vue-router';

import { Page, useVbenDrawer } from '@vben/common-ui';
import { LOGIN_PATH } from '@vben/constants';

import {
  Alert,
  Button,
  message,
  Segmented,
  Select,
  Space,
  Tag,
  Tooltip,
} from 'ant-design-vue';
import dayjs from 'dayjs';

import { useVbenVxeGrid } from '#/adapter/vxe-table';
import { createAdminApi, resolveAdminApiError } from '#/api';
import { useAuthStore } from '#/store';

import AuditDetailDrawerComp from './components/audit-detail-drawer.vue';
import {
  applyAuditCursorPageResult,
  buildAuditListRequest,
  buildAuditQuerySignature,
  buildServiceAutoCompleteOptions,
  createDefaultAuditTimeRange,
  createInitialAuditCursorState,
  extractAuditInitialQuery,
  formatServiceDisplay,
  formatShortOperation,
  getActorBadge,
  getAuditGridColumns,
  getAuditSearchFormSchema,
  getCategoryBadge,
  hasPreciseAuditUrlQuery,
  isInvertedTimeRange,
  mergeAuditServices,
  resolveAuditEventResourceName,
  shouldExpandAuditAdvancedFilters,
  stepAuditCursorNext,
  stepAuditCursorPrev,
} from './schemas';

defineOptions({ name: 'AdminAuditLogsPage' });

const route = useRoute();
const router = useRouter();
const authStore = useAuthStore();

const initialQuery = extractAuditInitialQuery(
  route.query as Record<string, unknown>,
);
const initialHasPreciseQuery = hasPreciseAuditUrlQuery(
  route.query as Record<string, unknown>,
);
const initialDefaultRange = initialHasPreciseQuery
  ? null
  : createDefaultAuditTimeRange();

const categoryTab = ref<AuditCategoryTab>('all');
const cursorState = ref<AuditCursorState>(createInitialAuditCursorState(20));
const listLoading = ref(false);
const bannerError = ref('');
const lastQuerySignature = ref('');
const preserveCursorOnce = ref(false);

const backendServices = ref<string[]>([]);
const pageServices = ref<string[]>([]);

const mergedRawServices = computed(() =>
  mergeAuditServices(backendServices.value, pageServices.value),
);
const serviceOptions = computed(() =>
  buildServiceAutoCompleteOptions(mergedRawServices.value),
);

let listAbortController: AbortController | null = null;
let servicesAbortController: AbortController | null = null;

function cancelListRequest() {
  listAbortController?.abort();
  listAbortController = null;
}

function cancelServicesRequest() {
  servicesAbortController?.abort();
  servicesAbortController = null;
}

onUnmounted(() => {
  cancelListRequest();
  cancelServicesRequest();
});

async function handleUnauthenticatedRedirect(msg: string) {
  message.warning(msg);
  authStore.markUnauthenticated();
  await router.replace({
    path: LOGIN_PATH,
    query: { redirect: router.currentRoute.value.fullPath },
  });
}

const [DetailDrawer, detailDrawerApi] = useVbenDrawer({
  connectedComponent: AuditDetailDrawerComp,
});
let hasSyncedInitialSubmission = false;
let suppressRouteQueryWatch = false;

async function syncProgrammaticFormValues(nextValues: AuditSearchFormValues) {
  await gridApi.formApi.setValues(nextValues);
  gridApi.formApi.setLatestSubmissionValues(toRaw({ ...nextValues }));
  hasSyncedInitialSubmission = true;
}

const [Grid, gridApi] = useVbenVxeGrid<Event, AuditSearchFormValues>({
  formOptions: {
    collapsed: !shouldExpandAuditAdvancedFilters(
      route.query as Record<string, unknown>,
    ),
    handleReset: async () => {
      const resetValues: AuditSearchFormValues = {
        actorId: '',
        actorType: '',
        operation: '',
        outcome: '',
        resourceName: '',
        service: '',
        timeRange: createDefaultAuditTimeRange(),
        traceId: '',
      };
      await syncProgrammaticFormValues(resetValues);
      if (Object.keys(route.query).length > 0) {
        suppressRouteQueryWatch = true;
        try {
          await router.replace({ path: '/audit', query: {} });
        } finally {
          suppressRouteQueryWatch = false;
        }
      }
      cursorState.value = createInitialAuditCursorState(
        cursorState.value.pageSize,
      );
      lastQuerySignature.value = '';
      await gridApi.query();
    },
    schema: getAuditSearchFormSchema({
      defaultTimeRange: initialDefaultRange,
      initialValues: initialQuery,
      serviceOptions: serviceOptions.value,
    }),
    showCollapseButton: true,
    submitOnChange: false,
    submitOnEnter: true,
    wrapperClass: 'grid-cols-1 md:grid-cols-2 lg:grid-cols-4',
  },
  gridOptions: {
    columns: getAuditGridColumns(),
    height: 'auto',
    keepSource: false,
    pagerConfig: {
      enabled: false,
    },
    proxyConfig: {
      ajax: {
        query: async (
          _params: unknown,
          rawFormValues?: AuditSearchFormValues,
        ) => {
          const formValues: AuditSearchFormValues = rawFormValues ?? {};
          if (!hasSyncedInitialSubmission) {
            gridApi.formApi.setLatestSubmissionValues(toRaw({ ...formValues }));
            hasSyncedInitialSubmission = true;
          }
          if (isInvertedTimeRange(formValues.timeRange)) {
            message.warning('开始时间不能晚于结束时间');
            return { items: [], total: 0 };
          }

          const nextSignature = buildAuditQuerySignature(
            categoryTab.value,
            formValues,
            cursorState.value.pageSize,
            mergedRawServices.value,
          );
          if (
            !preserveCursorOnce.value &&
            nextSignature !== lastQuerySignature.value
          ) {
            cursorState.value = createInitialAuditCursorState(
              cursorState.value.pageSize,
            );
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
      keyField: 'eventId',
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

watch(serviceOptions, (nextOptions) => {
  gridApi.formApi.updateSchema([
    {
      componentProps: {
        options: nextOptions,
      },
      fieldName: 'service',
    },
  ]);
});

async function fetchObservedServices() {
  cancelServicesRequest();
  const controller = new AbortController();
  servicesAbortController = controller;
  try {
    const resp = await createAdminApi(controller.signal).audit.ListServices({});
    if (servicesAbortController !== controller) {
      return;
    }
    backendServices.value = resp.services ?? [];
  } catch (error) {
    const resolved = resolveAdminApiError(error);
    if (!resolved.silent && !resolved.preserveSession) {
      await handleUnauthenticatedRedirect(resolved.message);
    }
  } finally {
    if (servicesAbortController === controller) {
      servicesAbortController = null;
    }
  }
}

async function executeListQuery(
  formValues: AuditSearchFormValues,
  retriedInvalidToken = false,
): Promise<{ items: Event[]; total: number }> {
  cancelListRequest();
  const controller = new AbortController();
  listAbortController = controller;
  listLoading.value = true;

  try {
    const activeToken =
      cursorState.value.cursorStack[cursorState.value.currentPageIndex] ?? '';
    const request = buildAuditListRequest(
      categoryTab.value,
      formValues,
      cursorState.value.pageSize,
      activeToken,
      mergedRawServices.value,
    );
    const response = await createAdminApi(controller.signal).audit.ListEvents(
      request,
    );
    bannerError.value = '';

    const events = response.events ?? [];
    pageServices.value = events
      .map((ev) => ev.service ?? '')
      .filter((s) => s.trim() !== '');

    const applied = applyAuditCursorPageResult(
      cursorState.value,
      events.length,
      response.nextPageToken,
    );
    cursorState.value = applied.state;

    if (applied.fallbackToPreviousPage) {
      return await executeListQuery(formValues, retriedInvalidToken);
    }

    return {
      items: events,
      total: events.length,
    };
  } catch (error) {
    const resolved = resolveAdminApiError(error);
    if (!resolved.silent) {
      if (!resolved.preserveSession) {
        await handleUnauthenticatedRedirect(resolved.message);
      } else if (
        resolved.category === 'invalid_argument' &&
        resolved.shouldRefreshRecord &&
        !retriedInvalidToken
      ) {
        message.info(resolved.message);
        cursorState.value = createInitialAuditCursorState(
          cursorState.value.pageSize,
        );
        return await executeListQuery(formValues, true);
      } else if (resolved.category === 'forbidden') {
        bannerError.value = resolved.message;
        await authStore.ensureSession(true);
      } else if (
        resolved.category === 'unavailable' ||
        resolved.category === 'network' ||
        resolved.category === 'timeout'
      ) {
        bannerError.value = resolved.message;
      } else {
        message.error(resolved.message);
      }
    }
    cursorState.value = {
      ...cursorState.value,
      currentPageCount: 0,
      nextPageToken: '',
    };
    return {
      items: [],
      total: 0,
    };
  } finally {
    if (listAbortController === controller) {
      listLoading.value = false;
    }
  }
}

async function reloadKeepingCurrentCursor() {
  preserveCursorOnce.value = true;
  void fetchObservedServices();
  await gridApi.query();
}

async function handleSwitchCategory(nextValue: number | string) {
  const nextTab = (nextValue as AuditCategoryTab) || 'all';
  if (nextTab === categoryTab.value) {
    return;
  }
  cancelListRequest();
  categoryTab.value = nextTab;
  cursorState.value = createInitialAuditCursorState(cursorState.value.pageSize);
  lastQuerySignature.value = '';
  await gridApi.query();
}

async function handleFirstPage() {
  if (cursorState.value.currentPageIndex === 0) {
    return;
  }
  cursorState.value = createInitialAuditCursorState(cursorState.value.pageSize);
  await reloadKeepingCurrentCursor();
}

async function handlePrevPage() {
  const prev = stepAuditCursorPrev(cursorState.value);
  if (!prev) {
    return;
  }
  cursorState.value = prev;
  await reloadKeepingCurrentCursor();
}

async function handleNextPage() {
  const next = stepAuditCursorNext(cursorState.value);
  if (!next) {
    return;
  }
  cursorState.value = next;
  await reloadKeepingCurrentCursor();
}

async function handlePageSizeChange(nextSize: unknown) {
  const parsed = Number(nextSize);
  if (!Number.isFinite(parsed) || parsed <= 0) {
    return;
  }
  cursorState.value = createInitialAuditCursorState(parsed);
  lastQuerySignature.value = '';
  await gridApi.query();
}

function openEventDetail(row: Event) {
  detailDrawerApi
    .setData({
      initialEvent: row,
      resourceName: resolveAuditEventResourceName(row),
    })
    .open();
}

async function handleFilterByTraceId(traceId: string) {
  categoryTab.value = 'all';
  gridApi.setState({
    formOptions: {
      collapsed: false,
    },
  });
  await nextTick();
  await syncProgrammaticFormValues({
    actorId: '',
    actorType: '',
    operation: '',
    outcome: '',
    resourceName: '',
    service: '',
    timeRange: null,
    traceId,
  });
  cursorState.value = createInitialAuditCursorState(cursorState.value.pageSize);
  lastQuerySignature.value = '';
  await gridApi.query();
}

function formatTableTime(iso?: string): string {
  if (!iso) {
    return '—';
  }
  const d = dayjs(iso);
  return d.isValid() ? d.format('YYYY-MM-DD HH:mm:ss') : iso;
}

function formatTooltipTime(iso?: string): string {
  if (!iso) {
    return '';
  }
  const d = dayjs(iso);
  return d.isValid()
    ? `本地：${d.format('YYYY-MM-DD HH:mm:ss.SSS')} | UTC：${iso}`
    : iso;
}

watch(
  () => [
    route.query.actorId,
    route.query.actorType,
    route.query.operation,
    route.query.resourceName,
    route.query.service,
    route.query.traceId,
  ],
  async () => {
    if (suppressRouteQueryWatch) {
      return;
    }
    const queryMap = route.query as Record<string, unknown>;
    const parsed = extractAuditInitialQuery(queryMap);
    const hasPrecise = hasPreciseAuditUrlQuery(queryMap);
    if (shouldExpandAuditAdvancedFilters(queryMap)) {
      gridApi.setState({
        formOptions: {
          collapsed: false,
        },
      });
    }
    await nextTick();
    await syncProgrammaticFormValues({
      actorId: parsed.actorId ?? '',
      actorType: parsed.actorType ?? '',
      operation: parsed.operation ?? '',
      outcome: '',
      resourceName: parsed.resourceName ?? '',
      service: parsed.service ?? '',
      timeRange: hasPrecise ? null : createDefaultAuditTimeRange(),
      traceId: parsed.traceId ?? '',
    });
    categoryTab.value = 'all';
    cursorState.value = createInitialAuditCursorState(
      cursorState.value.pageSize,
    );
    lastQuerySignature.value = '';
    await gridApi.query();
  },
);

watch(
  () => route.query.event,
  async (rawEvent) => {
    const eventName = Array.isArray(rawEvent)
      ? rawEvent[0]?.trim()
      : rawEvent?.trim();
    if (!eventName) {
      return;
    }
    const nextQuery = { ...route.query };
    delete nextQuery.event;
    await router.replace({ path: '/audit', query: nextQuery });
    await nextTick();
    detailDrawerApi.setData({ resourceName: eventName }).open();
  },
  { flush: 'post', immediate: true },
);

onMounted(() => {
  void fetchObservedServices();
});
</script>

<template>
  <Page auto-content-height title="全平台审计日志">
    <div class="flex h-full flex-col gap-3">
      <Alert
        v-if="bannerError"
        :description="bannerError"
        message="审计查询服务暂时不可用"
        show-icon
        type="error"
      >
        <template #action>
          <Button
            :loading="listLoading"
            size="small"
            type="primary"
            @click="reloadKeepingCurrentCursor"
          >
            立即重试
          </Button>
        </template>
      </Alert>

      <div class="min-h-0 flex-1">
        <Grid>
          <template #toolbar-actions>
            <Space wrap>
              <Segmented
                :options="[
                  { label: '全部', value: 'all' },
                  { label: '认证与会话', value: 'AUDIT_CATEGORY_AUTHN' },
                  { label: '资源变更', value: 'AUDIT_CATEGORY_RESOURCE' },
                  { label: '授权拒绝', value: 'AUDIT_CATEGORY_AUTHZ' },
                ]"
                :value="categoryTab"
                @change="handleSwitchCategory"
              />
              <Button
                :loading="listLoading"
                @click="reloadKeepingCurrentCursor"
              >
                刷新列表
              </Button>
            </Space>
          </template>

          <template #occurredTime="{ row }">
            <Tooltip :title="formatTooltipTime(row.occurredTime)">
              <span class="font-mono text-xs">
                {{ formatTableTime(row.occurredTime) }}
              </span>
            </Tooltip>
          </template>

          <template #category="{ row }">
            <Tag :color="getCategoryBadge(row.category).color">
              {{ getCategoryBadge(row.category).text }}
            </Tag>
          </template>

          <template #serviceOperation="{ row }">
            <Tooltip
              :title="`${row.service || '—'} · ${row.operation || row.eventType || '—'}`"
            >
              <div class="flex min-w-0 flex-col gap-0.5 py-0.5 text-left">
                <div class="flex items-center gap-1.5 leading-5">
                  <span
                    class="text-foreground truncate font-mono text-xs font-medium"
                  >
                    {{ formatShortOperation(row.operation || row.eventType) }}
                  </span>
                </div>
                <div
                  class="text-muted-foreground truncate font-mono text-xs leading-4"
                >
                  {{ formatServiceDisplay(row.service) }}
                </div>
              </div>
            </Tooltip>
          </template>

          <template #actor="{ row }">
            <div class="flex items-center gap-1.5">
              <Tag
                class="m-0 shrink-0"
                :color="getActorBadge(row.actorType).color"
              >
                {{ getActorBadge(row.actorType).text }}
              </Tag>
              <span
                v-if="row.actorType === 'anonymous'"
                class="text-muted-foreground text-xs"
              >
                匿名请求
              </span>
              <Tooltip v-else :title="row.actorId || undefined">
                <span class="truncate font-mono text-xs">
                  {{ row.actorId || '—' }}
                </span>
              </Tooltip>
            </div>
          </template>

          <template #resourceName="{ row }">
            <span v-if="!row.resourceName" class="text-muted-foreground">
              —
            </span>
            <Tooltip
              v-else
              :title="
                row.resourceType
                  ? `${row.resourceType} · ${row.resourceName}`
                  : row.resourceName
              "
            >
              <span class="font-mono text-xs">{{ row.resourceName }}</span>
            </Tooltip>
          </template>

          <template #outcome="{ row }">
            <Space :size="4" wrap>
              <Tag class="m-0" :color="row.success ? 'success' : 'error'">
                {{ row.success ? '成功' : '失败' }}
              </Tag>
              <Tooltip
                v-if="!row.success && row.errorCode"
                :title="row.errorMessage || row.errorCode"
              >
                <span
                  class="text-muted-foreground max-w-[130px] truncate font-mono text-xs"
                >
                  {{ row.errorCode }}
                </span>
              </Tooltip>
            </Space>
          </template>

          <template #clientIp="{ row }">
            <span class="font-mono text-xs">{{ row.clientIp || '—' }}</span>
          </template>

          <template #actions="{ row }">
            <Button size="small" type="link" @click="openEventDetail(row)">
              详情
            </Button>
          </template>
        </Grid>
      </div>

      <div
        class="bg-card border-border flex flex-wrap items-center justify-between gap-3 rounded-md border px-4 py-2.5 text-sm"
      >
        <div class="text-muted-foreground">
          当前第
          <span class="text-foreground font-medium">{{
            cursorState.currentPageIndex + 1
          }}</span>
          页 · 本页
          <span class="text-foreground font-medium">{{
            cursorState.currentPageCount
          }}</span>
          条记录
        </div>

        <Space wrap>
          <Select
            :disabled="listLoading"
            :options="[
              { label: '20 条 / 页', value: 20 },
              { label: '50 条 / 页', value: 50 },
              { label: '100 条 / 页', value: 100 },
            ]"
            size="small"
            style="width: 116px"
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

    <DetailDrawer @filter-by-trace-id="handleFilterByTraceId" />
  </Page>
</template>
