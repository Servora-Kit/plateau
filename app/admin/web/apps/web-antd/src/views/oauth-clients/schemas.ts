import type {
  CreateOAuthClientRequest,
  ListOAuthClientsRequest,
  UpdateOAuthClientRequest,
} from '@plateau/api/plateau/admin/client/v1';
import type {
  ApplicationType,
  ClientSource,
  ClientStatus,
  OAuthClient,
  OAuthClientSecret,
  TokenEndpointAuthMethod,
} from '@plateau/api/plateau/iam/client/v1';
import type { FilterExpression } from '@servora/plateau-utils/crud';

import type { VbenFormSchema } from '#/adapter/form';
import type { VxeTableGridOptions } from '#/adapter/vxe-table';

import {
  OAuthClientFields,
  OAuthClientName,
  OAuthClientUpdateFields,
} from '@plateau/api/plateau/iam/client/v1/client.crud';
import {
  buildFilter,
  buildOrderBy,
  filterContains,
  makeUpdateMask,
  rawFilterValue,
} from '@servora/plateau-utils/crud';

export const MAX_OAUTH_CLIENT_SECRETS = 5;

export const CLIENT_ID_PATTERN = /^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$/;

export type OAuthClientPresetKey = 'm2m' | 'native' | 'spa' | 'traditional_web';

export type OAuthClientSortOption =
  | 'create_time_asc'
  | 'create_time_desc'
  | 'update_time_asc'
  | 'update_time_desc';

export interface OAuthClientsSearchFormValues {
  applicationType?: '' | ApplicationType;
  keyword?: string;
  orderBy?: OAuthClientSortOption;
  source?: '' | ClientSource;
  status?: '' | ClientStatus;
}

export interface CursorPaginationState {
  currentPageIndex: number;
  cursorStack: string[];
  nextPageToken: string;
  pageSize: number;
  totalSize: number;
}

export interface OAuthClientPresetDefinition {
  allowedGrantTypes: readonly string[];
  allowedScopes: readonly string[];
  applicationType: ApplicationType;
  badgeText: string;
  description: string;
  key: OAuthClientPresetKey;
  title: string;
  tokenEndpointAuthMethod: TokenEndpointAuthMethod;
  trusted: boolean;
}

export interface CreateOAuthClientFormValues {
  allowedGrantTypes: string[];
  allowedScopes: string[];
  applicationType: ApplicationType;
  audiencesText: string;
  description: string;
  displayName: string;
  initialSecretName: string;
  logoUri: string;
  oauthClientId: string;
  postLogoutRedirectUrisText: string;
  preset: OAuthClientPresetKey;
  redirectUrisText: string;
  tokenEndpointAuthMethod: TokenEndpointAuthMethod;
  trusted: boolean;
}

export interface EditOAuthClientFormValues {
  allowedGrantTypes: string[];
  allowedScopes: string[];
  applicationType: ApplicationType;
  audiencesText: string;
  description: string;
  displayName: string;
  logoUri: string;
  postLogoutRedirectUrisText: string;
  redirectUrisText: string;
  tokenEndpointAuthMethod: TokenEndpointAuthMethod;
  trusted: boolean;
}

export type OAuthClientFormErrors = Partial<
  Record<
    | 'allowedGrantTypes'
    | 'allowedScopes'
    | 'audiencesText'
    | 'displayName'
    | 'logoUri'
    | 'oauthClientId'
    | 'postLogoutRedirectUrisText'
    | 'redirectUrisText'
    | 'tokenEndpointAuthMethod',
    string
  >
>;

export interface SecretDeletionState {
  deletable: boolean;
  reason?: string;
}

export const STANDARD_OIDC_SCOPES = [
  {
    description: '返回用户唯一身份标识（sub）与 ID Token',
    label: 'openid（基础身份）',
    value: 'openid',
  },
  {
    description: '读取用户显示姓名、昵称与头像等标准资料',
    label: 'profile（个人资料）',
    value: 'profile',
  },
  {
    description: '读取用户登录邮箱及邮箱验证状态',
    label: 'email（电子邮箱）',
    value: 'email',
  },
  {
    description: '支持签发与轮换刷新令牌（Refresh Token）',
    label: 'offline_access（离线访问）',
    value: 'offline_access',
  },
] as const;

const DEFAULT_OAUTH_CLIENT_PRESET: OAuthClientPresetDefinition = {
  allowedGrantTypes: ['authorization_code', 'refresh_token'],
  allowedScopes: ['openid', 'profile', 'email', 'offline_access'],
  applicationType: 'APPLICATION_TYPE_WEB',
  badgeText: '机密客户端 · Basic',
  description:
    '有后端服务的传统 Web 或 BFF 应用，在服务端持有客户端密钥完成授权码换取。',
  key: 'traditional_web',
  title: '传统 Web 应用',
  tokenEndpointAuthMethod: 'TOKEN_ENDPOINT_AUTH_METHOD_CLIENT_SECRET_BASIC',
  trusted: true,
};

