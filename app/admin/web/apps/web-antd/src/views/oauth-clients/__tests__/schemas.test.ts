import { describe, expect, it } from 'vitest';

import {
  applyCursorPageResult,
  applyPresetToCreateForm,
  buildCreateOAuthClientRequest,
  buildOAuthClientsListRequest,
  buildUpdateOAuthClientRequest,
  countActiveSecrets,
  createInitialCreateFormValues,
  createInitialCursorState,
  extractEditOAuthClientFormValues,
  formatSecretPrefixMask,
  getAuthMethodOptions,
  getClientArchitectureBadge,
  getGrantTypeOptions,
  getOAuthClientsGridColumns,
  getTrustedPolicyBadge,
  isDeleteClientIdConfirmed,
  isSecretExpired,
  normalizeAuthMethodChange,
  normalizeSafeHttpImageUrl,
  resolveOAuthClientResourceName,
  resolveSecretDeletionState,
  stepCursorNext,
  stepCursorPrev,
  toggleGrantTypeSelection,
  validateCallbackUri,
  validateLogoUri,
  validateProtocolFormFields,
} from '../schemas';

describe('buildOAuthClientsListRequest', () => {
  it('builds AIP-160 filter and orderBy using generated OAuthClientFields', () => {
    const req = buildOAuthClientsListRequest(
      {
        applicationType: 'APPLICATION_TYPE_WEB',
        keyword: ' blog ',
        orderBy: 'update_time_desc',
        source: 'CLIENT_SOURCE_DYNAMIC',
        status: 'CLIENT_STATUS_ACTIVE',
      },
      20,
      'cursor_2',
    );

    expect(req).toEqual({
      filter:
        '((display_name = "*blog*" OR client_id = "*blog*") AND source = CLIENT_SOURCE_DYNAMIC AND status = CLIENT_STATUS_ACTIVE AND application_type = APPLICATION_TYPE_WEB)',
      includeTotal: true,
      orderBy: 'update_time desc',
      pageSize: 20,
      pageToken: 'cursor_2',
    });
  });

  it('defaults to create_time desc and omits filter when search form is empty', () => {
    expect(buildOAuthClientsListRequest({}, 10, '')).toEqual({
      filter: undefined,
      includeTotal: true,
      orderBy: 'create_time desc',
      pageSize: 10,
      pageToken: undefined,
    });
  });
});

describe('preset scenarios and protocol linkage', () => {
  it('initializes all 4 preset templates with expected protocol defaults', () => {
    const web = createInitialCreateFormValues('traditional_web');
    expect(web).toMatchObject({
      allowedGrantTypes: ['authorization_code', 'refresh_token'],
      allowedScopes: ['openid', 'profile', 'email', 'offline_access'],
      applicationType: 'APPLICATION_TYPE_WEB',
      tokenEndpointAuthMethod: 'TOKEN_ENDPOINT_AUTH_METHOD_CLIENT_SECRET_BASIC',
      trusted: true,
    });

    const spa = applyPresetToCreateForm(web, 'spa');
    expect(spa).toMatchObject({
      allowedGrantTypes: ['authorization_code', 'refresh_token'],
      applicationType: 'APPLICATION_TYPE_USER_AGENT',
      tokenEndpointAuthMethod: 'TOKEN_ENDPOINT_AUTH_METHOD_NONE',
      trusted: true,
    });

    const native = applyPresetToCreateForm(spa, 'native');
    expect(native).toMatchObject({
      allowedGrantTypes: ['authorization_code', 'refresh_token'],
      applicationType: 'APPLICATION_TYPE_NATIVE',
      tokenEndpointAuthMethod: 'TOKEN_ENDPOINT_AUTH_METHOD_NONE',
      trusted: true,
    });

    const m2m = applyPresetToCreateForm(native, 'm2m');
    expect(m2m).toMatchObject({
      allowedGrantTypes: ['client_credentials'],
      allowedScopes: [],
      applicationType: 'APPLICATION_TYPE_WEB',
      tokenEndpointAuthMethod: 'TOKEN_ENDPOINT_AUTH_METHOD_CLIENT_SECRET_BASIC',
      trusted: false,
    });
  });

  it('locks SPA auth method to NONE, strips client_credentials on NONE, and links refresh_token with authorization_code', () => {
    const spaAuthOptions = getAuthMethodOptions('APPLICATION_TYPE_USER_AGENT');
    expect(
      spaAuthOptions.find(
        (o) => o.value === 'TOKEN_ENDPOINT_AUTH_METHOD_CLIENT_SECRET_BASIC',
      )?.disabled,
    ).toBe(true);
    expect(
      spaAuthOptions.find((o) => o.value === 'TOKEN_ENDPOINT_AUTH_METHOD_NONE')
        ?.disabled,
    ).toBe(false);

    const publicWebGrants = getGrantTypeOptions(
      'APPLICATION_TYPE_WEB',
      'TOKEN_ENDPOINT_AUTH_METHOD_NONE',
    );
    expect(
      publicWebGrants.find((g) => g.value === 'client_credentials')?.disabled,
    ).toBe(true);

    const m2mSwitchedToNone = normalizeAuthMethodChange(
      createInitialCreateFormValues('m2m'),
      'TOKEN_ENDPOINT_AUTH_METHOD_NONE',
    );
    expect(m2mSwitchedToNone.allowedGrantTypes).toEqual([]);

    expect(toggleGrantTypeSelection([], 'refresh_token', true)).toEqual([
      'authorization_code',
      'refresh_token',
    ]);
    expect(
      toggleGrantTypeSelection(
        ['authorization_code', 'refresh_token', 'client_credentials'],
        'authorization_code',
        false,
      ),
    ).toEqual(['client_credentials']);
  });
});

