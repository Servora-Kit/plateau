import type { ListEventsRequest } from '@plateau/api/admin/audit/v1';
import type { Event } from '@plateau/api/audit/event/v1';
import type { AuditCategory } from '@plateau/api/plateau/security/audit/v1';
import type { Dayjs } from 'dayjs';

import type { VbenFormSchema } from '#/adapter/form';
import type { VxeTableGridOptions } from '#/adapter/vxe-table';

import dayjs from 'dayjs';

export const BASELINE_AUDIT_SERVICES = [
  '//admin.service',
  '//audit.service',
  '//iam.service',
] as const;

export const TRACE_TIMELINE_PAGE_SIZE = 200;

export type AuditCategoryTab =
  | 'all'
  | 'AUDIT_CATEGORY_AUTHN'
  | 'AUDIT_CATEGORY_AUTHZ'
  | 'AUDIT_CATEGORY_RESOURCE';

export type AuditActorTypeOption = '' | 'anonymous' | 'human' | 'service';

export type AuditOutcomeOption = '' | 'failure' | 'success';

export type AuditTimeValue = Date | Dayjs | number | string;

export interface AuditSearchFormValues {
  actorId?: string;
  actorType?: AuditActorTypeOption;
  operation?: string;
  outcome?: AuditOutcomeOption;
  resourceName?: string;
  service?: string;
  timeRange?: [AuditTimeValue, AuditTimeValue] | null;
  traceId?: string;
}

export interface AuditCursorState {
  currentPageCount: number;
  currentPageIndex: number;
  cursorStack: string[];
  nextPageToken: string;
  pageSize: number;
}

const PRECISE_QUERY_KEYS = [
  'actorId',
  'event',
  'operation',
  'resourceName',
  'traceId',
] as const;

const FOLDED_FILTER_KEYS = [
  'actorId',
  'operation',
  'resourceName',
  'traceId',
] as const;

function firstQueryString(value: unknown): string {
  if (Array.isArray(value)) {
    return typeof value[0] === 'string' ? value[0].trim() : '';
  }
  return typeof value === 'string' ? value.trim() : '';
}

export function extractAuditInitialQuery(
  query: Record<string, unknown>,
): Partial<AuditSearchFormValues> & { eventName?: string } {
  const actorId = firstQueryString(query.actorId);
  const actorTypeRaw = firstQueryString(query.actorType);
  const operation = firstQueryString(query.operation);
  const resourceName = firstQueryString(query.resourceName);
  const service = firstQueryString(query.service);
  const traceId = firstQueryString(query.traceId);
  const eventName = firstQueryString(query.event);

  const actorType: AuditActorTypeOption =
    actorTypeRaw === 'human' ||
    actorTypeRaw === 'service' ||
    actorTypeRaw === 'anonymous'
      ? actorTypeRaw
      : '';

  return {
    ...(actorId ? { actorId } : {}),
    ...(actorType ? { actorType } : {}),
    ...(eventName ? { eventName } : {}),
    ...(operation ? { operation } : {}),
    ...(resourceName ? { resourceName } : {}),
    ...(service ? { service: formatServiceDisplay(service) } : {}),
    ...(traceId ? { traceId } : {}),
  };
}

export function hasPreciseAuditUrlQuery(
  query: Record<string, unknown>,
): boolean {
  return PRECISE_QUERY_KEYS.some((key) => firstQueryString(query[key]) !== '');
}

export function shouldExpandAuditAdvancedFilters(
  query: Record<string, unknown>,
): boolean {
  return FOLDED_FILTER_KEYS.some((key) => firstQueryString(query[key]) !== '');
}

export function createDefaultAuditTimeRange(
  now: Dayjs = dayjs(),
): [Dayjs, Dayjs] {
  const end = now;
  const start = end.subtract(7, 'day');
  return [start, end];
}