export const OAUTH_CLIENT_PRESETS: readonly OAuthClientPresetDefinition[] = [
  DEFAULT_OAUTH_CLIENT_PRESET,
  {
    allowedGrantTypes: ['authorization_code', 'refresh_token'],
    allowedScopes: ['openid', 'profile', 'email', 'offline_access'],
    applicationType: 'APPLICATION_TYPE_USER_AGENT',
    badgeText: '公共客户端 · PKCE',
    description:
      '运行在浏览器中的纯前端单页应用，无客户端密钥，强制基于 PKCE S256 保护授权码。',
    key: 'spa',
    title: '单页应用 (SPA)',
    tokenEndpointAuthMethod: 'TOKEN_ENDPOINT_AUTH_METHOD_NONE',
    trusted: true,
  },
  {
    allowedGrantTypes: ['authorization_code', 'refresh_token'],
    allowedScopes: ['openid', 'profile', 'email', 'offline_access'],
    applicationType: 'APPLICATION_TYPE_NATIVE',
    badgeText: '公共客户端 · RFC 8252',
    description:
      '桌面客户端、移动应用或命令行工具，支持自定义 URI Scheme 与本地回环动态端口。',
    key: 'native',
    title: '原生 / CLI 应用',
    tokenEndpointAuthMethod: 'TOKEN_ENDPOINT_AUTH_METHOD_NONE',
    trusted: true,
  },
  {
    allowedGrantTypes: ['client_credentials'],
    allowedScopes: [],
    applicationType: 'APPLICATION_TYPE_WEB',
    badgeText: '服务端凭据 · M2M',
    description:
      '无人类用户参与的后端微服务，通过客户端密钥直接签发面向目标受众的访问令牌。',
    key: 'm2m',
    title: '机器间服务 (M2M)',
    tokenEndpointAuthMethod: 'TOKEN_ENDPOINT_AUTH_METHOD_CLIENT_SECRET_BASIC',
    trusted: false,
  },
] as const;

const VALID_FILTER_SOURCES: Partial<Record<ClientSource, true>> = {
  CLIENT_SOURCE_DYNAMIC: true,
  CLIENT_SOURCE_STATIC: true,
};

const VALID_FILTER_STATUSES: Partial<Record<ClientStatus, true>> = {
  CLIENT_STATUS_ACTIVE: true,
  CLIENT_STATUS_DISABLED: true,
};

const VALID_FILTER_APPLICATION_TYPES: Partial<Record<ApplicationType, true>> = {
  APPLICATION_TYPE_NATIVE: true,
  APPLICATION_TYPE_USER_AGENT: true,
  APPLICATION_TYPE_WEB: true,
};

const DISALLOWED_NATIVE_SCHEMES = new Set([
  'about:',
  'blob:',
  'data:',
  'file:',
  'javascript:',
  'vbscript:',
]);

export function resolveOAuthClientResourceName(
  client: Pick<OAuthClient, 'clientId' | 'name'>,
): string {
  if (client.name?.trim()) {
    return client.name.trim();
  }
  return OAuthClientName.format({ oauth_client: client.clientId ?? '' });
}

export function resolveOAuthClientDisplayName(
  client?: null | Pick<OAuthClient, 'clientId' | 'displayName'>,
): string {
  return (
    client?.displayName?.trim() || client?.clientId?.trim() || '未命名客户端'
  );
}

export function isStaticOAuthClient(
  client?: null | Pick<OAuthClient, 'source'>,
): boolean {
  return client?.source === 'CLIENT_SOURCE_STATIC';
}

export function isConfidentialAuthMethod(
  method?: null | TokenEndpointAuthMethod,
): boolean {
  return (
    method === 'TOKEN_ENDPOINT_AUTH_METHOD_CLIENT_SECRET_BASIC' ||
    method === 'TOKEN_ENDPOINT_AUTH_METHOD_CLIENT_SECRET_POST'
  );
}

export function isConfidentialOAuthClient(
  client?: null | Pick<OAuthClient, 'tokenEndpointAuthMethod'>,
): boolean {
  return isConfidentialAuthMethod(client?.tokenEndpointAuthMethod);
}

export function getPresetDefinition(
  key: OAuthClientPresetKey,
): OAuthClientPresetDefinition {
  const found = OAUTH_CLIENT_PRESETS.find((item) => item.key === key);
  return found ?? DEFAULT_OAUTH_CLIENT_PRESET;
}

export function createInitialCreateFormValues(
  presetKey: OAuthClientPresetKey = 'traditional_web',
): CreateOAuthClientFormValues {
  const preset = getPresetDefinition(presetKey);
  return {
    allowedGrantTypes: [...preset.allowedGrantTypes],
    allowedScopes: [...preset.allowedScopes],
    applicationType: preset.applicationType,
    audiencesText: '',
    description: '',
    displayName: '',
    initialSecretName: '',
    logoUri: '',
    oauthClientId: '',
    postLogoutRedirectUrisText: '',
    preset: preset.key,
    redirectUrisText: '',
    tokenEndpointAuthMethod: preset.tokenEndpointAuthMethod,
    trusted: preset.trusted,
  };
}

export function applyPresetToCreateForm(
  current: CreateOAuthClientFormValues,
  presetKey: OAuthClientPresetKey,
): CreateOAuthClientFormValues {
  const preset = getPresetDefinition(presetKey);
  return {
    ...current,
    allowedGrantTypes: [...preset.allowedGrantTypes],
    allowedScopes: [...preset.allowedScopes],
    applicationType: preset.applicationType,
    preset: preset.key,
    tokenEndpointAuthMethod: preset.tokenEndpointAuthMethod,
    trusted: preset.trusted,
  };
}

