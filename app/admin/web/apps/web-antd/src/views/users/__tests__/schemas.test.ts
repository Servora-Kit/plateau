import { describe, expect, it } from 'vitest';

import {
  applyCursorPageResult,
  buildCreateUserRequest,
  buildProfileUpdateRequest,
  buildUsersListRequest,
  createInitialCursorState,
  extractProfileFormValues,
  getCreateUserFormSchema,
  getUsersGridColumns,
  isExpungeEmailConfirmed,
  stepCursorNext,
  stepCursorPrev,
} from '../schemas';

describe('buildUsersListRequest', () => {
  it('builds active-view AIP-160 filter and orderBy using generated UserFields', () => {
    const req = buildUsersListRequest(
      'active',
      {
        email: ' alice@example.com ',
        orderBy: 'update_time_asc',
        status: 'USER_STATUS_ACTIVE',
        userId: ' u_01 ',
      },
      20,
      'tok_1',
    );

    expect(req).toEqual({
      filter:
        '(email = "*alice@example.com*" AND status = USER_STATUS_ACTIVE AND user_id = "u_01")',
      includeTotal: true,
      orderBy: 'update_time',
      pageSize: 20,
      pageToken: 'tok_1',
      showDeleted: false,
    });
  });

  it('builds recycle-bin request with showDeleted=true, delete_time != null, and ignores status filter', () => {
    const req = buildUsersListRequest(
      'deleted',
      {
        email: 'gone@example.com',
        status: 'USER_STATUS_DISABLED',
      },
      10,
      '',
    );

    expect(req).toEqual({
      filter: '(delete_time != null AND email = "*gone@example.com*")',
      includeTotal: true,
      orderBy: 'delete_time desc',
      pageSize: 10,
      pageToken: undefined,
      showDeleted: true,
    });
  });
});

describe('cursor pagination helpers', () => {
  it('advances cursor stack, steps back, and auto-falls back when tail page becomes empty', () => {
    let state = createInitialCursorState(20);
    const first = applyCursorPageResult(state, 20, 'cursor_p2', '21');
    expect(first.fallbackToPreviousPage).toBe(false);
    state = first.state;
    expect(state.totalSize).toBe(21);

    const page2 = stepCursorNext(state);
    if (!page2) {
      throw new Error('Expected next page cursor state');
    }
    expect(page2.currentPageIndex).toBe(1);
    expect(page2.cursorStack).toEqual(['', 'cursor_p2']);

    // Deleting the sole user on page 2 yields 0 items -> auto fallback to page 1
    const afterDelete = applyCursorPageResult(page2, 0, '', '20');
    expect(afterDelete.fallbackToPreviousPage).toBe(true);
    expect(afterDelete.state.currentPageIndex).toBe(0);
    expect(afterDelete.state.cursorStack).toEqual(['']);

    expect(stepCursorPrev(afterDelete.state)).toBeNull();
  });
});

describe('buildProfileUpdateRequest', () => {
  it('emits only changed and cleared fields in FieldMask and preserves etag', () => {
    const initial = extractProfileFormValues({
      familyName: 'Lee',
      givenName: 'Horon',
      locale: 'zh-CN',
      name: 'Horon Lee',
      nickname: 'OldNick',
      picture: 'https://img.example/old.png',
      preferredUsername: 'horon',
    });

    expect(
      buildProfileUpdateRequest('users/u_1', 'W/"1"', initial, { ...initial }),
    ).toBeNull();

    const updated = buildProfileUpdateRequest('users/u_1', 'W/"1"', initial, {
      ...initial,
      name: 'Horon L.',
      nickname: '   ', // cleared!
    });

    expect(updated).toEqual({
      updateMask: 'profile.name,profile.nickname',
      user: {
        etag: 'W/"1"',
        name: 'users/u_1',
        profile: {
          name: 'Horon L.',
        },
      },
    });
  });
});

describe('buildCreateUserRequest and isExpungeEmailConfirmed', () => {
  it('always passes explicit boolean requirePasswordChange (both true and false)', () => {
    const defaultChecked = buildCreateUserRequest({
      email: ' new@example.com ',
      name: ' New User ',
      password: 'Password123!',
      requirePasswordChange: true,
    });
    expect(defaultChecked).toEqual({
      password: 'Password123!',
      requirePasswordChange: true,
      user: {
        email: 'new@example.com',
        profile: { name: 'New User' },
      },
    });

    const unchecked = buildCreateUserRequest({
      email: 'direct@example.com',
      password: 'Password123!',
      requirePasswordChange: false,
    });
    expect(unchecked.requirePasswordChange).toBe(false);
    expect(typeof unchecked.requirePasswordChange).toBe('boolean');
  });

  it('configures requirePasswordChange Checkbox with defaultValue=true and Vue default slot label', () => {
    const schema = getCreateUserFormSchema();
    const checkboxItem = schema.find(
      (item) => item.fieldName === 'requirePasswordChange',
    );
    if (checkboxItem?.component !== 'Checkbox') {
      throw new Error('Expected Checkbox schema for requirePasswordChange');
    }
    expect(checkboxItem.defaultValue).toBe(true);
    const slots = checkboxItem.renderComponentContent?.({});
    expect(slots?.default?.()).toBe('要求该用户首次登录 IAM 时修改初始密码');
  });

  it('matches confirmation email case-insensitively with trim and Unicode NFC', () => {
    expect(
      isExpungeEmailConfirmed('  USER@Example.com ', 'user@example.com'),
    ).toBe(true);
    expect(
      isExpungeEmailConfirmed('other@example.com', 'user@example.com'),
    ).toBe(false);
    expect(isExpungeEmailConfirmed('', '')).toBe(false);
  });

  it('disables single-line cell overflow clipping on multi-line user table columns', () => {
    const cols = getUsersGridColumns('active') ?? [];
    const byField = Object.fromEntries(cols.map((c) => [c.field, c]));
    expect(byField.email?.showOverflow).toBe(false);
    expect(byField.status?.showOverflow).toBe(false);
    expect(byField.actions?.showOverflow).toBe(false);
    expect(byField.userId?.showOverflow).toBe('tooltip');
  });
});