export function formatServiceDisplay(service?: null | string): string {
  const trimmed = service?.trim() ?? '';
  if (!trimmed) {
    return '—';
  }
  const stripped = trimmed.replace(/^\/\/+/, '');
  return stripped || '—';
}

export function mergeAuditServices(
  ...sources: Array<ReadonlyArray<null | string | undefined>>
): string[] {
  const merged = new Set<string>(BASELINE_AUDIT_SERVICES);
  for (const list of sources) {
    for (const item of list) {
      const trimmed = item?.trim();
      if (trimmed) {
        merged.add(trimmed);
      }
    }
  }
  return [...merged].toSorted((a, b) =>
    formatServiceDisplay(a).localeCompare(formatServiceDisplay(b)),
  );
}

export function buildServiceAutoCompleteOptions(
  rawServices: ReadonlyArray<string>,
): Array<{ label: string; value: string }> {
  const seen = new Set<string>();
  const options: Array<{ label: string; value: string }> = [];
  for (const raw of rawServices) {
    const display = formatServiceDisplay(raw);
    if (display === '—' || seen.has(display)) {
      continue;
    }
    seen.add(display);
    options.push({ label: display, value: display });
  }
  return options.toSorted((a, b) => a.label.localeCompare(b.label));
}

export function normalizeServiceFilter(
  rawInput?: null | string,
  knownRawServices: ReadonlyArray<string> = [],
): string | undefined {
  const trimmed = rawInput?.trim() ?? '';
  if (!trimmed) {
    return undefined;
  }
  if (knownRawServices.includes(trimmed)) {
    return trimmed;
  }
  if (trimmed.startsWith('//')) {
    const withoutSlashes = trimmed.replace(/^\/\/+/, '');
    return withoutSlashes ? `//${withoutSlashes}` : undefined;
  }
  return `//${trimmed}`;
}

export function toIsoTimestamp(
  input?: AuditTimeValue | null,
): string | undefined {
  if (input === undefined || input === null || input === '') {
    return undefined;
  }
  const parsed = dayjs.isDayjs(input) ? input : dayjs(input);
  if (!parsed.isValid()) {
    return undefined;
  }
  return parsed.toDate().toISOString();
}

export function isInvertedTimeRange(
  timeRange?: [AuditTimeValue, AuditTimeValue] | null,
): boolean {
  if (!Array.isArray(timeRange) || timeRange.length < 2) {
    return false;
  }
  const startIso = toIsoTimestamp(timeRange[0]);
  const endIso = toIsoTimestamp(timeRange[1]);
  if (!startIso || !endIso) {
    return false;
  }
  return new Date(startIso).getTime() > new Date(endIso).getTime();
}

export function resolveAuditEventResourceName(
  event: Pick<Event, 'eventId' | 'name'>,
): string {
  if (event.name?.trim()) {
    return event.name.trim();
  }
  if (event.eventId?.trim()) {
    return `events/${event.eventId.trim()}`;
  }
  return '';
}

export function buildAuditListRequest(
  categoryTab: AuditCategoryTab,
  formValues: AuditSearchFormValues,
  pageSize: number,
  pageToken: string,
  knownRawServices: ReadonlyArray<string> = [],
): ListEventsRequest {
  const category: AuditCategory | undefined =
    categoryTab === 'all' ? undefined : categoryTab;

  let startTime: string | undefined;
  let endTime: string | undefined;
  if (Array.isArray(formValues.timeRange) && formValues.timeRange.length >= 2) {
    startTime = toIsoTimestamp(formValues.timeRange[0]);
    endTime = toIsoTimestamp(formValues.timeRange[1]);
  }

  let success: boolean | undefined;
  if (formValues.outcome === 'success') {
    success = true;
  } else if (formValues.outcome === 'failure') {
    success = false;
  }

  const service = normalizeServiceFilter(formValues.service, knownRawServices);
  const actorType = formValues.actorType?.trim() || undefined;
  const actorId = formValues.actorId?.trim() || undefined;
  const resourceName = formValues.resourceName?.trim() || undefined;
  const operation = formValues.operation?.trim() || undefined;
  const traceId = formValues.traceId?.trim().toLowerCase() || undefined;
  const token = pageToken.trim() || undefined;

  return {
    ...(actorId ? { actorId } : {}),
    ...(actorType ? { actorType } : {}),
    ...(category ? { category } : {}),
    ...(endTime ? { endTime } : {}),
    ...(operation ? { operation } : {}),
    pageSize,
    ...(token ? { pageToken: token } : {}),
    ...(resourceName ? { resourceName } : {}),
    ...(service ? { service } : {}),
    ...(startTime ? { startTime } : {}),
    ...(success === undefined ? {} : { success }),
    ...(traceId ? { traceId } : {}),
  };
}