export function getAuthMethodOptions(applicationType: ApplicationType): Array<{
  disabled: boolean;
  label: string;
  value: TokenEndpointAuthMethod;
}> {
  const isSpa = applicationType === 'APPLICATION_TYPE_USER_AGENT';
  return [
    {
      disabled: isSpa,
      label: 'HTTP Basic 认证（client_secret_basic）',
      value: 'TOKEN_ENDPOINT_AUTH_METHOD_CLIENT_SECRET_BASIC',
    },
    {
      disabled: isSpa,
      label: '表单请求体认证（client_secret_post）',
      value: 'TOKEN_ENDPOINT_AUTH_METHOD_CLIENT_SECRET_POST',
    },
    {
      disabled: false,
      label: '无密钥公共客户端（none + PKCE S256）',
      value: 'TOKEN_ENDPOINT_AUTH_METHOD_NONE',
    },
  ];
}

export function getGrantTypeOptions(
  applicationType: ApplicationType,
  authMethod: TokenEndpointAuthMethod,
): Array<{
  disabled: boolean;
  label: string;
  value: string;
}> {
  const allowClientCredentials =
    applicationType === 'APPLICATION_TYPE_WEB' &&
    isConfidentialAuthMethod(authMethod);
  return [
    {
      disabled: false,
      label: '授权码模式（authorization_code）',
      value: 'authorization_code',
    },
    {
      disabled: false,
      label: '刷新令牌（refresh_token，需搭配授权码模式）',
      value: 'refresh_token',
    },
    {
      disabled: !allowClientCredentials,
      label: '客户端凭据（client_credentials，仅机密 Web 服务）',
      value: 'client_credentials',
    },
  ];
}

export function toggleGrantTypeSelection(
  currentGrantTypes: ReadonlyArray<string>,
  grantType: string,
  checked: boolean,
): string[] {
  const set = new Set(currentGrantTypes);
  if (checked) {
    set.add(grantType);
    if (grantType === 'refresh_token') {
      set.add('authorization_code');
    }
  } else {
    set.delete(grantType);
    if (grantType === 'authorization_code') {
      set.delete('refresh_token');
    }
  }
  const ordered: string[] = [];
  for (const candidate of [
    'authorization_code',
    'refresh_token',
    'client_credentials',
  ]) {
    if (set.has(candidate)) {
      ordered.push(candidate);
    }
  }
  return ordered;
}

export function normalizeAuthMethodChange<
  T extends {
    allowedGrantTypes: string[];
    allowedScopes: string[];
    applicationType: ApplicationType;
    tokenEndpointAuthMethod: TokenEndpointAuthMethod;
  },
>(form: T, nextMethod: TokenEndpointAuthMethod): T {
  const effectiveMethod: TokenEndpointAuthMethod =
    form.applicationType === 'APPLICATION_TYPE_USER_AGENT'
      ? 'TOKEN_ENDPOINT_AUTH_METHOD_NONE'
      : nextMethod;

  const canKeepClientCredentials =
    form.applicationType === 'APPLICATION_TYPE_WEB' &&
    isConfidentialAuthMethod(effectiveMethod);

  const nextGrants = canKeepClientCredentials
    ? [...form.allowedGrantTypes]
    : form.allowedGrantTypes.filter((g) => g !== 'client_credentials');

  const nextScopes =
    nextGrants.includes('authorization_code') &&
    !form.allowedScopes.includes('openid')
      ? ['openid', ...form.allowedScopes]
      : [...form.allowedScopes];

  return {
    ...form,
    allowedGrantTypes: nextGrants,
    allowedScopes: nextScopes,
    tokenEndpointAuthMethod: effectiveMethod,
  };
}

export function parseMultiLineValues(raw?: null | string): string[] {
  if (!raw) {
    return [];
  }
  const seen = new Set<string>();
  const result: string[] = [];
  for (const line of raw.split(/\r?\n/)) {
    const trimmed = line.trim();
    if (trimmed && !seen.has(trimmed)) {
      seen.add(trimmed);
      result.push(trimmed);
    }
  }
  return result;
}

export function formatMultiLineValues(
  values?: null | ReadonlyArray<string>,
): string {
  if (!values || values.length === 0) {
    return '';
  }
  return values
    .map((item) => item.trim())
    .filter(Boolean)
    .join('\n');
}

export function isLoopbackHost(hostname: string): boolean {
  const lower = hostname.trim().toLowerCase();
  if (!lower) {
    return false;
  }
  if (
    lower === 'localhost' ||
    lower.endsWith('.localhost') ||
    lower === '[::1]' ||
    lower === '::1'
  ) {
    return true;
  }
  return /^127(?:\.(?:25[0-5]|2[0-4]\d|1?\d?\d)){3}$/.test(lower);
}

export function validateCallbackUri(
  rawUri: string,
  applicationType: ApplicationType,
): null | string {
  const uri = rawUri.trim();
  if (!uri) {
    return 'URI 不能为空';
  }
  if (uri.includes('*')) {
    return `地址 "${uri}" 不能包含通配符 *`;
  }
  if (uri.includes('#')) {
    return `地址 "${uri}" 不能包含 #fragment 片段`;
  }

  let parsed: URL;
  try {
    parsed = new URL(uri);
  } catch {
    return `地址 "${uri}" 不是合法的完整 URI`;
  }

  const protocol = parsed.protocol.toLowerCase();
  if (DISALLOWED_NATIVE_SCHEMES.has(protocol)) {
    return `地址 "${uri}" 使用了禁止的协议头 ${protocol}`;
  }

  if (protocol === 'https:') {
    if (!parsed.hostname) {
      return `地址 "${uri}" 缺少有效主机名`;
    }
    return null;
  }

  if (protocol === 'http:') {
    if (!isLoopbackHost(parsed.hostname)) {
      return `非回环地址 "${uri}" 禁止使用明文 http://，请使用 https:// 或本地回环地址`;
    }
    return null;
  }

  if (applicationType === 'APPLICATION_TYPE_NATIVE') {
    if (
      !/^[a-z][a-z0-9+.-]*:$/.test(protocol) ||
      uri.length <= protocol.length
    ) {
      return `地址 "${uri}" 的自定义 Scheme 格式不合法`;
    }
    return null;
  }

  return `Web 与 SPA 客户端仅支持 https:// 或本地回环 http:// 地址（当前：${uri}）`;
}

