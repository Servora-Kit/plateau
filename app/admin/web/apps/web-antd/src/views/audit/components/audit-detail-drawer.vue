<script lang="ts" setup>
import type { Event } from '@plateau/api/plateau/audit/event/v1';

import { computed, onUnmounted, ref } from 'vue';
import { useRouter } from 'vue-router';

import { useVbenDrawer } from '@vben/common-ui';
import { LOGIN_PATH } from '@vben/constants';
import { formatDateTime } from '@vben/utils';

import {
  Alert,
  Button,
  Descriptions,
  Divider,
  message,
  Skeleton,
  Space,
  Spin,
  Tag,
  Timeline,
  Tooltip,
} from 'ant-design-vue';
import dayjs from 'dayjs';

import { createAdminApi, resolveAdminApiError } from '#/api';
import { useAuthStore } from '#/store';

import {
  extractLinkedUserIdFromResource,
  formatDetailJson,
  formatServiceDisplay,
  formatShortOperation,
  getActorBadge,
  getCategoryBadge,
  resolveAuditEventResourceName,
  TRACE_TIMELINE_PAGE_SIZE,
} from '../schemas';

export interface AuditDetailDrawerPayload {
  initialEvent?: Event;
  resourceName: string;
}

const emit = defineEmits<{
  filterByTraceId: [traceId: string];
}>();

const router = useRouter();
const authStore = useAuthStore();

const detailLoading = ref(false);
const detailError = ref('');
const activeResourceName = ref('');
const activeEvent = ref<Event | null>(null);

const timelineLoading = ref(false);
const timelineError = ref('');
const timelineEvents = ref<Event[]>([]);
const timelineTruncated = ref(false);
const loadedTraceId = ref('');

let detailAbortController: AbortController | null = null;
let timelineAbortController: AbortController | null = null;

function cancelDetailRequest() {
  detailAbortController?.abort();
  detailAbortController = null;
}

function cancelTimelineRequest() {
  timelineAbortController?.abort();
  timelineAbortController = null;
}

function cancelAllRequests() {
  cancelDetailRequest();
  cancelTimelineRequest();
}

onUnmounted(() => {
  cancelAllRequests();
});

const categoryBadge = computed(() =>
  getCategoryBadge(activeEvent.value?.category),
);
const actorBadge = computed(() => getActorBadge(activeEvent.value?.actorType));
const detailPayload = computed(() =>
  formatDetailJson(activeEvent.value?.detailJson),
);

const linkedResourceUserId = computed(() =>
  extractLinkedUserIdFromResource(activeEvent.value?.resourceName),
);

const linkedActorUserId = computed(() => {
  if (
    activeEvent.value?.actorType?.trim().toLowerCase() === 'human' &&
    activeEvent.value.actorId?.trim()
  ) {
    return activeEvent.value.actorId.trim();
  }
  return null;
});

function formatPreciseTime(iso?: string): string {
  if (!iso) {
    return '—';
  }
  const d = dayjs(iso);
  if (!d.isValid()) {
    return formatDateTime(iso);
  }
  return d.format('YYYY-MM-DD HH:mm:ss.SSS');
}

function formatTimelineClock(iso?: string): string {
  if (!iso) {
    return '--:--:--';
  }
  const d = dayjs(iso);
  return d.isValid() ? d.format('HH:mm:ss.SSS') : iso;
}

async function handleUnauthenticatedRedirect(msg: string) {
  message.warning(msg);
  authStore.markUnauthenticated();
  await router.replace({
    path: LOGIN_PATH,
    query: { redirect: router.currentRoute.value.fullPath },
  });
}