export function buildAuditQuerySignature(
  categoryTab: AuditCategoryTab,
  formValues: AuditSearchFormValues,
  pageSize: number,
  knownRawServices: ReadonlyArray<string> = [],
): string {
  const normalized = buildAuditListRequest(
    categoryTab,
    formValues,
    pageSize,
    '',
    knownRawServices,
  );
  return JSON.stringify({
    actorId: normalized.actorId ?? '',
    actorType: normalized.actorType ?? '',
    category: normalized.category ?? '',
    endTime: normalized.endTime ?? '',
    operation: normalized.operation ?? '',
    pageSize: normalized.pageSize ?? 20,
    resourceName: normalized.resourceName ?? '',
    service: normalized.service ?? '',
    startTime: normalized.startTime ?? '',
    success: normalized.success ?? null,
    traceId: normalized.traceId ?? '',
  });
}

export function createInitialAuditCursorState(pageSize = 20): AuditCursorState {
  return {
    currentPageCount: 0,
    currentPageIndex: 0,
    cursorStack: [''],
    nextPageToken: '',
    pageSize,
  };
}

export function applyAuditCursorPageResult(
  state: AuditCursorState,
  itemCount: number,
  nextPageToken?: string,
): { fallbackToPreviousPage: boolean; state: AuditCursorState } {
  const cleanNext = nextPageToken?.trim() ?? '';
  if (itemCount === 0 && state.currentPageIndex > 0) {
    const prevIndex = state.currentPageIndex - 1;
    return {
      fallbackToPreviousPage: true,
      state: {
        ...state,
        currentPageCount: 0,
        currentPageIndex: prevIndex,
        cursorStack: state.cursorStack.slice(0, prevIndex + 1),
        nextPageToken: '',
      },
    };
  }
  return {
    fallbackToPreviousPage: false,
    state: {
      ...state,
      currentPageCount: itemCount,
      cursorStack: state.cursorStack.slice(0, state.currentPageIndex + 1),
      nextPageToken: cleanNext,
    },
  };
}

export function stepAuditCursorNext(
  state: AuditCursorState,
): AuditCursorState | null {
  if (!state.nextPageToken) {
    return null;
  }
  const nextIndex = state.currentPageIndex + 1;
  const nextStack = state.cursorStack.slice(0, nextIndex);
  nextStack[nextIndex] = state.nextPageToken;
  return {
    ...state,
    currentPageIndex: nextIndex,
    cursorStack: nextStack,
    nextPageToken: '',
  };
}

export function stepAuditCursorPrev(
  state: AuditCursorState,
): AuditCursorState | null {
  if (state.currentPageIndex <= 0) {
    return null;
  }
  const prevIndex = state.currentPageIndex - 1;
  return {
    ...state,
    currentPageIndex: prevIndex,
    cursorStack: state.cursorStack.slice(0, prevIndex + 1),
    nextPageToken: '',
  };
}