export function normalizeSafeHttpImageUrl(raw?: null | string): string {
  const trimmed = raw?.trim() ?? '';
  if (!trimmed || trimmed.includes('*') || trimmed.includes('#')) {
    return '';
  }
  try {
    const parsed = new URL(trimmed);
    const protocol = parsed.protocol.toLowerCase();
    if (protocol === 'https:' && parsed.hostname) {
      return trimmed;
    }
    if (protocol === 'http:' && isLoopbackHost(parsed.hostname)) {
      return trimmed;
    }
    return '';
  } catch {
    return '';
  }
}

export function validateLogoUri(raw: string): null | string {
  const uri = raw.trim();
  if (!uri) {
    return null;
  }
  if (uri.includes('*') || uri.includes('#')) {
    return '图标 URL 不能包含通配符 * 或 #fragment 片段';
  }
  let parsed: URL;
  try {
    parsed = new URL(uri);
  } catch {
    return '请输入合法的完整图标 URL（如 https://example.com/logo.png）';
  }
  const protocol = parsed.protocol.toLowerCase();
  if (protocol === 'https:' && parsed.hostname) {
    return null;
  }
  if (protocol === 'http:' && isLoopbackHost(parsed.hostname)) {
    return null;
  }
  return '图标 URL 必须使用 https://（仅本地回环地址允许使用 http://）';
}

export function validateProtocolFormFields(input: {
  allowedGrantTypes: ReadonlyArray<string>;
  allowedScopes: ReadonlyArray<string>;
  applicationType: ApplicationType;
  audiencesText: string;
  displayName: string;
  isCreate?: boolean;
  logoUri: string;
  oauthClientId?: string;
  postLogoutRedirectUrisText: string;
  redirectUrisText: string;
  tokenEndpointAuthMethod: TokenEndpointAuthMethod;
}): OAuthClientFormErrors {
  const errors: OAuthClientFormErrors = {};

  const trimmedName = input.displayName.trim();
  if (!trimmedName) {
    errors.displayName = '请输入客户端显示名称';
  } else if (trimmedName.length > 128) {
    errors.displayName = '客户端显示名称长度不能超过 128 个字符';
  }

  if (input.isCreate) {
    const trimmedId = input.oauthClientId?.trim() ?? '';
    if (trimmedId && !CLIENT_ID_PATTERN.test(trimmedId)) {
      errors.oauthClientId =
        '仅允许小写字母、数字和连字符，长度 1–63 位，且首尾必须为字母或数字';
    }
  }

  const logoErr = validateLogoUri(input.logoUri);
  if (logoErr) {
    errors.logoUri = logoErr;
  }

  if (
    input.applicationType === 'APPLICATION_TYPE_USER_AGENT' &&
    input.tokenEndpointAuthMethod !== 'TOKEN_ENDPOINT_AUTH_METHOD_NONE'
  ) {
    errors.tokenEndpointAuthMethod =
      '单页应用（USER_AGENT）仅允许无密钥公共客户端认证方式（NONE）';
  }

  if (input.allowedGrantTypes.length === 0) {
    errors.allowedGrantTypes = '请至少选择一种允许的授权类型';
  } else if (
    input.allowedGrantTypes.includes('refresh_token') &&
    !input.allowedGrantTypes.includes('authorization_code')
  ) {
    errors.allowedGrantTypes =
      '启用刷新令牌（refresh_token）时必须同时勾选授权码模式（authorization_code）';
  } else if (input.allowedGrantTypes.includes('client_credentials')) {
    if (!isConfidentialAuthMethod(input.tokenEndpointAuthMethod)) {
      errors.allowedGrantTypes =
        '无密钥公共客户端（NONE）禁止授予客户端凭据模式（client_credentials）';
    } else if (input.applicationType !== 'APPLICATION_TYPE_WEB') {
      errors.allowedGrantTypes =
        '仅 Web / 服务端类型客户端支持授予客户端凭据模式（client_credentials）';
    }
  }

  const hasAuthCode = input.allowedGrantTypes.includes('authorization_code');
  const hasClientCredentials =
    input.allowedGrantTypes.includes('client_credentials');

  const redirectUris = parseMultiLineValues(input.redirectUrisText);
  if (hasAuthCode && redirectUris.length === 0) {
    errors.redirectUrisText =
      '启用授权码模式时至少需要配置 1 个登录回调地址（redirect_uris）';
  } else {
    for (const uri of redirectUris) {
      const uriErr = validateCallbackUri(uri, input.applicationType);
      if (uriErr) {
        errors.redirectUrisText = uriErr;
        break;
      }
    }
  }

  const postLogoutUris = parseMultiLineValues(input.postLogoutRedirectUrisText);
  for (const uri of postLogoutUris) {
    const uriErr = validateCallbackUri(uri, input.applicationType);
    if (uriErr) {
      errors.postLogoutRedirectUrisText = uriErr;
      break;
    }
  }

  if (hasAuthCode && !input.allowedScopes.includes('openid')) {
    errors.allowedScopes = '启用授权码模式时允许的 Scopes 必须包含 openid';
  }

  const audiences = parseMultiLineValues(input.audiencesText);
  if (hasClientCredentials && audiences.length === 0) {
    errors.audiencesText =
      '启用客户端凭据模式（client_credentials）时必须至少填写 1 个目标服务受众（Audience）';
  }

  return errors;
}