describe('client_id, callback URI, and protocol form validation', () => {
  it('validates custom client_id slug and callback URIs across WEB, USER_AGENT, and NATIVE', () => {
    expect(
      validateCallbackUri(
        'https://blog.example.com/auth/callback',
        'APPLICATION_TYPE_WEB',
      ),
    ).toBeNull();
    expect(
      validateCallbackUri(
        'http://blog.plateau.localhost:8080/callback',
        'APPLICATION_TYPE_USER_AGENT',
      ),
    ).toBeNull();
    expect(
      validateCallbackUri(
        'http://127.0.0.1:5173/callback',
        'APPLICATION_TYPE_USER_AGENT',
      ),
    ).toBeNull();
    expect(
      validateCallbackUri(
        'http://[::1]:3000/callback',
        'APPLICATION_TYPE_NATIVE',
      ),
    ).toBeNull();
    expect(
      validateCallbackUri('myapp://oauth/callback', 'APPLICATION_TYPE_NATIVE'),
    ).toBeNull();

    expect(
      validateCallbackUri(
        'http://insecure.example.com/callback',
        'APPLICATION_TYPE_WEB',
      ),
    ).toContain('禁止使用明文 http://');
    expect(
      validateCallbackUri(
        'https://blog.example.com/*/callback',
        'APPLICATION_TYPE_WEB',
      ),
    ).toContain('通配符');
    expect(
      validateCallbackUri(
        'https://blog.example.com/callback#frag',
        'APPLICATION_TYPE_WEB',
      ),
    ).toContain('#fragment');
    expect(
      validateCallbackUri('myapp://callback', 'APPLICATION_TYPE_WEB'),
    ).toContain('仅支持 https:// 或本地回环 http://');
    expect(
      validateCallbackUri('javascript:alert(1)', 'APPLICATION_TYPE_NATIVE'),
    ).toContain('禁止的协议头');
    expect(validateCallbackUri('myapp:', 'APPLICATION_TYPE_NATIVE')).toContain(
      '自定义 Scheme',
    );

    expect(validateLogoUri('https://cdn.example.com/logo.png')).toBeNull();
    expect(
      validateLogoUri('http://admin.plateau.localhost:8080/logo.svg'),
    ).toBeNull();
    expect(validateLogoUri('http://cdn.example.com/logo.png')).toContain(
      'https://',
    );
    expect(validateLogoUri('https://cdn.example.com/logo.png#frag')).toContain(
      '#fragment',
    );
    expect(normalizeSafeHttpImageUrl('https://cdn.example.com/logo.png')).toBe(
      'https://cdn.example.com/logo.png',
    );
    expect(normalizeSafeHttpImageUrl('http://cdn.example.com/logo.png')).toBe(
      '',
    );
  });

  it('validates required protocol fields for authorization_code and client_credentials flows', () => {
    const invalidWeb = validateProtocolFormFields({
      ...createInitialCreateFormValues('traditional_web'),
      displayName: '   ',
      isCreate: true,
      logoUri: 'ftp://bad.example/logo.png',
      oauthClientId: '-Invalid_Slug-',
      redirectUrisText: '',
    });
    expect(invalidWeb.displayName).toBeDefined();
    expect(invalidWeb.oauthClientId).toBeDefined();
    expect(invalidWeb.logoUri).toBeDefined();
    expect(invalidWeb.redirectUrisText).toBeDefined();

    const invalidNativeClientCreds = validateProtocolFormFields({
      ...createInitialCreateFormValues('m2m'),
      applicationType: 'APPLICATION_TYPE_NATIVE',
      audiencesText: 'iam',
      displayName: 'Native CLI',
      isCreate: true,
    });
    expect(invalidNativeClientCreds.allowedGrantTypes).toContain('仅 Web');

    const invalidM2m = validateProtocolFormFields({
      ...createInitialCreateFormValues('m2m'),
      audiencesText: '   ',
      displayName: 'Order Worker',
      isCreate: true,
      oauthClientId: 'order-worker',
    });
    expect(invalidM2m.oauthClientId).toBeUndefined();
    expect(invalidM2m.audiencesText).toBeDefined();

    const validM2m = validateProtocolFormFields({
      ...createInitialCreateFormValues('m2m'),
      audiencesText: 'iam\naudit',
      displayName: 'Order Worker',
      isCreate: true,
      oauthClientId: 'a',
    });
    expect(validM2m).toEqual({});
  });
});

