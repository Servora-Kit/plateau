<script lang="ts" setup>
import type {
  OAuthClient,
  TokenEndpointAuthMethod,
} from '@plateau/api/plateau/iam/client/v1';

import type {
  CreateOAuthClientFormValues,
  OAuthClientFormErrors,
  OAuthClientPresetKey,
} from '../schemas';

import { computed, ref } from 'vue';
import { useRouter } from 'vue-router';

import { useVbenDrawer } from '@vben/common-ui';
import { LOGIN_PATH } from '@vben/constants';

import {
  Alert,
  Checkbox,
  Divider,
  Input,
  message,
  Select,
  Switch,
  Tag,
} from 'ant-design-vue';

import { createOAuthClient, resolveAdminApiError } from '#/api';
import { useAuthStore } from '#/store';

import {
  applyPresetToCreateForm,
  buildCreateOAuthClientRequest,
  createInitialCreateFormValues,
  getAuthMethodOptions,
  getGrantTypeOptions,
  isConfidentialAuthMethod,
  normalizeAuthMethodChange,
  OAUTH_CLIENT_PRESETS,
  STANDARD_OIDC_SCOPES,
  toggleGrantTypeSelection,
  validateProtocolFormFields,
} from '../schemas';

const emit = defineEmits<{
  created: [
    payload: {
      client: OAuthClient;
      clientSecret?: string;
    },
  ];
}>();

const Textarea = Input.TextArea;

const router = useRouter();
const authStore = useAuthStore();

const form = ref<CreateOAuthClientFormValues>(
  createInitialCreateFormValues('traditional_web'),
);
const errors = ref<OAuthClientFormErrors>({});

const hasAuthCode = computed(() =>
  form.value.allowedGrantTypes.includes('authorization_code'),
);
const hasClientCredentials = computed(() =>
  form.value.allowedGrantTypes.includes('client_credentials'),
);
const isConfidential = computed(() =>
  isConfidentialAuthMethod(form.value.tokenEndpointAuthMethod),
);

const authMethodOptions = computed(() =>
  getAuthMethodOptions(form.value.applicationType),
);
const grantTypeOptions = computed(() =>
  getGrantTypeOptions(
    form.value.applicationType,
    form.value.tokenEndpointAuthMethod,
  ),
);

const redirectPlaceholder = computed(() => {
  if (form.value.applicationType === 'APPLICATION_TYPE_NATIVE') {
    return '每行填写一个回调地址，例如：\nmyapp://callback\nhttp://127.0.0.1:8080/callback';
  }
  return '每行填写一个精确匹配的回调地址，例如：\nhttps://app.example.com/auth/callback\nhttp://app.plateau.localhost:8080/auth/callback';
});

function selectPreset(presetKey: OAuthClientPresetKey) {
  form.value = applyPresetToCreateForm(form.value, presetKey);
  errors.value = {};
}

function handleAuthMethodChange(nextValue: unknown) {
  const method = nextValue as TokenEndpointAuthMethod;
  form.value = normalizeAuthMethodChange(form.value, method);
  errors.value = {
    ...errors.value,
    allowedGrantTypes: undefined,
    tokenEndpointAuthMethod: undefined,
  };
}

function handleToggleGrantType(grantType: string, checked: boolean) {
  const nextGrants = toggleGrantTypeSelection(
    form.value.allowedGrantTypes,
    grantType,
    checked,
  );
  const nextScopes =
    nextGrants.includes('authorization_code') &&
    !form.value.allowedScopes.includes('openid')
      ? ['openid', ...form.value.allowedScopes]
      : [...form.value.allowedScopes];
  form.value = {
    ...form.value,
    allowedGrantTypes: nextGrants,
    allowedScopes: nextScopes,
  };
  errors.value = {
    ...errors.value,
    allowedGrantTypes: undefined,
  };
}

function handleToggleScope(scopeValue: string, checked: boolean) {
  if (scopeValue === 'openid' && hasAuthCode.value) {
    return;
  }
  const set = new Set(form.value.allowedScopes);
  if (checked) {
    set.add(scopeValue);
  } else {
    set.delete(scopeValue);
  }
  if (hasAuthCode.value) {
    set.add('openid');
  }
  form.value = {
    ...form.value,
    allowedScopes: [...set],
  };
  errors.value = {
    ...errors.value,
    allowedScopes: undefined,
  };
}