function canonicalizeScopes(
  scopes: ReadonlyArray<string>,
  ensureOpenid: boolean,
): string[] {
  const seen = new Set<string>();
  const ordered: string[] = [];
  if (ensureOpenid) {
    seen.add('openid');
    ordered.push('openid');
  }
  for (const scope of scopes) {
    const clean = scope.trim();
    if (clean && !seen.has(clean)) {
      seen.add(clean);
      ordered.push(clean);
    }
  }
  return ordered;
}

export function buildCreateOAuthClientRequest(
  values: CreateOAuthClientFormValues,
): CreateOAuthClientRequest {
  const hasAuthCode = values.allowedGrantTypes.includes('authorization_code');
  const hasClientCredentials =
    values.allowedGrantTypes.includes('client_credentials');
  const confidential = isConfidentialAuthMethod(values.tokenEndpointAuthMethod);

  const trimmedClientId = values.oauthClientId.trim();
  const trimmedDisplayName = values.displayName.trim();
  const trimmedDescription = values.description.trim();
  const trimmedLogoUri = values.logoUri.trim();
  const trimmedSecretName = values.initialSecretName.trim();

  const redirectUris = hasAuthCode
    ? parseMultiLineValues(values.redirectUrisText)
    : [];
  const postLogoutRedirectUris = hasAuthCode
    ? parseMultiLineValues(values.postLogoutRedirectUrisText)
    : [];
  const allowedScopes = hasAuthCode
    ? canonicalizeScopes(values.allowedScopes, true)
    : canonicalizeScopes(values.allowedScopes, false);
  const audiences = hasClientCredentials
    ? parseMultiLineValues(values.audiencesText)
    : [];

  return {
    ...(confidential && trimmedSecretName
      ? { initialSecretName: trimmedSecretName }
      : {}),
    oauthClient: {
      allowedGrantTypes: [...values.allowedGrantTypes],
      ...(allowedScopes.length > 0 ? { allowedScopes } : {}),
      applicationType: values.applicationType,
      ...(audiences.length > 0 ? { audiences } : {}),
      ...(trimmedDescription ? { description: trimmedDescription } : {}),
      displayName: trimmedDisplayName,
      ...(trimmedLogoUri ? { logoUri: trimmedLogoUri } : {}),
      ...(postLogoutRedirectUris.length > 0 ? { postLogoutRedirectUris } : {}),
      ...(redirectUris.length > 0 ? { redirectUris } : {}),
      tokenEndpointAuthMethod: values.tokenEndpointAuthMethod,
      trusted: hasAuthCode && Boolean(values.trusted),
    },
    ...(trimmedClientId ? { oauthClientId: trimmedClientId } : {}),
  };
}

export function extractEditOAuthClientFormValues(
  client?: null | OAuthClient,
): EditOAuthClientFormValues {
  const applicationType: ApplicationType =
    client?.applicationType &&
    client.applicationType !== 'APPLICATION_TYPE_UNSPECIFIED'
      ? client.applicationType
      : 'APPLICATION_TYPE_WEB';
  const tokenEndpointAuthMethod: TokenEndpointAuthMethod =
    client?.tokenEndpointAuthMethod &&
    client.tokenEndpointAuthMethod !== 'TOKEN_ENDPOINT_AUTH_METHOD_UNSPECIFIED'
      ? client.tokenEndpointAuthMethod
      : 'TOKEN_ENDPOINT_AUTH_METHOD_CLIENT_SECRET_BASIC';

  return {
    allowedGrantTypes: [...(client?.allowedGrantTypes ?? [])],
    allowedScopes: [...(client?.allowedScopes ?? [])],
    applicationType,
    audiencesText: formatMultiLineValues(client?.audiences),
    description: client?.description ?? '',
    displayName: client?.displayName ?? '',
    logoUri: client?.logoUri ?? '',
    postLogoutRedirectUrisText: formatMultiLineValues(
      client?.postLogoutRedirectUris,
    ),
    redirectUrisText: formatMultiLineValues(client?.redirectUris),
    tokenEndpointAuthMethod,
    trusted: Boolean(client?.trusted),
  };
}

function areStringArraysEqual(
  left: ReadonlyArray<string>,
  right: ReadonlyArray<string>,
): boolean {
  if (left.length !== right.length) {
    return false;
  }
  return left.every((item, idx) => item === right[idx]);
}

