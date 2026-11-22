import type {
  CreateUserRequest,
  ListUsersRequest,
  UpdateUserRequest,
} from '@plateau/api/admin/user/v1';
import type { User, UserProfile, UserStatus } from '@plateau/api/iam/user/v1';
import type { FilterExpression } from '@servora/proto-utils/crud';

import type { VbenFormSchema } from '#/adapter/form';
import type { VxeTableGridOptions } from '#/adapter/vxe-table';

import {
  UserFields,
  UserName,
  UserUpdateFields,
} from '@plateau/api/iam/user/v1/user.crud';
import {
  buildFilter,
  buildOrderBy,
  filterContains,
  makeUpdateMask,
  rawFilterValue,
} from '@servora/proto-utils/crud';

import { z } from '#/adapter/form';

export type UsersViewMode = 'active' | 'deleted';

export type ActiveSortOption =
  | 'create_time_asc'
  | 'create_time_desc'
  | 'update_time_asc'
  | 'update_time_desc';

export type DeletedSortOption =
  | 'create_time_desc'
  | 'delete_time_asc'
  | 'delete_time_desc';

export interface UsersSearchFormValues {
  email?: string;
  orderBy?: ActiveSortOption | DeletedSortOption;
  status?: '' | UserStatus;
  userId?: string;
}

export interface CursorPaginationState {
  currentPageIndex: number;
  cursorStack: string[];
  nextPageToken: string;
  pageSize: number;
  totalSize: number;
}

export interface ProfileFormValues {
  familyName: string;
  givenName: string;
  locale: string;
  name: string;
  nickname: string;
  picture: string;
  preferredUsername: string;
}

export interface CreateUserFormValues {
  email: string;
  name?: string;
  nickname?: string;
  password: string;
  requirePasswordChange?: boolean;
}

const VALID_FILTER_STATUSES: Partial<Record<UserStatus, true>> = {
  USER_STATUS_ACTIVE: true,
  USER_STATUS_DISABLED: true,
  USER_STATUS_PENDING_EMAIL_VERIFICATION: true,
};

export function resolveUserResourceName(
  user: Pick<User, 'name' | 'userId'>,
): string {
  if (user.name?.trim()) {
    return user.name.trim();
  }
  return UserName.format({ user: user.userId ?? '' });
}

export function buildUsersListRequest(
  viewMode: UsersViewMode,
  formValues: UsersSearchFormValues,
  pageSize: number,
  pageToken: string,
): ListUsersRequest {
  type UserFieldName = (typeof UserFields)[keyof typeof UserFields];
  const clauses: Array<FilterExpression<UserFieldName>> = [];

  if (viewMode === 'deleted') {
    clauses.push({
      field: UserFields.deleteTime,
      operator: '!=',
      value: null,
    });
  }

  const trimmedEmail = formValues.email?.trim();
  if (trimmedEmail) {
    clauses.push({
      field: UserFields.email,
      operator: '=',
      value: filterContains(trimmedEmail),
    });
  }

  if (
    viewMode === 'active' &&
    formValues.status &&
    VALID_FILTER_STATUSES[formValues.status]
  ) {
    clauses.push({
      field: UserFields.status,
      operator: '=',
      value: rawFilterValue(formValues.status),
    });
  }

  const trimmedUserId = formValues.userId?.trim();
  if (trimmedUserId) {
    clauses.push({
      field: UserFields.userId,
      operator: '=',
      value: trimmedUserId,
    });
  }

  let filter: string | undefined;
  if (clauses.length === 1) {
    filter = buildFilter(UserFields, clauses[0]);
  } else if (clauses.length > 1) {
    filter = buildFilter(UserFields, { and: clauses });
  }

  const sortOption =
    formValues.orderBy ||
    (viewMode === 'deleted' ? 'delete_time_desc' : 'create_time_desc');

  let orderBy: string | undefined;
  switch (sortOption) {
    case 'create_time_asc': {
      orderBy = buildOrderBy(UserFields, [
        { direction: 'asc', field: UserFields.createTime },
      ]);
      break;
    }
    case 'delete_time_asc': {
      orderBy = buildOrderBy(UserFields, [
        { direction: 'asc', field: UserFields.deleteTime },
      ]);
      break;
    }
    case 'delete_time_desc': {
      orderBy = buildOrderBy(UserFields, [
        { direction: 'desc', field: UserFields.deleteTime },
      ]);
      break;
    }
    case 'update_time_asc': {
      orderBy = buildOrderBy(UserFields, [
        { direction: 'asc', field: UserFields.updateTime },
      ]);
      break;
    }
    case 'update_time_desc': {
      orderBy = buildOrderBy(UserFields, [
        { direction: 'desc', field: UserFields.updateTime },
      ]);
      break;
    }
    default: {
      orderBy = buildOrderBy(UserFields, [
        { direction: 'desc', field: UserFields.createTime },
      ]);
      break;
    }
  }

  return {
    filter,
    includeTotal: true,
    orderBy,
    pageSize,
    pageToken: pageToken || undefined,
    showDeleted: viewMode === 'deleted',
  };
}

