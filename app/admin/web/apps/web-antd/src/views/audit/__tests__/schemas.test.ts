import dayjs from 'dayjs';
import { describe, expect, it } from 'vitest';

import {
  applyAuditCursorPageResult,
  buildAuditListRequest,
  buildAuditQuerySignature,
  buildServiceAutoCompleteOptions,
  createDefaultAuditTimeRange,
  createInitialAuditCursorState,
  extractAuditInitialQuery,
  extractLinkedUserIdFromResource,
  formatDetailJson,
  formatServiceDisplay,
  formatShortOperation,
  hasPreciseAuditUrlQuery,
  isInvertedTimeRange,
  mergeAuditServices,
  normalizeServiceFilter,
  shouldExpandAuditAdvancedFilters,
  stepAuditCursorNext,
  stepAuditCursorPrev,
} from '../schemas';

describe('audit service discovery and normalization', () => {
  it('merges baseline, backend ListServices, and page services without duplicates', () => {
    const merged = mergeAuditServices(
      ['//cms.service', '//iam.service'],
      ['//mall.service', ''],
    );
    expect(merged).toEqual([
      '//admin.service',
      '//audit.service',
      '//cms.service',
      '//iam.service',
      '//mall.service',
    ]);

    const options = buildServiceAutoCompleteOptions(merged);
    expect(options).toEqual([
      { label: 'admin.service', value: 'admin.service' },
      { label: 'audit.service', value: 'audit.service' },
      { label: 'cms.service', value: 'cms.service' },
      { label: 'iam.service', value: 'iam.service' },
      { label: 'mall.service', value: 'mall.service' },
    ]);
  });

  it('normalizes bare and prefixed service inputs while respecting verbatim non-prefixed sources', () => {
    expect(normalizeServiceFilter('')).toBeUndefined();
    expect(normalizeServiceFilter('  ')).toBeUndefined();
    expect(normalizeServiceFilter('//')).toBeUndefined();
    expect(normalizeServiceFilter('iam.service')).toBe('//iam.service');
    expect(normalizeServiceFilter(' //cms.service ')).toBe('//cms.service');
    expect(
      normalizeServiceFilter('legacy-worker', [
        '//iam.service',
        'legacy-worker',
      ]),
    ).toBe('legacy-worker');
    expect(formatServiceDisplay('//admin.service')).toBe('admin.service');
  });
});