function clearFieldError(key: keyof OAuthClientFormErrors) {
  if (errors.value[key]) {
    errors.value = {
      ...errors.value,
      [key]: undefined,
    };
  }
}

const [Drawer, drawerApi] = useVbenDrawer({
  confirmText: '创建客户端',
  onCancel() {
    drawerApi.close();
  },
  async onConfirm() {
    const nextErrors = validateProtocolFormFields({
      ...form.value,
      isCreate: true,
    });
    errors.value = nextErrors;
    if (Object.keys(nextErrors).length > 0) {
      message.warning('请先修正表单中标红的配置项');
      return;
    }

    const request = buildCreateOAuthClientRequest(form.value);
    drawerApi.setState({ confirmLoading: true });
    try {
      const response = await createOAuthClient(request);
      const createdClient = response.client ?? {
        applicationType: form.value.applicationType,
        clientId: form.value.oauthClientId.trim() || undefined,
        displayName: form.value.displayName.trim(),
        tokenEndpointAuthMethod: form.value.tokenEndpointAuthMethod,
      };
      const rawSecret = response.clientSecret?.trim() || undefined;
      drawerApi.close();
      if (!rawSecret) {
        message.success('公共 OAuth 客户端已创建');
      }
      emit('created', {
        client: createdClient,
        clientSecret: rawSecret,
      });
    } catch (error) {
      const resolved = resolveAdminApiError(error, 'create');
      if (resolved.silent) {
        return;
      }
      if (!resolved.preserveSession) {
        drawerApi.close();
        message.warning(resolved.message);
        authStore.markUnauthenticated();
        await router.replace({
          path: LOGIN_PATH,
          query: { redirect: router.currentRoute.value.fullPath },
        });
        return;
      }
      message.error(resolved.message);
    } finally {
      drawerApi.setState({ confirmLoading: false });
    }
  },
  onOpenChange(isOpen) {
    if (isOpen) {
      form.value = createInitialCreateFormValues('traditional_web');
      errors.value = {};
    }
  },
  title: '新建 OAuth 2.0 / OIDC 客户端',
});
</script>

