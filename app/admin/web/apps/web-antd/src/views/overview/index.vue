<script lang="ts" setup>
import { computed } from 'vue';
import { useRouter } from 'vue-router';

import { Page } from '@vben/common-ui';

import {
  Avatar,
  Button,
  Card,
  Descriptions,
  Space,
  Tag,
  Tooltip,
} from 'ant-design-vue';

import { resolveUserDisplayName, useAuthStore } from '#/store';

defineOptions({ name: 'AdminOverviewPage' });

const router = useRouter();
const authStore = useAuthStore();

const currentUser = computed(() => authStore.currentUser);
const displayName = computed(() => resolveUserDisplayName(currentUser.value));
const avatarUrl = computed(
  () => currentUser.value?.profile?.picture?.trim() || '',
);
const avatarInitial = computed(() =>
  displayName.value.slice(0, 1).toUpperCase(),
);

const canManageUsers = computed(() => authStore.hasPermission('manage_users'));
const canReadAudit = computed(() => authStore.hasPermission('read_audit'));

const PERMISSION_CATALOG = [
  {
    code: 'access',
    label: '管理台访问（access）',
  },
  {
    code: 'manage_users',
    label: '用户日常管理（manage_users）',
  },
  {
    code: 'expunge_users',
    label: '显式永久删除（expunge_users）',
  },
  {
    code: 'read_audit',
    label: '审计日志查阅（read_audit）',
  },
] as const;

const catalogCodeSet = new Set<string>(
  PERMISSION_CATALOG.map((item) => item.code),
);
const uncataloguedPermissions = computed(() =>
  authStore.permissions.filter((code) => !catalogCodeSet.has(code)),
);

const statusBadge = computed(() => {
  switch (currentUser.value?.status) {
    case 'USER_STATUS_ACTIVE': {
      return { color: 'success', text: '正常（ACTIVE）' };
    }
    case 'USER_STATUS_DISABLED': {
      return { color: 'error', text: '已封禁（DISABLED）' };
    }
    case 'USER_STATUS_PENDING_EMAIL_VERIFICATION': {
      return { color: 'warning', text: '待邮箱验证' };
    }
    default: {
      return { color: 'success', text: '管理会话有效' };
    }
  }
});

function goToUsers() {
  router.push('/users');
}

function goToCreateUser() {
  router.push({ path: '/users', query: { action: 'create' } });
}

function goToRecycleBin() {
  router.push({ path: '/users', query: { view: 'deleted' } });
}

function goToAuditLogs() {
  router.push('/audit');
}
</script>

<template>
  <Page
    description="展示当前平台管理员真实身份与实时授权范围，并提供 IAM 全局用户管理与全平台审计检索快捷入口。"
    title="平台管理总览"
  >
    <div class="flex flex-col gap-4">
      <Card :bordered="false" title="当前管理员身份">
        <div class="mb-6 flex flex-wrap items-center gap-4">
          <Avatar :size="56" :src="avatarUrl || undefined">
            {{ avatarInitial }}
          </Avatar>
          <div class="space-y-1">
            <div class="flex flex-wrap items-center gap-2">
              <span class="text-foreground text-lg font-semibold">
                {{ displayName }}
              </span>
              <Tag :color="statusBadge.color">{{ statusBadge.text }}</Tag>
              <Tag
                :color="currentUser?.emailVerified ? 'processing' : 'default'"
              >
                {{ currentUser?.emailVerified ? '邮箱已验证' : '邮箱未验证' }}
              </Tag>
            </div>
            <div class="text-muted-foreground text-sm">
              {{ currentUser?.email || '—' }}
            </div>
          </div>
        </div>

        <Descriptions
          bordered
          :column="{ lg: 2, md: 2, sm: 1, xs: 1 }"
          size="small"
        >
          <Descriptions.Item label="用户 ID">
            <span class="font-mono text-xs">{{
              currentUser?.userId || '—'
            }}</span>
          </Descriptions.Item>
          <Descriptions.Item label="资源名称">
            <span class="font-mono text-xs">{{
              currentUser?.name || '—'
            }}</span>
          </Descriptions.Item>
          <Descriptions.Item label="登录邮箱（只读）">
            {{ currentUser?.email || '—' }}
          </Descriptions.Item>
          <Descriptions.Item label="实时持有权限" :span="2">
            <Space wrap>
              <Tag
                v-for="item in PERMISSION_CATALOG"
                :key="item.code"
                :color="authStore.hasPermission(item.code) ? 'blue' : 'default'"
              >
                {{ item.label }}：{{
                  authStore.hasPermission(item.code) ? '已授权' : '未授权'
                }}
              </Tag>
              <Tag
                v-for="rawCode in uncataloguedPermissions"
                :key="rawCode"
                color="blue"
              >
                {{ rawCode }}：已授权
              </Tag>
            </Space>
          </Descriptions.Item>
        </Descriptions>
      </Card>

      <Card :bordered="false" title="平台管理与合规审计快捷入口">
        <div class="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-4">
          <Card size="small" title="正常用户列表">
            <p class="text-muted-foreground mb-4 text-sm">
              按邮箱、状态与用户 ID 查询全局账号，查看详情、编辑 7
              项标准资料或执行设密、强制登出与封禁操作。
            </p>
            <Button type="primary" @click="goToUsers">进入正常用户列表</Button>
          </Card>

          <Card size="small" title="创建新用户">
            <p class="text-muted-foreground mb-4 text-sm">
              创建待验证 IAM
              账号并配置初始密码与首次登录改密要求，支持验证邮件发送状态提示与补发。
            </p>
            <Tooltip
              :title="
                canManageUsers ? undefined : '当前账号缺少 manage_users 权限'
              "
            >
              <Button :disabled="!canManageUsers" @click="goToCreateUser">
                立即创建用户
              </Button>
            </Tooltip>
          </Card>

          <Card size="small" title="已删除用户（回收站）">
            <p class="text-muted-foreground mb-4 text-sm">
              查看无限期保留的已软删除账号（原邮箱保持占用），支持恢复账号或核对完整邮箱后显式永久删除。
            </p>
            <Button @click="goToRecycleBin">打开回收站视图</Button>
          </Card>

          <Card size="small" title="全平台审计日志">
            <p class="text-muted-foreground mb-4 text-sm">
              跨服务检索认证会话、资源变更与授权拒绝问责记录，查看脱敏载荷并按同一
              TraceID 串联多服务调用链。
            </p>
            <Tooltip
              :title="canReadAudit ? undefined : '当前账号缺少 read_audit 权限'"
            >
              <Button :disabled="!canReadAudit" @click="goToAuditLogs">
                查看审计日志
              </Button>
            </Tooltip>
          </Card>
        </div>
      </Card>
    </div>
  </Page>
</template>
