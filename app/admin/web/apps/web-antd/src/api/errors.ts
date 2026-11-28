import {
  AdminAuditErrorReason,
  isAdminAuditErrorReason,
} from '@plateau/api/plateau/admin/audit/v1/audit.errors';
import {
  AdminClientErrorReason,
  isAdminClientErrorReason,
} from '@plateau/api/plateau/admin/client/v1/client.errors';
import {
  AdminUserErrorReason,
  isAdminUserErrorReason,
} from '@plateau/api/plateau/admin/user/v1/user.errors';
import { isCrudErrorReason } from '@servora/plateau-utils/error-reasons/plateau/crud/v1/errors';
import {
  isSecurityErrorReason,
  SecurityErrorReason,
} from '@servora/plateau-utils/error-reasons/plateau/security/errors/v1/errors';
import { ApiError, parseKratosErrorBody } from '@servora/proto-utils/errors';

export type AdminErrorAction =
  | 'create'
  | 'default'
  | 'delete'
  | 'undelete'
  | 'update';

export type AdminErrorCategory =
  | 'already_exists'
  | 'cancelled'
  | 'email_failed'
  | 'etag_mismatch'
  | 'forbidden'
  | 'invalid_argument'
  | 'invalid_state'
  | 'network'
  | 'not_found'
  | 'self_protected'
  | 'static_immutable'
  | 'target_admin'
  | 'timeout'
  | 'unauthenticated'
  | 'unavailable'
  | 'unknown';

export interface ResolvedAdminError {
  category: AdminErrorCategory;
  message: string;
  preserveSession: boolean;
  shouldRefreshRecord: boolean;
  silent: boolean;
}

function baseResult(
  category: AdminErrorCategory,
  message: string,
  overrides: Partial<
    Pick<
      ResolvedAdminError,
      'preserveSession' | 'shouldRefreshRecord' | 'silent'
    >
  > = {},
): ResolvedAdminError {
  return {
    category,
    message,
    preserveSession: overrides.preserveSession ?? true,
    shouldRefreshRecord: overrides.shouldRefreshRecord ?? false,
    silent: overrides.silent ?? false,
  };
}