export function formatShortOperation(operation?: null | string): string {
  const trimmed = operation?.trim() ?? '';
  if (!trimmed) {
    return '—';
  }
  if (trimmed.startsWith('/')) {
    const parts = trimmed.split('/').filter(Boolean);
    if (parts.length >= 2) {
      return parts.at(-1) ?? trimmed;
    }
  }
  return trimmed;
}

export function getCategoryBadge(category?: AuditCategory | null | string): {
  color: string;
  text: string;
} {
  switch (category) {
    case 'AUDIT_CATEGORY_AUTHN': {
      return { color: 'processing', text: '身份认证' };
    }
    case 'AUDIT_CATEGORY_AUTHZ': {
      return { color: 'error', text: '授权拒绝' };
    }
    case 'AUDIT_CATEGORY_RESOURCE': {
      return { color: 'purple', text: '资源变更' };
    }
    default: {
      return { color: 'default', text: '未分类' };
    }
  }
}

export function getActorBadge(actorType?: null | string): {
  color: string;
  text: string;
} {
  switch (actorType?.trim().toLowerCase()) {
    case 'anonymous': {
      return { color: 'default', text: '匿名' };
    }
    case 'human': {
      return { color: 'blue', text: '人类' };
    }
    case 'service': {
      return { color: 'cyan', text: '服务' };
    }
    default: {
      return { color: 'default', text: actorType?.trim() || '未知' };
    }
  }
}

export function formatDetailJson(raw?: null | string): {
  formatted: string;
  isEmptyObject: boolean;
} {
  const trimmed = raw?.trim() ?? '';
  if (!trimmed || trimmed === '{}') {
    return { formatted: '{}', isEmptyObject: true };
  }
  try {
    const parsed: unknown = JSON.parse(trimmed);
    if (
      parsed !== null &&
      typeof parsed === 'object' &&
      !Array.isArray(parsed) &&
      Object.keys(parsed).length === 0
    ) {
      return { formatted: '{}', isEmptyObject: true };
    }
    return {
      formatted: JSON.stringify(parsed, null, 2),
      isEmptyObject: false,
    };
  } catch {
    return { formatted: trimmed, isEmptyObject: false };
  }
}

export function extractLinkedUserIdFromResource(
  resourceName?: null | string,
): null | string {
  const trimmed = resourceName?.trim() ?? '';
  const match = /^users\/([^/\s]+)$/.exec(trimmed);
  return match?.[1] ?? null;
}