export function buildUpdateOAuthClientRequest(
  resourceName: string,
  etag: string,
  initialValues: EditOAuthClientFormValues,
  nextValues: EditOAuthClientFormValues,
): null | UpdateOAuthClientRequest {
  const maskPatch: Partial<
    Record<keyof typeof OAuthClientUpdateFields, unknown>
  > = {};
  const nextClient: OAuthClient = {
    etag,
    name: resourceName,
  };
  let changedCount = 0;

  const beforeDisplayName = initialValues.displayName.trim();
  const afterDisplayName = nextValues.displayName.trim();
  if (beforeDisplayName !== afterDisplayName) {
    changedCount += 1;
    maskPatch.displayName = afterDisplayName;
    nextClient.displayName = afterDisplayName;
  }

  const beforeDescription = initialValues.description.trim();
  const afterDescription = nextValues.description.trim();
  if (beforeDescription !== afterDescription) {
    changedCount += 1;
    if (afterDescription === '') {
      maskPatch.description = undefined;
    } else {
      maskPatch.description = afterDescription;
      nextClient.description = afterDescription;
    }
  }

  const beforeLogo = initialValues.logoUri.trim();
  const afterLogo = nextValues.logoUri.trim();
  if (beforeLogo !== afterLogo) {
    changedCount += 1;
    if (afterLogo === '') {
      maskPatch.logoUri = undefined;
    } else {
      maskPatch.logoUri = afterLogo;
      nextClient.logoUri = afterLogo;
    }
  }

  if (
    initialValues.tokenEndpointAuthMethod !== nextValues.tokenEndpointAuthMethod
  ) {
    changedCount += 1;
    maskPatch.tokenEndpointAuthMethod = nextValues.tokenEndpointAuthMethod;
    nextClient.tokenEndpointAuthMethod = nextValues.tokenEndpointAuthMethod;
  }

  const hasAuthCode =
    nextValues.allowedGrantTypes.includes('authorization_code');
  const hasClientCredentials =
    nextValues.allowedGrantTypes.includes('client_credentials');

  const beforeGrants = [...initialValues.allowedGrantTypes];
  const afterGrants = [...nextValues.allowedGrantTypes];
  if (!areStringArraysEqual(beforeGrants, afterGrants)) {
    changedCount += 1;
    maskPatch.allowedGrantTypes = afterGrants;
    nextClient.allowedGrantTypes = afterGrants;
  }

  const beforeRedirects = parseMultiLineValues(initialValues.redirectUrisText);
  const afterRedirects = hasAuthCode
    ? parseMultiLineValues(nextValues.redirectUrisText)
    : [];
  if (!areStringArraysEqual(beforeRedirects, afterRedirects)) {
    changedCount += 1;
    maskPatch.redirectUris = afterRedirects;
    nextClient.redirectUris = afterRedirects;
  }

  const beforePostLogout = parseMultiLineValues(
    initialValues.postLogoutRedirectUrisText,
  );
  const afterPostLogout = hasAuthCode
    ? parseMultiLineValues(nextValues.postLogoutRedirectUrisText)
    : [];
  if (!areStringArraysEqual(beforePostLogout, afterPostLogout)) {
    changedCount += 1;
    maskPatch.postLogoutRedirectUris = afterPostLogout;
    nextClient.postLogoutRedirectUris = afterPostLogout;
  }

  const beforeScopes = canonicalizeScopes(
    initialValues.allowedScopes,
    initialValues.allowedGrantTypes.includes('authorization_code'),
  );
  const afterScopes = hasAuthCode
    ? canonicalizeScopes(nextValues.allowedScopes, true)
    : [];
  if (!areStringArraysEqual(beforeScopes, afterScopes)) {
    changedCount += 1;
    maskPatch.allowedScopes = afterScopes;
    nextClient.allowedScopes = afterScopes;
  }

  const beforeAudiences = parseMultiLineValues(initialValues.audiencesText);
  const afterAudiences = hasClientCredentials
    ? parseMultiLineValues(nextValues.audiencesText)
    : [];
  if (!areStringArraysEqual(beforeAudiences, afterAudiences)) {
    changedCount += 1;
    maskPatch.audiences = afterAudiences;
    nextClient.audiences = afterAudiences;
  }

  const beforeTrusted = Boolean(initialValues.trusted);
  const afterTrusted = hasAuthCode && Boolean(nextValues.trusted);
  if (beforeTrusted !== afterTrusted) {
    changedCount += 1;
    maskPatch.trusted = afterTrusted;
    nextClient.trusted = afterTrusted;
  }

  if (changedCount === 0) {
    return null;
  }

  const updateMask = makeUpdateMask(OAuthClientUpdateFields, maskPatch);
  if (!updateMask) {
    return null;
  }

  return {
    oauthClient: nextClient,
    updateMask,
  };
}