function resolveKnownReason(
  reason: string | undefined,
  httpStatus: number | undefined,
  action: AdminErrorAction,
): null | ResolvedAdminError {
  if (isAdminUserErrorReason(reason)) {
    switch (reason) {
      case AdminUserErrorReason.ADMIN_USER_ERROR_REASON_USER_ALREADY_EXISTS: {
        return baseResult(
          'already_exists',
          action === 'undelete'
            ? '该用户已处于正常状态'
            : '该邮箱已被占用（含已软删除账号占用）',
          { shouldRefreshRecord: action === 'undelete' },
        );
      }
      case AdminUserErrorReason.ADMIN_USER_ERROR_REASON_DOWNSTREAM_UNAVAILABLE: {
        return baseResult(
          'unavailable',
          '下游身份或权限服务暂时不可用，当前管理会话仍有效，请稍后重试',
        );
      }
      case AdminUserErrorReason.ADMIN_USER_ERROR_REASON_ETAG_MISMATCH: {
        return baseResult(
          'etag_mismatch',
          '数据已被更新，已刷新最新状态，请确认后重试',
          { shouldRefreshRecord: true },
        );
      }
      case AdminUserErrorReason.ADMIN_USER_ERROR_REASON_INVALID_STATE: {
        return baseResult('invalid_state', '当前用户状态不支持该操作', {
          shouldRefreshRecord: true,
        });
      }
      case AdminUserErrorReason.ADMIN_USER_ERROR_REASON_SELF_ACTION_FORBIDDEN: {
        return baseResult(
          'self_protected',
          '安全保护：不能对当前登录管理员自身执行封禁或删除',
        );
      }
      case AdminUserErrorReason.ADMIN_USER_ERROR_REASON_TARGET_IS_ADMIN: {
        return baseResult(
          'target_admin',
          '安全保护：不能对具备管理员资格的账号执行封禁或删除',
        );
      }
      case AdminUserErrorReason.ADMIN_USER_ERROR_REASON_USER_NOT_FOUND: {
        return baseResult(
          'not_found',
          action === 'delete'
            ? '目标用户不存在或已被删除'
            : '目标用户不存在或状态已变更',
          { shouldRefreshRecord: true },
        );
      }
      case AdminUserErrorReason.ADMIN_USER_ERROR_REASON_VERIFICATION_EMAIL_FAILED: {
        return baseResult(
          'email_failed',
          '验证邮件投递失败，邮件服务暂时不可用，请稍后重试',
        );
      }
      default: {
        break;
      }
    }
  }

  if (isAdminClientErrorReason(reason)) {
    switch (reason) {
      case AdminClientErrorReason.ADMIN_CLIENT_ERROR_REASON_ALREADY_EXISTS: {
        return baseResult(
          'already_exists',
          '该客户端 ID 已被占用（含静态托管客户端），请更换后重试',
        );
      }
      case AdminClientErrorReason.ADMIN_CLIENT_ERROR_REASON_DOWNSTREAM_UNAVAILABLE: {
        return baseResult(
          'unavailable',
          '下游身份或权限服务暂时不可用，当前管理会话仍有效，请稍后重试',
        );
      }
      case AdminClientErrorReason.ADMIN_CLIENT_ERROR_REASON_ETAG_MISMATCH: {
        return baseResult(
          'etag_mismatch',
          '客户端数据已被更新，已刷新最新状态，请确认后重试',
          { shouldRefreshRecord: true },
        );
      }
      case AdminClientErrorReason.ADMIN_CLIENT_ERROR_REASON_INVALID_CONFIG: {
        return baseResult(
          'invalid_argument',
          '客户端协议或回调地址配置不符合安全规范，请检查授权类型、认证方式与 URI 格式',
        );
      }
      case AdminClientErrorReason.ADMIN_CLIENT_ERROR_REASON_INVALID_STATE: {
        return baseResult('invalid_state', '当前客户端状态或类型不支持该操作', {
          shouldRefreshRecord: true,
        });
      }
      case AdminClientErrorReason.ADMIN_CLIENT_ERROR_REASON_LAST_ACTIVE_SECRET: {
        return baseResult(
          'invalid_state',
          '机密客户端至少需保留一把有效密钥，请先新增新密钥完成轮换后再删除旧密钥',
          { shouldRefreshRecord: true },
        );
      }
      case AdminClientErrorReason.ADMIN_CLIENT_ERROR_REASON_NOT_FOUND: {
        return baseResult(
          'not_found',
          action === 'delete'
            ? '目标 OAuth 客户端不存在或已被删除'
            : '目标 OAuth 客户端或密钥不存在，状态可能已变更',
          { shouldRefreshRecord: true },
        );
      }
      case AdminClientErrorReason.ADMIN_CLIENT_ERROR_REASON_SECRET_LIMIT_EXCEEDED: {
        return baseResult(
          'invalid_state',
          '单个动态客户端最多同时持有 5 把密钥，请先删除旧密钥后再新增',
          { shouldRefreshRecord: true },
        );
      }
      case AdminClientErrorReason.ADMIN_CLIENT_ERROR_REASON_STATIC_CLIENT_IMMUTABLE: {
        return baseResult(
          'static_immutable',
          '该客户端由 IAM 静态配置文件（oidc.yaml）引导托管，不支持在线修改、启停、轮换密钥或删除',
        );
      }
      default: {
        break;
      }
    }
  }

  if (isAdminAuditErrorReason(reason)) {
    switch (reason) {
      case AdminAuditErrorReason.ADMIN_AUDIT_ERROR_REASON_DOWNSTREAM_UNAVAILABLE: {
        return baseResult(
          'unavailable',
          '下游审计存储服务暂时不可用，当前管理会话仍有效，请稍后重试',
        );
      }
      case AdminAuditErrorReason.ADMIN_AUDIT_ERROR_REASON_EVENT_NOT_FOUND: {
        return baseResult('not_found', '目标审计记录不存在或已超过保留期清理');
      }
      case AdminAuditErrorReason.ADMIN_AUDIT_ERROR_REASON_INVALID_ARGUMENT: {
        return baseResult(
          'invalid_argument',
          '审计筛选条件格式有误，请检查起止时间或输入项',
        );
      }
      case AdminAuditErrorReason.ADMIN_AUDIT_ERROR_REASON_INVALID_PAGE_TOKEN: {
        return baseResult(
          'invalid_argument',
          '分页游标已失效，已自动重置回首页',
          { shouldRefreshRecord: true },
        );
      }
      default: {
        break;
      }
    }
  }

  if (isSecurityErrorReason(reason)) {
    switch (reason) {
      case SecurityErrorReason.SECURITY_ERROR_REASON_INVALID_ARGUMENT: {
        return baseResult(
          'invalid_argument',
          '提交的参数格式不符合要求，请检查输入项',
        );
      }
      case SecurityErrorReason.SECURITY_ERROR_REASON_PERMISSION_DENIED: {
        return baseResult('forbidden', '当前账号缺少执行该操作的权限');
      }
      case SecurityErrorReason.SECURITY_ERROR_REASON_UNAUTHENTICATED: {
        return baseResult('unauthenticated', '管理会话已过期，请重新登录', {
          preserveSession: false,
        });
      }
      case SecurityErrorReason.SECURITY_ERROR_REASON_UNAVAILABLE: {
        return baseResult(
          'unavailable',
          '下游身份或权限服务暂时不可用，当前管理会话仍有效，请稍后重试',
        );
      }
      default: {
        break;
      }
    }
  }

  if (isCrudErrorReason(reason) || reason === 'INVALID_ARGUMENT') {
    return baseResult(
      'invalid_argument',
      '提交的参数格式不符合要求，请检查输入项',
    );
  }

  if (reason === 'GATEWAY_TIMEOUT' || httpStatus === 504) {
    return baseResult('timeout', '请求超时，请检查服务响应后重试');
  }

  return null;
}