describe('buildCreateOAuthClientRequest and buildUpdateOAuthClientRequest', () => {
  it('builds create request cleanly for confidential Web and public SPA', () => {
    const webForm = {
      ...createInitialCreateFormValues('traditional_web'),
      description: '  Blog SSR App  ',
      displayName: '  Plateau Blog  ',
      initialSecretName: '  primary-prod  ',
      logoUri: ' https://cdn.example.com/logo.png ',
      oauthClientId: ' blog-web ',
      postLogoutRedirectUrisText: 'https://blog.example.com/\n',
      redirectUrisText:
        'https://blog.example.com/cb\nhttps://blog.example.com/cb\n',
      trusted: false,
    };

    expect(buildCreateOAuthClientRequest(webForm)).toEqual({
      initialSecretName: 'primary-prod',
      oauthClient: {
        allowedGrantTypes: ['authorization_code', 'refresh_token'],
        allowedScopes: ['openid', 'profile', 'email', 'offline_access'],
        applicationType: 'APPLICATION_TYPE_WEB',
        description: 'Blog SSR App',
        displayName: 'Plateau Blog',
        logoUri: 'https://cdn.example.com/logo.png',
        postLogoutRedirectUris: ['https://blog.example.com/'],
        redirectUris: ['https://blog.example.com/cb'],
        tokenEndpointAuthMethod:
          'TOKEN_ENDPOINT_AUTH_METHOD_CLIENT_SECRET_BASIC',
        trusted: false,
      },
      oauthClientId: 'blog-web',
    });

    const spaForm = {
      ...createInitialCreateFormValues('spa'),
      displayName: 'Admin SPA',
      initialSecretName: 'ignored-for-public',
      redirectUrisText: 'http://localhost:5173/cb',
    };
    const spaReq = buildCreateOAuthClientRequest(spaForm);
    expect(spaReq.initialSecretName).toBeUndefined();
    expect(spaReq.oauthClientId).toBeUndefined();
    expect(spaReq.oauthClient?.trusted).toBe(true);
  });

  it('emits minimal FieldMask for changed and cleared mutable fields on UpdateOAuthClient', () => {
    const initial = extractEditOAuthClientFormValues({
      allowedGrantTypes: ['authorization_code', 'refresh_token'],
      allowedScopes: ['openid', 'profile'],
      applicationType: 'APPLICATION_TYPE_WEB',
      clientId: 'blog-web',
      description: 'Old description',
      displayName: 'Blog',
      logoUri: 'https://cdn.example.com/old.png',
      name: 'oauthClients/blog-web',
      redirectUris: ['https://blog.example.com/cb'],
      tokenEndpointAuthMethod: 'TOKEN_ENDPOINT_AUTH_METHOD_CLIENT_SECRET_BASIC',
      trusted: true,
    });

    expect(
      buildUpdateOAuthClientRequest(
        'oauthClients/blog-web',
        'W/"etag-1"',
        initial,
        { ...initial },
      ),
    ).toBeNull();

    const updated = buildUpdateOAuthClientRequest(
      'oauthClients/blog-web',
      'W/"etag-1"',
      initial,
      {
        ...initial,
        description: '   ', // cleared optional field
        displayName: 'Blog Portal',
        redirectUrisText:
          'https://blog.example.com/cb\nhttps://blog.example.com/cb2',
        trusted: false,
      },
    );

    expect(updated).toEqual({
      oauthClient: {
        displayName: 'Blog Portal',
        etag: 'W/"etag-1"',
        name: 'oauthClients/blog-web',
        redirectUris: [
          'https://blog.example.com/cb',
          'https://blog.example.com/cb2',
        ],
        trusted: false,
      },
      updateMask: 'display_name,description,redirect_uris,trusted',
    });
  });
});