describe('audit list request, frozen time range, and cursor pagination', () => {
  it('keeps frozen preset timestamps identical across pagination steps', () => {
    const anchor = dayjs('2026-11-25T05:00:00.123Z');
    const frozenRange = createDefaultAuditTimeRange(anchor);

    const page1Req = buildAuditListRequest(
      'AUDIT_CATEGORY_RESOURCE',
      {
        actorType: 'human',
        outcome: 'failure',
        service: 'admin.service',
        timeRange: frozenRange,
        traceId: '  4BF92F3577B34DA6A3CE929D0E0E4736 ',
      },
      20,
      '',
    );

    const page2Req = buildAuditListRequest(
      'AUDIT_CATEGORY_RESOURCE',
      {
        actorType: 'human',
        outcome: 'failure',
        service: 'admin.service',
        timeRange: frozenRange,
        traceId: '  4BF92F3577B34DA6A3CE929D0E0E4736 ',
      },
      20,
      'cursor-page-2',
    );

    expect(page1Req).toEqual({
      actorType: 'human',
      category: 'AUDIT_CATEGORY_RESOURCE',
      endTime: '2026-11-25T05:00:00.123Z',
      pageSize: 20,
      service: '//admin.service',
      startTime: '2026-11-18T05:00:00.123Z',
      success: false,
      traceId: '4bf92f3577b34da6a3ce929d0e0e4736',
    });
    expect(page2Req.startTime).toBe(page1Req.startTime);
    expect(page2Req.endTime).toBe(page1Req.endTime);
    expect(page2Req.pageToken).toBe('cursor-page-2');

    expect(
      buildAuditQuerySignature(
        'AUDIT_CATEGORY_RESOURCE',
        { service: 'admin.service', timeRange: frozenRange },
        20,
      ),
    ).toBe(
      buildAuditQuerySignature(
        'AUDIT_CATEGORY_RESOURCE',
        { service: '//admin.service', timeRange: frozenRange },
        20,
      ),
    );
  });

  it('detects inverted time ranges and URL query auto-expansion rules', () => {
    const start = dayjs('2026-11-25T10:00:00Z');
    const end = dayjs('2026-11-25T09:00:00Z');
    expect(isInvertedTimeRange([start, end])).toBe(true);
    expect(isInvertedTimeRange([end, start])).toBe(false);

    expect(hasPreciseAuditUrlQuery({})).toBe(false);
    expect(hasPreciseAuditUrlQuery({ traceId: 'abc' })).toBe(true);
    expect(hasPreciseAuditUrlQuery({ event: 'events/ev-1' })).toBe(true);
    expect(shouldExpandAuditAdvancedFilters({ service: 'iam.service' })).toBe(
      false,
    );
    expect(
      shouldExpandAuditAdvancedFilters({ resourceName: 'users/u-1' }),
    ).toBe(true);

    expect(
      extractAuditInitialQuery({
        actorId: ' u-1 ',
        actorType: 'human',
        service: '//iam.service',
      }),
    ).toEqual({
      actorId: 'u-1',
      actorType: 'human',
      service: 'iam.service',
    });
  });

  it('advances, steps back, and falls back on empty trailing cursor page without totalSize', () => {
    const initial = createInitialAuditCursorState(20);
    const p1 = applyAuditCursorPageResult(initial, 20, 'tok-2');
    expect(p1.fallbackToPreviousPage).toBe(false);
    expect(p1.state.currentPageCount).toBe(20);
    expect(p1.state.nextPageToken).toBe('tok-2');

    const p2Pending = stepAuditCursorNext(p1.state);
    if (!p2Pending) {
      throw new Error('expected next cursor state');
    }
    expect(p2Pending.currentPageIndex).toBe(1);
    expect(p2Pending.cursorStack).toEqual(['', 'tok-2']);

    const emptyTrailing = applyAuditCursorPageResult(p2Pending, 0, '');
    expect(emptyTrailing.fallbackToPreviousPage).toBe(true);
    expect(emptyTrailing.state.currentPageIndex).toBe(0);

    const p2Loaded = applyAuditCursorPageResult(p2Pending, 5, '');
    const backToP1 = stepAuditCursorPrev(p2Loaded.state);
    expect(backToP1?.currentPageIndex).toBe(0);
    expect(backToP1?.cursorStack).toEqual(['']);
  });
});

describe('audit detail formatting and user link extraction', () => {
  it('formats detail_json safely and identifies empty objects', () => {
    expect(formatDetailJson('')).toEqual({
      formatted: '{}',
      isEmptyObject: true,
    });
    expect(formatDetailJson(' { } ')).toEqual({
      formatted: '{}',
      isEmptyObject: true,
    });
    expect(formatDetailJson('{"name":"users/u-1"}')).toEqual({
      formatted: '{\n  "name": "users/u-1"\n}',
      isEmptyObject: false,
    });
    expect(formatDetailJson('not-json')).toEqual({
      formatted: 'not-json',
      isEmptyObject: false,
    });
  });

  it('extracts short RPC operation and canonical userId from resource names', () => {
    expect(formatShortOperation('/admin.user.v1.UserService/DisableUser')).toBe(
      'DisableUser',
    );
    expect(formatShortOperation('GET /v1/admin/auth/callback')).toBe(
      'GET /v1/admin/auth/callback',
    );
    expect(extractLinkedUserIdFromResource('users/0195f4a1')).toBe('0195f4a1');
    expect(extractLinkedUserIdFromResource('admin/global')).toBeNull();
  });
});