export function createInitialCursorState(pageSize = 20): CursorPaginationState {
  return {
    currentPageIndex: 0,
    cursorStack: [''],
    nextPageToken: '',
    pageSize,
    totalSize: 0,
  };
}

export function applyCursorPageResult(
  state: CursorPaginationState,
  itemsCount: number,
  nextPageToken: string | undefined,
  totalSize: string | undefined,
): { fallbackToPreviousPage: boolean; state: CursorPaginationState } {
  const parsedTotal = Number.parseInt(totalSize ?? '0', 10);
  const safeTotal =
    Number.isFinite(parsedTotal) && parsedTotal >= 0 ? parsedTotal : 0;

  if (itemsCount === 0 && state.currentPageIndex > 0) {
    const prevIndex = state.currentPageIndex - 1;
    return {
      fallbackToPreviousPage: true,
      state: {
        ...state,
        currentPageIndex: prevIndex,
        cursorStack: state.cursorStack.slice(0, prevIndex + 1),
        nextPageToken: '',
        totalSize: safeTotal,
      },
    };
  }

  return {
    fallbackToPreviousPage: false,
    state: {
      ...state,
      nextPageToken: nextPageToken ?? '',
      totalSize: safeTotal,
    },
  };
}

export function stepCursorNext(
  state: CursorPaginationState,
): CursorPaginationState | null {
  if (!state.nextPageToken) {
    return null;
  }
  const nextIndex = state.currentPageIndex + 1;
  const nextStack = state.cursorStack.slice(0, nextIndex);
  nextStack.push(state.nextPageToken);
  return {
    ...state,
    currentPageIndex: nextIndex,
    cursorStack: nextStack,
    nextPageToken: '',
  };
}

export function stepCursorPrev(
  state: CursorPaginationState,
): CursorPaginationState | null {
  if (state.currentPageIndex <= 0) {
    return null;
  }
  return {
    ...state,
    currentPageIndex: state.currentPageIndex - 1,
    nextPageToken: '',
  };
}

export function extractProfileFormValues(
  profile?: UserProfile,
): ProfileFormValues {
  return {
    familyName: profile?.familyName ?? '',
    givenName: profile?.givenName ?? '',
    locale: profile?.locale ?? '',
    name: profile?.name ?? '',
    nickname: profile?.nickname ?? '',
    picture: profile?.picture ?? '',
    preferredUsername: profile?.preferredUsername ?? '',
  };
}

const PROFILE_FIELD_MAP: ReadonlyArray<{
  formKey: keyof ProfileFormValues;
  maskKey: Exclude<keyof typeof UserUpdateFields, 'profile'>;
}> = [
  { formKey: 'name', maskKey: 'profileName' },
  { formKey: 'givenName', maskKey: 'profileGivenName' },
  { formKey: 'familyName', maskKey: 'profileFamilyName' },
  { formKey: 'nickname', maskKey: 'profileNickname' },
  { formKey: 'preferredUsername', maskKey: 'profilePreferredUsername' },
  { formKey: 'picture', maskKey: 'profilePicture' },
  { formKey: 'locale', maskKey: 'profileLocale' },
];

export function buildProfileUpdateRequest(
  resourceName: string,
  etag: string,
  initialValues: ProfileFormValues,
  nextValues: ProfileFormValues,
): null | UpdateUserRequest {
  const maskPatch: Partial<
    Record<
      Exclude<keyof typeof UserUpdateFields, 'profile'>,
      string | undefined
    >
  > = {};
  const nextProfile: UserProfile = {};
  let changedCount = 0;

  for (const { formKey, maskKey } of PROFILE_FIELD_MAP) {
    const before = (initialValues[formKey] ?? '').trim();
    const after = (nextValues[formKey] ?? '').trim();
    if (before === after) {
      continue;
    }
    changedCount += 1;
    if (after === '') {
      maskPatch[maskKey] = undefined;
    } else {
      maskPatch[maskKey] = after;
      nextProfile[formKey] = after;
    }
  }

  if (changedCount === 0) {
    return null;
  }

  const updateMask = makeUpdateMask(UserUpdateFields, maskPatch);
  if (!updateMask) {
    return null;
  }

  return {
    updateMask,
    user: {
      etag,
      name: resourceName,
      profile: nextProfile,
    },
  };
}