export function buildOAuthClientsListRequest(
  formValues: OAuthClientsSearchFormValues,
  pageSize: number,
  pageToken: string,
): ListOAuthClientsRequest {
  type OAuthClientFieldName =
    (typeof OAuthClientFields)[keyof typeof OAuthClientFields];
  const clauses: Array<FilterExpression<OAuthClientFieldName>> = [];

  const trimmedKeyword = formValues.keyword?.trim();
  if (trimmedKeyword) {
    clauses.push({
      or: [
        {
          field: OAuthClientFields.displayName,
          operator: '=',
          value: filterContains(trimmedKeyword),
        },
        {
          field: OAuthClientFields.clientId,
          operator: '=',
          value: filterContains(trimmedKeyword),
        },
      ],
    });
  }

  if (formValues.source && VALID_FILTER_SOURCES[formValues.source]) {
    clauses.push({
      field: OAuthClientFields.source,
      operator: '=',
      value: rawFilterValue(formValues.source),
    });
  }

  if (formValues.status && VALID_FILTER_STATUSES[formValues.status]) {
    clauses.push({
      field: OAuthClientFields.status,
      operator: '=',
      value: rawFilterValue(formValues.status),
    });
  }

  if (
    formValues.applicationType &&
    VALID_FILTER_APPLICATION_TYPES[formValues.applicationType]
  ) {
    clauses.push({
      field: OAuthClientFields.applicationType,
      operator: '=',
      value: rawFilterValue(formValues.applicationType),
    });
  }

  let filter: string | undefined;
  if (clauses.length === 1) {
    filter = buildFilter(OAuthClientFields, clauses[0]);
  } else if (clauses.length > 1) {
    filter = buildFilter(OAuthClientFields, { and: clauses });
  }

  const sortOption = formValues.orderBy || 'create_time_desc';
  let orderBy: string | undefined;
  switch (sortOption) {
    case 'create_time_asc': {
      orderBy = buildOrderBy(OAuthClientFields, [
        { direction: 'asc', field: OAuthClientFields.createTime },
      ]);
      break;
    }
    case 'update_time_asc': {
      orderBy = buildOrderBy(OAuthClientFields, [
        { direction: 'asc', field: OAuthClientFields.updateTime },
      ]);
      break;
    }
    case 'update_time_desc': {
      orderBy = buildOrderBy(OAuthClientFields, [
        { direction: 'desc', field: OAuthClientFields.updateTime },
      ]);
      break;
    }
    default: {
      orderBy = buildOrderBy(OAuthClientFields, [
        { direction: 'desc', field: OAuthClientFields.createTime },
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

export function getSourceBadge(source?: ClientSource): {
  color: string;
  text: string;
} {
  switch (source) {
    case 'CLIENT_SOURCE_DYNAMIC': {
      return { color: 'processing', text: '动态创建' };
    }
    case 'CLIENT_SOURCE_STATIC': {
      return { color: 'purple', text: '系统静态' };
    }
    default: {
      return { color: 'default', text: '未知来源' };
    }
  }
}

export function getStatusBadge(status?: ClientStatus): {
  color: string;
  text: string;
} {
  switch (status) {
    case 'CLIENT_STATUS_ACTIVE': {
      return { color: 'success', text: '启用' };
    }
    case 'CLIENT_STATUS_DISABLED': {
      return { color: 'error', text: '已禁用' };
    }
    default: {
      return { color: 'default', text: '未知' };
    }
  }
}

export function getAuthMethodShortLabel(
  method?: TokenEndpointAuthMethod,
): string {
  switch (method) {
    case 'TOKEN_ENDPOINT_AUTH_METHOD_CLIENT_SECRET_BASIC': {
      return 'Basic 密钥';
    }
    case 'TOKEN_ENDPOINT_AUTH_METHOD_CLIENT_SECRET_POST': {
      return 'Post 密钥';
    }
    case 'TOKEN_ENDPOINT_AUTH_METHOD_NONE': {
      return '公共 PKCE';
    }
    default: {
      return '未知认证';
    }
  }
}

export function getClientArchitectureBadge(
  client: Pick<
    OAuthClient,
    'allowedGrantTypes' | 'applicationType' | 'tokenEndpointAuthMethod'
  >,
): {
  authLabel: string;
  color: string;
  modeLabel: string;
} {
  const grants = client.allowedGrantTypes ?? [];
  const hasAuthCode = grants.includes('authorization_code');
  const hasM2m = grants.includes('client_credentials');
  const authLabel = getAuthMethodShortLabel(client.tokenEndpointAuthMethod);

  switch (client.applicationType) {
    case 'APPLICATION_TYPE_NATIVE': {
      return {
        authLabel,
        color: 'orange',
        modeLabel: '原生应用',
      };
    }
    case 'APPLICATION_TYPE_USER_AGENT': {
      return {
        authLabel,
        color: 'cyan',
        modeLabel: 'SPA 单页应用',
      };
    }
    case 'APPLICATION_TYPE_WEB': {
      if (hasM2m && !hasAuthCode) {
        return {
          authLabel,
          color: 'geekblue',
          modeLabel: 'M2M 服务',
        };
      }
      if (hasM2m && hasAuthCode) {
        return {
          authLabel,
          color: 'blue',
          modeLabel: 'Web 应用（含 M2M）',
        };
      }
      return {
        authLabel,
        color: 'blue',
        modeLabel: 'Web 应用',
      };
    }
    default: {
      return {
        authLabel,
        color: 'default',
        modeLabel: '未指定类型',
      };
    }
  }
}

export function getTrustedPolicyBadge(
  client: Pick<OAuthClient, 'allowedGrantTypes' | 'trusted'>,
): {
  color: string;
  text: string;
} {
  const grants = client.allowedGrantTypes ?? [];
  if (grants.length > 0 && !grants.includes('authorization_code')) {
    return { color: 'default', text: '仅服务端凭据' };
  }
  if (client.trusted) {
    return { color: 'green', text: '第一方免审' };
  }
  return { color: 'gold', text: '第三方需授权' };
}

export function formatSecretPrefixMask(secretPrefix?: null | string): string {
  const clean = secretPrefix?.trim() ?? '';
  if (!clean) {
    return '••••••••';
  }
  if (clean.endsWith('••••') || clean.endsWith('*')) {
    return clean;
  }
  return `${clean}••••`;
}

export function isSecretExpired(
  secret: Pick<OAuthClientSecret, 'expiresTime'>,
  nowMs = Date.now(),
): boolean {
  const raw = secret.expiresTime?.trim();
  if (!raw) {
    return false;
  }
  const ts = new Date(raw).getTime();
  if (Number.isNaN(ts)) {
    return false;
  }
  return ts <= nowMs;
}

export function countActiveSecrets(
  secrets: null | ReadonlyArray<OAuthClientSecret> | undefined,
  nowMs = Date.now(),
): number {
  if (!secrets || secrets.length === 0) {
    return 0;
  }
  let count = 0;
  for (const item of secrets) {
    if (!isSecretExpired(item, nowMs)) {
      count += 1;
    }
  }
  return count;
}

export function resolveSecretDeletionState(
  client: null | Pick<OAuthClient, 'secrets' | 'source'> | undefined,
  targetSecret: OAuthClientSecret,
  canManageClients: boolean,
  nowMs = Date.now(),
): SecretDeletionState {
  if (!canManageClients) {
    return {
      deletable: false,
      reason: '当前账号缺少 manage_clients 权限',
    };
  }
  if (isStaticOAuthClient(client)) {
    return {
      deletable: false,
      reason:
        '该客户端由 IAM 静态配置文件（oidc.yaml）托管，不支持在线删除密钥',
    };
  }

  const allSecrets = client?.secrets ?? [];
  const targetActive = !isSecretExpired(targetSecret, nowMs);
  let remainingActive = 0;
  for (const item of allSecrets) {
    if (
      item.secretId !== targetSecret.secretId &&
      !isSecretExpired(item, nowMs)
    ) {
      remainingActive += 1;
    }
  }

  if ((targetActive && remainingActive < 1) || allSecrets.length <= 1) {
    return {
      deletable: false,
      reason:
        '机密客户端至少需保留一把有效密钥，请先新增新密钥完成轮换后再删除旧密钥',
    };
  }

  return { deletable: true };
}

export function isDeleteClientIdConfirmed(
  inputClientId: string,
  targetClientId?: null | string,
): boolean {
  const expected = targetClientId?.trim() ?? '';
  if (!expected) {
    return false;
  }
  return inputClientId.trim() === expected;
}

export function getOAuthClientsSearchFormSchema(): VbenFormSchema[] {
  return [
    {
      component: 'Input',
      componentProps: {
        allowClear: true,
        placeholder: '搜索客户端名称或 Client ID',
      },
      fieldName: 'keyword',
      label: '关键字',
    },
    {
      component: 'Select',
      componentProps: {
        allowClear: true,
        options: [
          { label: '全部来源', value: '' },
          { label: '系统静态（STATIC）', value: 'CLIENT_SOURCE_STATIC' },
          { label: '动态创建（DYNAMIC）', value: 'CLIENT_SOURCE_DYNAMIC' },
        ],
        placeholder: '全部来源',
      },
      defaultValue: '',
      fieldName: 'source',
      label: '来源',
    },
    {
      component: 'Select',
      componentProps: {
        allowClear: true,
        options: [
          { label: '全部状态', value: '' },
          { label: '启用（ACTIVE）', value: 'CLIENT_STATUS_ACTIVE' },
          { label: '已禁用（DISABLED）', value: 'CLIENT_STATUS_DISABLED' },
        ],
        placeholder: '全部状态',
      },
      defaultValue: '',
      fieldName: 'status',
      label: '状态',
    },
    {
      component: 'Select',
      componentProps: {
        allowClear: true,
        options: [
          { label: '全部类型', value: '' },
          { label: 'Web / 服务端（WEB）', value: 'APPLICATION_TYPE_WEB' },
          {
            label: '单页应用（USER_AGENT）',
            value: 'APPLICATION_TYPE_USER_AGENT',
          },
          { label: '原生 / CLI（NATIVE）', value: 'APPLICATION_TYPE_NATIVE' },
        ],
        placeholder: '全部应用类型',
      },
      defaultValue: '',
      fieldName: 'applicationType',
      label: '应用类型',
    },
    {
      component: 'Select',
      componentProps: {
        options: [
          { label: '创建时间：最新优先', value: 'create_time_desc' },
          { label: '创建时间：最早优先', value: 'create_time_asc' },
          { label: '更新时间：最新优先', value: 'update_time_desc' },
          { label: '更新时间：最早优先', value: 'update_time_asc' },
        ],
      },
      defaultValue: 'create_time_desc',
      fieldName: 'orderBy',
      label: '排序方式',
    },
  ];
}

export type OAuthClientsGridColumns =
  VxeTableGridOptions<OAuthClient>['columns'];

export function getOAuthClientsGridColumns(): OAuthClientsGridColumns {
  return [
    {
      align: 'left',
      field: 'displayName',
      minWidth: 230,
      showOverflow: false,
      slots: { default: 'clientIdentity' },
      title: '客户端应用',
    },
    {
      align: 'left',
      field: 'clientId',
      minWidth: 190,
      showOverflow: false,
      slots: { default: 'clientId' },
      title: 'Client ID',
    },
    {
      field: 'source',
      minWidth: 120,
      showOverflow: false,
      slots: { default: 'source' },
      title: '托管来源',
    },
    {
      align: 'left',
      field: 'applicationType',
      minWidth: 205,
      showOverflow: false,
      slots: { default: 'architecture' },
      title: '架构与认证模式',
    },
    {
      field: 'trusted',
      minWidth: 135,
      showOverflow: false,
      slots: { default: 'trusted' },
      title: '受信策略',
    },
    {
      field: 'status',
      minWidth: 110,
      showOverflow: false,
      slots: { default: 'status' },
      title: '状态',
    },
    {
      field: 'createTime',
      formatter: 'formatDateTime',
      minWidth: 170,
      showOverflow: 'tooltip',
      title: '创建时间',
    },
    {
      align: 'right',
      field: 'actions',
      fixed: 'right',
      minWidth: 260,
      showOverflow: false,
      slots: { default: 'actions' },
      title: '操作',
    },
  ];
}