async function copyText(text: string, label: string) {
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

async function fetchTraceTimeline(traceId: string, force = false) {
  const cleanTraceId = traceId.trim().toLowerCase();
  if (!cleanTraceId) {
    timelineEvents.value = [];
    timelineTruncated.value = false;
    timelineError.value = '';
    loadedTraceId.value = '';
    return;
  }
  if (!force && loadedTraceId.value === cleanTraceId) {
    return;
  }

  cancelTimelineRequest();
  const controller = new AbortController();
  timelineAbortController = controller;
  timelineLoading.value = true;
  timelineError.value = '';

  try {
    const response = await createAdminApi(controller.signal).audit.ListEvents({
      pageSize: TRACE_TIMELINE_PAGE_SIZE,
      traceId: cleanTraceId,
    });
    if (timelineAbortController !== controller) {
      return;
    }
    const items = [...(response.events ?? [])].toSorted((a, b) => {
      const ta = a.occurredTime ? new Date(a.occurredTime).getTime() : 0;
      const tb = b.occurredTime ? new Date(b.occurredTime).getTime() : 0;
      if (ta !== tb) {
        return ta - tb;
      }
      return (a.eventId ?? '').localeCompare(b.eventId ?? '');
    });
    timelineEvents.value = items;
    timelineTruncated.value = Boolean(response.nextPageToken?.trim());
    loadedTraceId.value = cleanTraceId;
  } catch (error) {
    const resolved = resolveAdminApiError(error);
    if (resolved.silent) {
      return;
    }
    if (!resolved.preserveSession) {
      await handleUnauthenticatedRedirect(resolved.message);
      return;
    }
    if (resolved.category === 'forbidden') {
      await authStore.ensureSession(true);
    }
    timelineError.value = resolved.message;
  } finally {
    if (timelineAbortController === controller) {
      timelineLoading.value = false;
    }
  }
}

async function fetchEventByName(resourceName: string, forceTimeline = false) {
  const cleanName = resourceName.trim();
  if (!cleanName) {
    detailError.value = '缺少有效的审计事件资源名称';
    return;
  }

  cancelDetailRequest();
  const controller = new AbortController();
  detailAbortController = controller;
  detailLoading.value = true;
  detailError.value = '';
  activeResourceName.value = cleanName;

  try {
    const ev = await createAdminApi(controller.signal).audit.GetEvent({
      name: cleanName,
    });
    if (detailAbortController !== controller) {
      return;
    }
    activeEvent.value = ev;
    activeResourceName.value = resolveAuditEventResourceName(ev) || cleanName;
    if (ev.traceId?.trim()) {
      void fetchTraceTimeline(ev.traceId, forceTimeline);
    } else {
      timelineEvents.value = [];
      timelineTruncated.value = false;
      loadedTraceId.value = '';
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
    if (resolved.category === 'forbidden') {
      await authStore.ensureSession(true);
    }
    detailError.value = resolved.message;
  } finally {
    if (detailAbortController === controller) {
      detailLoading.value = false;
    }
  }
}

function applyInMemoryEvent(ev: Event) {
  cancelDetailRequest();
  detailLoading.value = false;
  detailError.value = '';
  activeEvent.value = ev;
  activeResourceName.value = resolveAuditEventResourceName(ev);
  if (ev.traceId?.trim()) {
    void fetchTraceTimeline(ev.traceId, false);
  } else {
    cancelTimelineRequest();
    timelineEvents.value = [];
    timelineTruncated.value = false;
    timelineError.value = '';
    loadedTraceId.value = '';
  }
}

function handleSelectTimelineNode(ev: Event) {
  if (resolveAuditEventResourceName(ev) === activeResourceName.value) {
    return;
  }
  applyInMemoryEvent(ev);
}

async function handleManualRefresh() {
  if (!activeResourceName.value) {
    return;
  }
  await fetchEventByName(activeResourceName.value, true);
}

const [Drawer, drawerApi] = useVbenDrawer<AuditDetailDrawerPayload>({
  footer: false,
  onOpenChange(isOpen) {
    if (!isOpen) {
      cancelAllRequests();
      detailError.value = '';
      timelineError.value = '';
      loadedTraceId.value = '';
      return;
    }
    const payload = drawerApi.getData();
    if (!payload?.resourceName && !payload?.initialEvent) {
      return;
    }
    if (payload.initialEvent) {
      applyInMemoryEvent(payload.initialEvent);
      return;
    }
    void fetchEventByName(payload.resourceName, true);
  },
  title: '审计事件详情',
});

async function jumpToUserDetail(userId: string) {
  drawerApi.close();
  await router.push({
    path: '/users',
    query: { userId },
  });
}

function handleFilterTraceInList() {
  const traceId = activeEvent.value?.traceId?.trim();
  if (!traceId) {
    return;
  }
  drawerApi.close();
  emit('filterByTraceId', traceId);
}
</script>

<template>
  <Drawer class="w-full max-w-[720px]">
    <template #extra>
      <Button
        :disabled="!activeResourceName || detailLoading"
        :loading="detailLoading"
        size="small"
        @click="handleManualRefresh"
      >
        刷新详情
      </Button>
    </template>

    <Skeleton
      v-if="detailLoading && !activeEvent"
      active
      :paragraph="{ rows: 9 }"
    />

    <Alert
      v-else-if="detailError"
      :message="detailError"
      show-icon
      type="error"
    >
      <template #action>
        <Button size="small" @click="handleManualRefresh">重试</Button>
      </template>
    </Alert>

    <div v-else-if="activeEvent" class="space-y-5">
      <div
        class="bg-muted/40 border-border flex flex-wrap items-center justify-between gap-2 rounded-md border px-3.5 py-2.5"
      >
        <div class="min-w-0 flex-1 space-y-1">
          <div class="flex flex-wrap items-center gap-2">
            <Tag :color="activeEvent.success ? 'success' : 'error'">
              {{ activeEvent.success ? '执行成功' : '失败 / 拒绝' }}
            </Tag>
            <Tag :color="categoryBadge.color">{{ categoryBadge.text }}</Tag>
            <Tag color="default">
              {{ formatServiceDisplay(activeEvent.service) }}
            </Tag>
          </div>
          <div
            class="text-foreground break-all font-mono text-sm font-semibold"
          >
            {{ activeEvent.operation || activeEvent.eventType || '—' }}
          </div>
        </div>
      </div>

      <Descriptions bordered :column="1" size="small" title="核心问责属性">
        <Descriptions.Item label="事件 ID">
          <div class="flex flex-wrap items-center justify-between gap-2">
            <span class="break-all font-mono text-xs">
              {{ activeEvent.eventId || '—' }}
            </span>
            <Button
              v-if="activeEvent.eventId"
              size="small"
              type="link"
              @click="copyText(activeEvent.eventId, '事件 ID')"
            >
              复制
            </Button>
          </div>
        </Descriptions.Item>
        <Descriptions.Item label="发生时间">
          <Space size="small" wrap>
            <span>{{ formatPreciseTime(activeEvent.occurredTime) }}</span>
            <span
              v-if="activeEvent.occurredTime"
              class="text-muted-foreground font-mono text-xs"
            >
              ({{ activeEvent.occurredTime }})
            </span>
          </Space>
        </Descriptions.Item>
        <Descriptions.Item label="事件类型">
          <span class="font-mono text-xs">{{
            activeEvent.eventType || '—'
          }}</span>
        </Descriptions.Item>
        <Descriptions.Item label="生产服务">
          <Space size="small" wrap>
            <span>{{ formatServiceDisplay(activeEvent.service) }}</span>
            <span
              v-if="activeEvent.service"
              class="text-muted-foreground font-mono text-xs"
            >
              ({{ activeEvent.service }})
            </span>
          </Space>
        </Descriptions.Item>
        <Descriptions.Item label="操作主体">
          <div class="flex flex-wrap items-center justify-between gap-2">
            <Space size="small" wrap>
              <Tag :color="actorBadge.color">{{ actorBadge.text }}</Tag>
              <span
                v-if="activeEvent.actorType === 'anonymous'"
                class="text-muted-foreground text-xs"
              >
                匿名请求（未携带已认证主体）
              </span>
              <span v-else class="break-all font-mono text-xs">
                {{ activeEvent.actorId || '—' }}
              </span>
            </Space>
            <Button
              v-if="linkedActorUserId"
              size="small"
              type="link"
              @click="jumpToUserDetail(linkedActorUserId)"
            >
              查看操作者账号
            </Button>
          </div>
        </Descriptions.Item>
        <Descriptions.Item label="目标资源">
          <div class="flex flex-wrap items-center justify-between gap-2">
            <Space direction="vertical" :size="2">
              <span class="break-all font-mono text-xs">
                {{ activeEvent.resourceName || '—' }}
              </span>
              <span
                v-if="activeEvent.resourceType"
                class="text-muted-foreground text-xs"
              >
                类型：{{ activeEvent.resourceType }}
              </span>
            </Space>
            <Button
              v-if="linkedResourceUserId"
              size="small"
              type="link"
              @click="jumpToUserDetail(linkedResourceUserId)"
            >
              查看目标用户
            </Button>
          </div>
        </Descriptions.Item>
        <Descriptions.Item v-if="!activeEvent.success" label="错误详情">
          <Space direction="vertical" :size="2">
            <Tag color="error">{{ activeEvent.errorCode || 'ERROR' }}</Tag>
            <span
              v-if="activeEvent.errorMessage"
              class="text-muted-foreground break-all text-xs"
            >
              {{ activeEvent.errorMessage }}
            </span>
          </Space>
        </Descriptions.Item>
        <Descriptions.Item label="来源 IP / UA">
          <Space direction="vertical" :size="2">
            <span class="font-mono text-xs">
              IP：{{ activeEvent.clientIp || '—' }}
            </span>
            <span
              v-if="activeEvent.userAgent"
              class="text-muted-foreground break-all text-xs"
            >
              UA：{{ activeEvent.userAgent }}
            </span>
          </Space>
        </Descriptions.Item>
        <Descriptions.Item label="链路与请求标识">
          <div class="space-y-1">
            <div class="flex flex-wrap items-center justify-between gap-2">
              <span class="break-all font-mono text-xs">
                TraceID：{{ activeEvent.traceId || '—' }}
              </span>
              <Space v-if="activeEvent.traceId" :size="4">
                <Button
                  size="small"
                  type="link"
                  @click="copyText(activeEvent.traceId, 'TraceID')"
                >
                  复制
                </Button>
                <Button
                  size="small"
                  type="link"
                  @click="handleFilterTraceInList"
                >
                  在列表中筛选
                </Button>
              </Space>
            </div>
            <div
              v-if="activeEvent.requestId"
              class="text-muted-foreground break-all font-mono text-xs"
            >
              RequestID：{{ activeEvent.requestId }}
            </div>
          </div>
        </Descriptions.Item>
      </Descriptions>

      <Divider class="!my-3" />

      <div class="space-y-3">
        <div class="flex flex-wrap items-center justify-between gap-2">
          <h4 class="text-foreground text-sm font-semibold">
            同链路关联审计记录（TraceID 串联）
          </h4>
          <span
            v-if="timelineEvents.length > 0"
            class="text-muted-foreground text-xs"
          >
            共加载 {{ timelineEvents.length }} 跳记录（按发生时间正序）
          </span>
        </div>

        <div
          v-if="!activeEvent.traceId"
          class="text-muted-foreground rounded-md border border-dashed px-3 py-2.5 text-xs"
        >
          当前事件未携带 TraceID，无上下游关联记录。
        </div>

        <Spin
          v-else-if="timelineLoading"
          size="small"
          tip="正在加载同链路审计记录..."
        />

        <Alert
          v-else-if="timelineError"
          :message="timelineError"
          show-icon
          type="warning"
        >
          <template #action>
            <Button
              size="small"
              @click="fetchTraceTimeline(activeEvent.traceId, true)"
            >
              重试
            </Button>
          </template>
        </Alert>

        <template v-else>
          <Alert
            v-if="timelineTruncated"
            message="该调用链关联审计记录超过 200 条，抽屉仅展示最近 200 条。"
            show-icon
            type="info"
          >
            <template #action>
              <Button size="small" type="link" @click="handleFilterTraceInList">
                在列表中按此 TraceID 查看全部
              </Button>
            </template>
          </Alert>

          <Timeline class="mt-2 pt-1">
            <Timeline.Item
              v-for="node in timelineEvents"
              :key="node.eventId || node.name"
              :color="
                resolveAuditEventResourceName(node) === activeResourceName
                  ? 'blue'
                  : node.success
                    ? 'green'
                    : 'red'
              "
            >
              <div
                class="rounded-md border px-3 py-2 transition-colors"
                :class="
                  resolveAuditEventResourceName(node) === activeResourceName
                    ? 'border-primary/60 bg-primary/5'
                    : 'border-border hover:bg-muted/40 cursor-pointer'
                "
                @click="handleSelectTimelineNode(node)"
              >
                <div class="flex flex-wrap items-center justify-between gap-2">
                  <Space size="small" wrap>
                    <span class="text-muted-foreground font-mono text-xs">
                      {{ formatTimelineClock(node.occurredTime) }}
                    </span>
                    <Tag class="m-0" color="default">
                      {{ formatServiceDisplay(node.service) }}
                    </Tag>
                    <Tag
                      class="m-0"
                      :color="getActorBadge(node.actorType).color"
                    >
                      {{ getActorBadge(node.actorType).text }}
                      <template v-if="node.actorId">
                        · {{ node.actorId }}
                      </template>
                    </Tag>
                    <Tag
                      class="m-0"
                      :color="node.success ? 'success' : 'error'"
                    >
                      {{ node.success ? '成功' : node.errorCode || '失败' }}
                    </Tag>
                  </Space>

                  <Tag
                    v-if="
                      resolveAuditEventResourceName(node) === activeResourceName
                    "
                    class="m-0"
                    color="processing"
                  >
                    当前查看
                  </Tag>
                  <Button
                    v-else
                    size="small"
                    type="link"
                    @click.stop="handleSelectTimelineNode(node)"
                  >
                    查看此跳
                  </Button>
                </div>
                <Tooltip :title="node.operation">
                  <div class="text-foreground mt-1 truncate font-mono text-xs">
                    {{ formatShortOperation(node.operation) }}
                    <span class="text-muted-foreground ml-1">
                      ({{ node.operation || node.eventType || '—' }})
                    </span>
                  </div>
                </Tooltip>
              </div>
            </Timeline.Item>
          </Timeline>
        </template>
      </div>

      <Divider class="!my-3" />

      <div class="space-y-2">
        <div class="flex items-center justify-between gap-2">
          <h4 class="text-foreground text-sm font-semibold">
            脱敏载荷摘要（detail_json）
          </h4>
          <Button
            v-if="!detailPayload.isEmptyObject"
            size="small"
            type="link"
            @click="copyText(detailPayload.formatted, '载荷 JSON')"
          >
            复制 JSON
          </Button>
        </div>

        <div
          v-if="detailPayload.isEmptyObject"
          class="text-muted-foreground rounded-md border border-dashed px-3 py-2.5 text-xs"
        >
          无附加请求载荷（<code class="font-mono">{}</code>）
        </div>
        <pre
          v-else
          class="bg-muted/50 border-border max-h-80 overflow-auto rounded-md border p-3 font-mono text-xs leading-5"
          >{{ detailPayload.formatted }}</pre>
      </div>
    </div>
  </Drawer>
</template>