export function buildCreateUserRequest(
  values: CreateUserFormValues,
): CreateUserRequest {
  const profile: UserProfile = {};
  const trimmedName = values.name?.trim();
  const trimmedNickname = values.nickname?.trim();
  if (trimmedName) {
    profile.name = trimmedName;
  }
  if (trimmedNickname) {
    profile.nickname = trimmedNickname;
  }

  return {
    password: values.password,
    requirePasswordChange: Boolean(values.requirePasswordChange),
    user: {
      email: values.email.trim(),
      ...(Object.keys(profile).length > 0 ? { profile } : {}),
    },
  };
}

export function isExpungeEmailConfirmed(
  inputEmail: string,
  targetEmail?: string,
): boolean {
  const normalizedTarget = (targetEmail ?? '')
    .trim()
    .normalize('NFC')
    .toLowerCase();
  if (!normalizedTarget) {
    return false;
  }
  const normalizedInput = inputEmail.trim().normalize('NFC').toLowerCase();
  return normalizedInput === normalizedTarget;
}

export function getUsersSearchFormSchema(
  viewMode: UsersViewMode,
): VbenFormSchema[] {
  const commonFields: VbenFormSchema[] = [
    {
      component: 'Input',
      componentProps: {
        allowClear: true,
        placeholder: '输入邮箱关键字模糊查询',
      },
      fieldName: 'email',
      label: '邮箱',
    },
  ];

  if (viewMode === 'active') {
    commonFields.push({
      component: 'Select',
      componentProps: {
        allowClear: true,
        options: [
          { label: '全部状态', value: '' },
          {
            label: '待邮箱验证',
            value: 'USER_STATUS_PENDING_EMAIL_VERIFICATION',
          },
          { label: '正常（ACTIVE）', value: 'USER_STATUS_ACTIVE' },
          { label: '已封禁（DISABLED）', value: 'USER_STATUS_DISABLED' },
        ],
        placeholder: '选择账号状态',
      },
      defaultValue: '',
      fieldName: 'status',
      label: '状态',
    });
  }

  commonFields.push(
    {
      component: 'Input',
      componentProps: {
        allowClear: true,
        placeholder: '输入精确的用户 ID',
      },
      fieldName: 'userId',
      label: '用户 ID',
    },
    {
      component: 'Select',
      componentProps: {
        options:
          viewMode === 'deleted'
            ? [
                { label: '删除时间：最新优先', value: 'delete_time_desc' },
                { label: '删除时间：最早优先', value: 'delete_time_asc' },
                { label: '创建时间：最新优先', value: 'create_time_desc' },
              ]
            : [
                { label: '创建时间：最新优先', value: 'create_time_desc' },
                { label: '创建时间：最早优先', value: 'create_time_asc' },
                { label: '更新时间：最新优先', value: 'update_time_desc' },
                { label: '更新时间：最早优先', value: 'update_time_asc' },
              ],
      },
      defaultValue:
        viewMode === 'deleted' ? 'delete_time_desc' : 'create_time_desc',
      fieldName: 'orderBy',
      label: '排序方式',
    },
  );

  return commonFields;
}

export function getUsersGridColumns(
  viewMode: UsersViewMode,
): VxeTableGridOptions<User>['columns'] {
  const baseColumns: VxeTableGridOptions<User>['columns'] = [
    {
      align: 'left',
      field: 'email',
      minWidth: 240,
      showOverflow: false,
      slots: { default: 'userIdentity' },
      title: '账号身份',
    },
    {
      align: 'left',
      field: 'userId',
      minWidth: 190,
      showOverflow: 'tooltip',
      slots: { default: 'userId' },
      title: '用户 ID',
    },
    {
      field: 'status',
      minWidth: 210,
      showOverflow: false,
      slots: { default: 'status' },
      title: '生命周期与待办标记',
    },
    {
      field: 'createTime',
      formatter: 'formatDateTime',
      minWidth: 170,
      showOverflow: 'tooltip',
      title: '创建时间',
    },
  ];

  if (viewMode === 'deleted') {
    baseColumns.push({
      field: 'deleteTime',
      formatter: 'formatDateTime',
      minWidth: 170,
      showOverflow: 'tooltip',
      title: '软删除时间',
    });
  } else {
    baseColumns.push({
      field: 'updateTime',
      formatter: 'formatDateTime',
      minWidth: 170,
      showOverflow: 'tooltip',
      title: '更新时间',
    });
  }

  baseColumns.push({
    align: 'right',
    field: 'actions',
    fixed: 'right',
    minWidth: viewMode === 'deleted' ? 240 : 340,
    showOverflow: false,
    slots: { default: 'actions' },
    title: '操作',
  });
  return baseColumns;
}