export function resolveAdminApiError(
  error: unknown,
  action: AdminErrorAction = 'default',
): ResolvedAdminError {
  if (!(error instanceof ApiError)) {
    return baseResult('unknown', '服务处理异常，请稍后重试');
  }

  switch (error.kind) {
    case 'cancelled': {
      return baseResult('cancelled', '', { silent: true });
    }
    case 'network': {
      return baseResult(
        'network',
        '网络连接异常，无法连接管理服务，请稍后重试',
      );
    }
    case 'timeout': {
      return baseResult('timeout', '请求超时，请检查服务响应后重试');
    }
    case 'http': {
      const kratos = parseKratosErrorBody(error.responseBody);
      const matched = resolveKnownReason(
        kratos?.reason,
        error.httpStatus,
        action,
      );
      if (matched) {
        return matched;
      }

      switch (error.httpStatus) {
        case 400:
        case 415: {
          return baseResult(
            'invalid_argument',
            '提交的参数格式不符合要求，请检查输入项',
          );
        }
        case 401: {
          return baseResult('unauthenticated', '管理会话已过期，请重新登录', {
            preserveSession: false,
          });
        }
        case 403: {
          return baseResult('forbidden', '当前账号缺少执行该操作的权限');
        }
        case 404: {
          return baseResult(
            'not_found',
            action === 'delete'
              ? '目标用户不存在或已被删除'
              : '目标用户不存在或状态已变更',
            { shouldRefreshRecord: true },
          );
        }
        case 502:
        case 503: {
          return baseResult(
            'unavailable',
            '下游身份或权限服务暂时不可用，当前管理会话仍有效，请稍后重试',
          );
        }
        default: {
          return baseResult('unknown', '服务处理异常，请稍后重试');
        }
      }
    }
  }
}