<template>
  <Drawer class="w-full max-w-[680px]">
    <div class="space-y-5">
      <div class="space-y-2">
        <div class="text-foreground text-sm font-semibold">
          1. 选择应用接入场景模板
        </div>
        <p class="text-muted-foreground text-xs">
          选择贴合业务架构的预设模板后，系统将自动预填安全推荐的应用类型、令牌认证方式与授权流；创建后应用架构类型不可更改。
        </p>

        <div class="grid grid-cols-1 gap-2.5 sm:grid-cols-2">
          <div
            v-for="item in OAUTH_CLIENT_PRESETS"
            :key="item.key"
            class="cursor-pointer rounded-lg border p-3 transition-colors"
            :class="
              form.preset === item.key
                ? 'border-primary bg-primary/5 ring-primary/30 ring-1'
                : 'border-border hover:bg-muted/40'
            "
            @click="selectPreset(item.key)"
          >
            <div class="mb-1 flex items-center justify-between gap-2">
              <span class="text-foreground text-sm font-semibold">
                {{ item.title }}
              </span>
              <Tag
                class="m-0"
                :color="form.preset === item.key ? 'blue' : 'default'"
              >
                {{ item.badgeText }}
              </Tag>
            </div>
            <p class="text-muted-foreground m-0 text-xs leading-5">
              {{ item.description }}
            </p>
          </div>
        </div>
      </div>

      <Divider class="!my-2" />

      <div class="space-y-4">
        <div class="text-foreground text-sm font-semibold">
          2. 基础标识与品牌信息
        </div>

        <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div class="space-y-1.5">
            <label class="text-foreground block text-sm font-medium">
              客户端显示名称 <span class="text-red-500">*</span>
            </label>
            <Input
              v-model:value="form.displayName"
              allow-clear
              :maxlength="128"
              placeholder="例如：Plateau 博客门户"
              :status="errors.displayName ? 'error' : undefined"
              @change="clearFieldError('displayName')"
            />
            <p v-if="errors.displayName" class="m-0 text-xs text-red-500">
              {{ errors.displayName }}
            </p>
          </div>

          <div class="space-y-1.5">
            <label class="text-foreground block text-sm font-medium">
              自定义 Client ID（选填）
            </label>
            <Input
              v-model:value="form.oauthClientId"
              allow-clear
              class="font-mono"
              :maxlength="63"
              placeholder="留空自动生成 cli_...，创建后不可修改"
              :status="errors.oauthClientId ? 'error' : undefined"
              @change="clearFieldError('oauthClientId')"
            />
            <p v-if="errors.oauthClientId" class="m-0 text-xs text-red-500">
              {{ errors.oauthClientId }}
            </p>
            <p v-else class="text-muted-foreground m-0 text-xs">
              支持小写字母、数字与连字符（如
              <code class="font-mono">blog-web</code>），创建后永久不可变。
            </p>
          </div>
        </div>

        <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div class="space-y-1.5">
            <label class="text-foreground block text-sm font-medium">
              应用图标 URL（选填）
            </label>
            <Input
              v-model:value="form.logoUri"
              allow-clear
              :maxlength="512"
              placeholder="https://example.com/logo.png"
              :status="errors.logoUri ? 'error' : undefined"
              @change="clearFieldError('logoUri')"
            />
            <p v-if="errors.logoUri" class="m-0 text-xs text-red-500">
              {{ errors.logoUri }}
            </p>
          </div>

          <div class="space-y-1.5">
            <label class="text-foreground block text-sm font-medium">
              备注说明（选填）
            </label>
            <Input
              v-model:value="form.description"
              allow-clear
              :maxlength="256"
              placeholder="简要说明该客户端的归属团队或用途"
            />
          </div>
        </div>
      </div>

      <Divider class="!my-2" />

      <div class="space-y-4">
        <div class="text-foreground text-sm font-semibold">
          3. 协议认证与授权策略
        </div>

        <div class="space-y-1.5">
          <label class="text-foreground block text-sm font-medium">
            令牌端点认证方式（Token Endpoint Auth Method）
            <span class="text-red-500">*</span>
          </label>
          <Select
            class="w-full"
            :options="authMethodOptions"
            :status="errors.tokenEndpointAuthMethod ? 'error' : undefined"
            :value="form.tokenEndpointAuthMethod"
            @change="handleAuthMethodChange"
          />
          <p
            v-if="errors.tokenEndpointAuthMethod"
            class="m-0 text-xs text-red-500"
          >
            {{ errors.tokenEndpointAuthMethod }}
          </p>
        </div>

        <div v-if="isConfidential" class="space-y-1.5">
          <label class="text-foreground block text-sm font-medium">
            初始密钥备注名称（选填）
          </label>
          <Input
            v-model:value="form.initialSecretName"
            allow-clear
            :maxlength="64"
            placeholder="例如：默认生产密钥（创建成功后将一次性返回明文密钥）"
          />
        </div>

        <div class="space-y-2">
          <label class="text-foreground block text-sm font-medium">
            允许的授权类型（Grant Types） <span class="text-red-500">*</span>
          </label>
          <div class="flex flex-col gap-2">
            <Checkbox
              v-for="opt in grantTypeOptions"
              :key="opt.value"
              :checked="form.allowedGrantTypes.includes(opt.value)"
              :disabled="opt.disabled"
              @change="
                (e) =>
                  handleToggleGrantType(opt.value, Boolean(e.target.checked))
              "
            >
              {{ opt.label }}
            </Checkbox>
          </div>
          <p v-if="errors.allowedGrantTypes" class="m-0 text-xs text-red-500">
            {{ errors.allowedGrantTypes }}
          </p>
        </div>

        <template v-if="hasAuthCode">
          <div class="space-y-1.5">
            <label class="text-foreground block text-sm font-medium">
              登录回调地址白名单（redirect_uris）
              <span class="text-red-500">*</span>
            </label>
            <Textarea
              v-model:value="form.redirectUrisText"
              class="font-mono text-xs"
              :placeholder="redirectPlaceholder"
              :rows="3"
              :status="errors.redirectUrisText ? 'error' : undefined"
              @change="clearFieldError('redirectUrisText')"
            />
            <p v-if="errors.redirectUrisText" class="m-0 text-xs text-red-500">
              {{ errors.redirectUrisText }}
            </p>
            <p v-else class="text-muted-foreground m-0 text-xs">
              每行一个完整地址，不支持通配符
              <code class="font-mono">*</code> 或
              <code class="font-mono">#fragment</code>。
            </p>
          </div>

          <div class="space-y-1.5">
            <label class="text-foreground block text-sm font-medium">
              登出跳回地址白名单（post_logout_redirect_uris，选填）
            </label>
            <Textarea
              v-model:value="form.postLogoutRedirectUrisText"
              class="font-mono text-xs"
              placeholder="每行填写一个允许在 OIDC /end_session 登出后跳回的地址"
              :rows="2"
              :status="errors.postLogoutRedirectUrisText ? 'error' : undefined"
              @change="clearFieldError('postLogoutRedirectUrisText')"
            />
            <p
              v-if="errors.postLogoutRedirectUrisText"
              class="m-0 text-xs text-red-500"
            >
              {{ errors.postLogoutRedirectUrisText }}
            </p>
          </div>

          <div class="space-y-2">
            <label class="text-foreground block text-sm font-medium">
              允许的权限范围（Allowed Scopes）
              <span class="text-red-500">*</span>
            </label>
            <div class="grid grid-cols-1 gap-2 sm:grid-cols-2">
              <div
                v-for="scope in STANDARD_OIDC_SCOPES"
                :key="scope.value"
                class="border-border rounded-md border px-3 py-2"
              >
                <Checkbox
                  :checked="form.allowedScopes.includes(scope.value)"
                  :disabled="scope.value === 'openid'"
                  @change="
                    (e) =>
                      handleToggleScope(scope.value, Boolean(e.target.checked))
                  "
                >
                  <span class="text-sm font-medium">{{ scope.label }}</span>
                </Checkbox>
                <p class="text-muted-foreground mt-0.5 mb-0 pl-6 text-xs">
                  {{ scope.description }}
                </p>
              </div>
            </div>
            <p v-if="errors.allowedScopes" class="m-0 text-xs text-red-500">
              {{ errors.allowedScopes }}
            </p>
          </div>

          <div
            class="border-border bg-muted/30 flex items-center justify-between gap-4 rounded-md border px-3.5 py-3"
          >
            <div class="space-y-0.5">
              <div class="text-foreground text-sm font-medium">
                第一方受信应用（免用户授权同意页）
              </div>
              <div class="text-muted-foreground text-xs">
                开启后用户完成登录即直接签发授权码；关闭则按第三方应用向用户展示
                <code class="font-mono">/consent/</code> 权限确认页并记忆授权。
              </div>
            </div>
            <Switch v-model:checked="form.trusted" />
          </div>
        </template>

        <div v-if="hasClientCredentials" class="space-y-1.5">
          <label class="text-foreground block text-sm font-medium">
            目标服务受众（Audiences） <span class="text-red-500">*</span>
          </label>
          <Textarea
            v-model:value="form.audiencesText"
            class="font-mono text-xs"
            placeholder="每行填写一个允许签发的服务受众标识，例如：&#10;iam&#10;audit"
            :rows="2"
            :status="errors.audiencesText ? 'error' : undefined"
            @change="clearFieldError('audiencesText')"
          />
          <p v-if="errors.audiencesText" class="m-0 text-xs text-red-500">
            {{ errors.audiencesText }}
          </p>
        </div>

        <Alert
          v-if="isConfidential"
          description="机密客户端创建成功后，服务端将生成首把 CSPRNG 高熵密钥并仅展示一次明文，请在随后的弹窗中立即复制保存。"
          message="初始密钥安全提示"
          show-icon
          type="info"
        />
      </div>
    </div>
  </Drawer>
</template>