export function getProfileFormSchema(): VbenFormSchema[] {
  return [
    {
      component: 'Input',
      componentProps: {
        allowClear: true,
        maxlength: 128,
        placeholder: '完整姓名（profile.name）',
      },
      fieldName: 'name',
      label: '显示姓名',
    },
    {
      component: 'Input',
      componentProps: {
        allowClear: true,
        maxlength: 64,
        placeholder: '昵称（profile.nickname）',
      },
      fieldName: 'nickname',
      label: '昵称',
    },
    {
      component: 'Input',
      componentProps: {
        allowClear: true,
        maxlength: 64,
        placeholder: '名（profile.given_name）',
      },
      fieldName: 'givenName',
      label: '名（Given Name）',
    },
    {
      component: 'Input',
      componentProps: {
        allowClear: true,
        maxlength: 64,
        placeholder: '姓（profile.family_name）',
      },
      fieldName: 'familyName',
      label: '姓（Family Name）',
    },
    {
      component: 'Input',
      componentProps: {
        allowClear: true,
        maxlength: 64,
        placeholder: '偏好用户名（profile.preferred_username）',
      },
      fieldName: 'preferredUsername',
      label: '偏好用户名',
    },
    {
      component: 'Input',
      componentProps: {
        allowClear: true,
        maxlength: 512,
        placeholder:
          'https://example.com/avatar.png（纯 URL 文本，不上传文件）',
      },
      fieldName: 'picture',
      label: '头像 URL',
      rules: z
        .string()
        .trim()
        .refine(
          (val) => val === '' || /^https?:\/\/\S+$/i.test(val),
          '请输入以 http:// 或 https:// 开头的有效头像 URL',
        ),
    },
    {
      component: 'Input',
      componentProps: {
        allowClear: true,
        maxlength: 32,
        placeholder: '例如 zh-CN、en-US',
      },
      fieldName: 'locale',
      label: '语言区域（Locale）',
    },
  ];
}

export function getCreateUserFormSchema(): VbenFormSchema[] {
  return [
    {
      component: 'Input',
      componentProps: {
        autocomplete: 'off',
        placeholder: 'user@example.com',
      },
      fieldName: 'email',
      label: '登录邮箱',
      rules: z.email('请输入格式有效的邮箱地址'),
    },
    {
      component: 'InputPassword',
      componentProps: {
        autocomplete: 'new-password',
        placeholder: '请输入 8–128 位初始密码',
      },
      fieldName: 'password',
      label: '初始密码',
      rules: z
        .string()
        .min(8, '初始密码长度至少为 8 位')
        .max(128, '初始密码长度不能超过 128 位'),
    },
    {
      component: 'Input',
      componentProps: {
        allowClear: true,
        maxlength: 128,
        placeholder: '选填，显示姓名',
      },
      fieldName: 'name',
      label: '显示姓名',
    },
    {
      component: 'Input',
      componentProps: {
        allowClear: true,
        maxlength: 64,
        placeholder: '选填，昵称',
      },
      fieldName: 'nickname',
      label: '昵称',
    },
    {
      component: 'Checkbox',
      defaultValue: true,
      fieldName: 'requirePasswordChange',
      label: '首次登录策略',
      renderComponentContent: () => ({
        default: () => '要求该用户首次登录 IAM 时修改初始密码',
      }),
    },
  ];
}

export function getSetPasswordFormSchema(): VbenFormSchema[] {
  return [
    {
      component: 'InputPassword',
      componentProps: {
        autocomplete: 'new-password',
        placeholder: '请输入 8–128 位新密码',
      },
      fieldName: 'password',
      label: '新登录密码',
      rules: z
        .string()
        .min(8, '新密码长度至少为 8 位')
        .max(128, '新密码长度不能超过 128 位'),
    },
    {
      component: 'InputPassword',
      componentProps: {
        autocomplete: 'new-password',
        placeholder: '请再次输入新密码',
      },
      dependencies: {
        rules(values) {
          return z
            .string()
            .min(1, '请再次输入新密码')
            .refine((val) => val === values.password, '两次输入的新密码不一致');
        },
        triggerFields: ['password'],
      },
      fieldName: 'confirmPassword',
      label: '确认新密码',
    },
  ];
}