export function getAuditSearchFormSchema(options: {
  defaultTimeRange?: [Dayjs, Dayjs] | null;
  initialValues?: Partial<AuditSearchFormValues>;
  serviceOptions: Array<{ label: string; value: string }>;
}): VbenFormSchema[] {
  const {
    defaultTimeRange = null,
    initialValues = {},
    serviceOptions,
  } = options;

  return [
    {
      component: 'RangePicker',
      componentProps: {
        allowClear: true,
        class: 'w-full',
        placeholder: ['开始时间', '结束时间'],
        presets: [
          {
            label: '近 1 小时',
            value: () => {
              const end = dayjs();
              return [end.subtract(1, 'hour'), end] as [Dayjs, Dayjs];
            },
          },
          {
            label: '近 24 小时',
            value: () => {
              const end = dayjs();
              return [end.subtract(24, 'hour'), end] as [Dayjs, Dayjs];
            },
          },
          {
            label: '近 7 天',
            value: () => {
              const end = dayjs();
              return [end.subtract(7, 'day'), end] as [Dayjs, Dayjs];
            },
          },
          {
            label: '近 30 天',
            value: () => {
              const end = dayjs();
              return [end.subtract(30, 'day'), end] as [Dayjs, Dayjs];
            },
          },
        ],
        showTime: true,
      },
      defaultValue: defaultTimeRange ?? undefined,
      fieldName: 'timeRange',
      label: '时间范围',
    },
    {
      component: 'Select',
      componentProps: {
        allowClear: true,
        options: [
          { label: '全部主体', value: '' },
          { label: '人类（human）', value: 'human' },
          { label: '服务（service）', value: 'service' },
          { label: '匿名（anonymous）', value: 'anonymous' },
        ],
        placeholder: '全部主体',
      },
      defaultValue: initialValues.actorType ?? '',
      fieldName: 'actorType',
      label: '主体类型',
    },
    {
      component: 'Select',
      componentProps: {
        allowClear: true,
        options: [
          { label: '全部结果', value: '' },
          { label: '成功', value: 'success' },
          { label: '失败 / 拒绝', value: 'failure' },
        ],
        placeholder: '全部结果',
      },
      defaultValue: initialValues.outcome ?? '',
      fieldName: 'outcome',
      label: '执行结果',
    },
    {
      component: 'AutoComplete',
      componentProps: {
        allowClear: true,
        filterOption: (input: string, option?: { value?: string }) =>
          (option?.value ?? '').toLowerCase().includes(
            input
              .trim()
              .replace(/^\/\/+/, '')
              .toLowerCase(),
          ),
        options: serviceOptions,
        placeholder: '选择或输入服务名（如 iam.service）',
      },
      defaultValue: initialValues.service ?? '',
      fieldName: 'service',
      label: '生产服务',
    },
    {
      component: 'Input',
      componentProps: {
        allowClear: true,
        placeholder: '精确匹配主体 ID（如用户 ID 或 client_id）',
      },
      defaultValue: initialValues.actorId ?? '',
      fieldName: 'actorId',
      label: '主体 ID',
    },
    {
      component: 'Input',
      componentProps: {
        allowClear: true,
        placeholder: '如 users/0195f4...',
      },
      defaultValue: initialValues.resourceName ?? '',
      fieldName: 'resourceName',
      label: '目标资源',
    },
    {
      component: 'Input',
      componentProps: {
        allowClear: true,
        placeholder: '如 /admin.user.v1.UserService/DisableUser',
      },
      defaultValue: initialValues.operation ?? '',
      fieldName: 'operation',
      label: '操作名称',
    },
    {
      component: 'Input',
      componentProps: {
        allowClear: true,
        placeholder: '32 位小写十六进制 TraceID',
      },
      defaultValue: initialValues.traceId ?? '',
      fieldName: 'traceId',
      label: '链路 TraceID',
    },
  ];
}

export function getAuditGridColumns(): VxeTableGridOptions<Event>['columns'] {
  return [
    {
      align: 'left',
      field: 'occurredTime',
      minWidth: 185,
      showOverflow: false,
      slots: { default: 'occurredTime' },
      title: '发生时间',
    },
    {
      align: 'left',
      field: 'category',
      minWidth: 120,
      showOverflow: false,
      slots: { default: 'category' },
      title: '类别',
    },
    {
      align: 'left',
      field: 'operation',
      minWidth: 250,
      showOverflow: false,
      slots: { default: 'serviceOperation' },
      title: '生产服务 & 操作',
    },
    {
      align: 'left',
      field: 'actorId',
      minWidth: 220,
      showOverflow: false,
      slots: { default: 'actor' },
      title: '操作主体',
    },
    {
      align: 'left',
      field: 'resourceName',
      minWidth: 200,
      showOverflow: 'tooltip',
      slots: { default: 'resourceName' },
      title: '目标资源',
    },
    {
      align: 'left',
      field: 'success',
      minWidth: 190,
      showOverflow: false,
      slots: { default: 'outcome' },
      title: '执行结果',
    },
    {
      align: 'left',
      field: 'clientIp',
      minWidth: 135,
      showOverflow: 'tooltip',
      slots: { default: 'clientIp' },
      title: '来源 IP',
    },
    {
      align: 'right',
      field: 'actions',
      fixed: 'right',
      minWidth: 96,
      showOverflow: false,
      slots: { default: 'actions' },
      title: '操作',
    },
  ];
}