describe('multi-secret rotation rules, badges, and delete confirmation', () => {
  it('enforces static immutability and last-active-secret protection while allowing expired secret cleanup', () => {
    const nowMs = Date.parse('2026-11-28T06:00:00Z');
    const activeSecret = {
      expiresTime: '2026-12-31T00:00:00Z',
      name: 'Current',
      secretId: 'sec_active_1',
      secretPrefix: 'pcs_1111aaaa',
    };
    const expiredSecret = {
      expiresTime: '2026-11-01T00:00:00Z',
      name: 'Old',
      secretId: 'sec_expired_1',
      secretPrefix: 'pcs_0000bbbb',
    };
    const secondActiveSecret = {
      name: 'Rotated',
      secretId: 'sec_active_2',
      secretPrefix: 'pcs_2222cccc',
    };

    expect(formatSecretPrefixMask('pcs_1111aaaa')).toBe('pcs_1111aaaa••••');
    expect(isSecretExpired(activeSecret, nowMs)).toBe(false);
    expect(isSecretExpired(expiredSecret, nowMs)).toBe(true);
    expect(countActiveSecrets([activeSecret, expiredSecret], nowMs)).toBe(1);

    // Static client: never deletable
    expect(
      resolveSecretDeletionState(
        {
          secrets: [activeSecret, secondActiveSecret],
          source: 'CLIENT_SOURCE_STATIC',
        },
        activeSecret,
        true,
        nowMs,
      ),
    ).toMatchObject({ deletable: false });

    // 1 active + 1 expired: active cannot be deleted, expired CAN be deleted
    const oneActiveClient = {
      secrets: [activeSecret, expiredSecret],
      source: 'CLIENT_SOURCE_DYNAMIC' as const,
    };
    const blockLastActive = resolveSecretDeletionState(
      oneActiveClient,
      activeSecret,
      true,
      nowMs,
    );
    expect(blockLastActive.deletable).toBe(false);
    expect(blockLastActive.reason).toContain('至少需保留一把有效密钥');

    expect(
      resolveSecretDeletionState(oneActiveClient, expiredSecret, true, nowMs),
    ).toEqual({ deletable: true });

    // 2 active secrets during zero-downtime rotation: both can be deleted
    expect(
      resolveSecretDeletionState(
        {
          secrets: [activeSecret, secondActiveSecret],
          source: 'CLIENT_SOURCE_DYNAMIC',
        },
        activeSecret,
        true,
        nowMs,
      ),
    ).toEqual({ deletable: true });

    // All secrets expired (e.g., 2 expired secrets, 0 active): expired secret cleanup is still allowed when total > 1
    const secondExpiredSecret = {
      expiresTime: '2026-10-01T00:00:00Z',
      name: 'Older',
      secretId: 'sec_expired_2',
      secretPrefix: 'pcs_9999zzzz',
    };
    expect(
      resolveSecretDeletionState(
        {
          secrets: [expiredSecret, secondExpiredSecret],
          source: 'CLIENT_SOURCE_DYNAMIC',
        },
        expiredSecret,
        true,
        nowMs,
      ),
    ).toEqual({ deletable: true });
  });

  it('resolves resource names, badges, delete confirmation, and cursor pagination accurately', () => {
    expect(resolveOAuthClientResourceName({ clientId: 'blog-web' })).toBe(
      'oauthClients/blog-web',
    );
    expect(
      getClientArchitectureBadge({
        allowedGrantTypes: ['client_credentials'],
        applicationType: 'APPLICATION_TYPE_WEB',
        tokenEndpointAuthMethod:
          'TOKEN_ENDPOINT_AUTH_METHOD_CLIENT_SECRET_BASIC',
      }).modeLabel,
    ).toBe('M2M 服务');
    expect(
      getClientArchitectureBadge({
        allowedGrantTypes: ['authorization_code'],
        applicationType: 'APPLICATION_TYPE_USER_AGENT',
        tokenEndpointAuthMethod: 'TOKEN_ENDPOINT_AUTH_METHOD_NONE',
      }).modeLabel,
    ).toBe('SPA 单页应用');
    expect(
      getTrustedPolicyBadge({
        allowedGrantTypes: ['authorization_code'],
        trusted: false,
      }).text,
    ).toBe('第三方需授权');

    expect(isDeleteClientIdConfirmed(' blog-web ', 'blog-web')).toBe(true);
    expect(isDeleteClientIdConfirmed('BLOG-WEB', 'blog-web')).toBe(false);
    expect(isDeleteClientIdConfirmed('', '')).toBe(false);

    const cols = getOAuthClientsGridColumns() ?? [];
    expect(cols.length).toBeGreaterThanOrEqual(8);

    const p1 = applyCursorPageResult(
      createInitialCursorState(20),
      20,
      'tok_2',
      '21',
    );
    const p2 = stepCursorNext(p1.state);
    if (!p2) {
      throw new Error('expected next cursor state');
    }
    const emptyAfterDelete = applyCursorPageResult(p2, 0, '', '20');
    expect(emptyAfterDelete.fallbackToPreviousPage).toBe(true);
    expect(emptyAfterDelete.state.currentPageIndex).toBe(0);
    expect(stepCursorPrev(emptyAfterDelete.state)).toBeNull();
  });
});
