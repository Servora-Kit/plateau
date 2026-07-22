<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { storeToRefs } from 'pinia'

import { useUsersStore } from '@/stores/users'

const store = useUsersStore()
const {
  busy,
  emailFilter,
  error,
  hasNextPage,
  listState,
  parent,
  selected,
  showDeleted,
  status,
  tenant,
  totalSize,
  users,
} = storeToRefs(store)

const createForm = reactive({
  userId: `user-${Date.now().toString().slice(-6)}`,
  displayName: 'Ada Lovelace',
  email: `ada-${Date.now().toString().slice(-6)}@example.com`,
  tenantPlan: 'developer',
  nickname: 'ada',
  temporaryPassword: 'change-me',
})
const editDisplayName = ref('')
const editNickname = ref('')
const clearNickname = ref(false)

const selectedIsDeleted = computed(() => selected.value?.deleteTime != null)
const selectedJson = computed(() =>
  selected.value ? JSON.stringify(selected.value, null, 2) : '尚未选择 User',
)

watch(
  selected,
  (user) => {
    editDisplayName.value = user?.displayName ?? ''
    editNickname.value = user?.nickname ?? ''
    clearNickname.value = false
  },
  { immediate: true },
)

onMounted(() => store.listUsers())

async function submitCreate(): Promise<void> {
  const created = await store.createUser(createForm)
  if (created) {
    createForm.userId = `user-${Date.now().toString().slice(-6)}`
    createForm.email = `user-${Date.now().toString().slice(-6)}@example.com`
    createForm.temporaryPassword = ''
  }
}

async function submitUpdate(): Promise<void> {
  await store.updateUser({
    displayName: editDisplayName.value,
    nickname: editNickname.value,
    clearNickname: clearNickname.value,
  })
}

function formatTimestamp(value: string | null | undefined): string {
  if (!value) return '—'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString('zh-CN', { hour12: false })
}
</script>

<template>
  <div class="console-layout">
    <section class="hero" aria-labelledby="page-title">
      <div>
        <p class="eyebrow">SERVORA / LIVE REFERENCE</p>
        <h1 id="page-title">User CRUD 控制台</h1>
        <p class="hero-copy">
          Vue 直接消费生成的 HTTP client、资源名 helper、FieldMask、filter、order 与 pager。
          每一次操作都会命中本地运行中的 example 微服务。
        </p>
      </div>
      <dl class="contract-strip" aria-label="当前协议合同">
        <div>
          <dt>Resource</dt>
          <dd>example.servora.dev/User</dd>
        </div>
        <div>
          <dt>Parent</dt>
          <dd>{{ parent }}</dd>
        </div>
        <div>
          <dt>Total</dt>
          <dd>{{ totalSize ?? '未计算' }}</dd>
        </div>
      </dl>
    </section>

    <div class="status-region" :class="{ 'status-region--error': error }">
      <output aria-live="polite" aria-atomic="true">{{ status }}</output>
      <p v-if="error" role="alert">{{ error }}</p>
    </div>

    <section class="panel panel--create" aria-labelledby="create-title">
      <div class="panel-heading">
        <div>
          <p class="panel-index">01 / CREATE</p>
          <h2 id="create-title">创建资源</h2>
        </div>
        <code>POST /v1/{{ parent }}/users</code>
      </div>

      <form class="form-grid" @submit.prevent="submitCreate">
        <label>
          <span>Tenant</span>
          <input v-model="tenant" name="tenant" required autocomplete="organization" :disabled="busy" />
        </label>
        <label>
          <span>User ID</span>
          <input v-model="createForm.userId" name="user-id" required autocomplete="off" />
        </label>
        <label>
          <span>显示名称</span>
          <input v-model="createForm.displayName" name="display-name" autocomplete="name" />
        </label>
        <label>
          <span>Email</span>
          <input
            v-model="createForm.email"
            name="email"
            type="email"
            required
            autocomplete="email"
          />
        </label>
        <label>
          <span>Tenant plan</span>
          <input v-model="createForm.tenantPlan" name="tenant-plan" autocomplete="off" />
        </label>
        <label>
          <span>Nickname</span>
          <input v-model="createForm.nickname" name="nickname" autocomplete="nickname" />
        </label>
        <label class="form-grid__wide">
          <span>临时密码 <small>INPUT_ONLY，响应中会被清理</small></span>
          <input
            v-model="createForm.temporaryPassword"
            name="temporary-password"
            type="password"
            autocomplete="new-password"
          />
        </label>
        <button class="button button--primary form-grid__action" type="submit" :disabled="busy">
          创建 User
        </button>
      </form>
    </section>

    <section class="panel panel--list" aria-labelledby="list-title">
      <div class="panel-heading">
        <div>
          <p class="panel-index">02 / LIST</p>
          <h2 id="list-title">集合查询</h2>
        </div>
        <code>filter + order_by + pager</code>
      </div>

      <form class="query-bar" @submit.prevent="store.listUsers()">
        <label class="query-bar__filter">
          <span>Email 精确过滤</span>
          <input
            v-model="emailFilter"
            name="email-filter"
            type="search"
            placeholder="name@example.com"
            :disabled="busy"
          />
        </label>
        <label class="toggle">
          <input v-model="showDeleted" name="show-deleted" type="checkbox" :disabled="busy" />
          <span>包含 tombstone</span>
        </label>
        <button class="button" type="submit" :disabled="busy">刷新列表</button>
      </form>

      <div class="table-shell">
        <table>
          <thead>
            <tr>
              <th scope="col">资源名</th>
              <th scope="col">显示名称</th>
              <th scope="col">Email</th>
              <th scope="col">状态</th>
              <th scope="col"><span class="visually-hidden">操作</span></th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="user in users"
              :key="user.name"
              :class="{ 'is-selected': user.name === selected?.name }"
            >
              <td>
                <code>{{ user.name }}</code>
              </td>
              <td>{{ user.displayName || '—' }}</td>
              <td>{{ user.email }}</td>
              <td>
                <span class="state-chip" :class="{ 'state-chip--deleted': user.deleteTime }">
                  {{ user.deleteTime ? '已删除' : '活跃' }}
                </span>
              </td>
              <td>
                <button
                  class="button button--small"
                  type="button"
                  :disabled="busy || !user.name"
                  :aria-label="`读取 ${user.name ?? 'User'}`"
                  @click="user.name && store.getUser(user.name)"
                >
                  读取
                </button>
              </td>
            </tr>
            <tr v-if="listState === 'loading'">
              <td class="empty-cell" colspan="5">正在加载 User…</td>
            </tr>
            <tr v-else-if="listState === 'error'">
              <td class="empty-cell" colspan="5">列表加载失败，请修正条件后重试</td>
            </tr>
            <tr v-else-if="listState === 'success' && users.length === 0">
              <td class="empty-cell" colspan="5">当前查询没有 User</td>
            </tr>
          </tbody>
        </table>
      </div>

      <button
        v-if="hasNextPage"
        class="button button--load-more"
        type="button"
        :disabled="busy"
        @click="store.listUsers(true)"
      >
        加载下一页
      </button>
    </section>

    <section class="panel panel--detail" aria-labelledby="detail-title">
      <div class="panel-heading">
        <div>
          <p class="panel-index">03 / MUTATE</p>
          <h2 id="detail-title">资源详情与生命周期</h2>
        </div>
        <span class="state-chip" :class="{ 'state-chip--deleted': selectedIsDeleted }">
          {{ selected ? (selectedIsDeleted ? 'TOMBSTONE' : 'ACTIVE') : 'NO SELECTION' }}
        </span>
      </div>

      <div v-if="selected" class="detail-grid">
        <form class="edit-form" @submit.prevent="submitUpdate">
          <label>
            <span>显示名称</span>
            <input v-model="editDisplayName" name="edit-display-name" />
          </label>
          <label>
            <span>Nickname</span>
            <input v-model="editNickname" name="edit-nickname" :disabled="clearNickname" />
          </label>
          <label class="toggle">
            <input v-model="clearNickname" name="clear-nickname" type="checkbox" />
            <span>FieldMask 显式清除 nickname</span>
          </label>
          <button
            class="button button--primary"
            type="submit"
            :disabled="busy || selectedIsDeleted"
          >
            更新选中 User
          </button>
        </form>

        <dl class="resource-facts">
          <div>
            <dt>Name</dt>
            <dd>
              <code>{{ selected.name }}</code>
            </dd>
          </div>
          <div>
            <dt>ETag</dt>
            <dd>
              <code>{{ selected.etag }}</code>
            </dd>
          </div>
          <div>
            <dt>Created</dt>
            <dd>{{ formatTimestamp(selected.createTime) }}</dd>
          </div>
          <div>
            <dt>Updated</dt>
            <dd>{{ formatTimestamp(selected.updateTime) }}</dd>
          </div>
          <div>
            <dt>Deleted</dt>
            <dd>{{ formatTimestamp(selected.deleteTime) }}</dd>
          </div>
        </dl>

        <div class="lifecycle-actions">
          <button
            class="button"
            :class="selectedIsDeleted ? 'button--restore' : 'button--danger'"
            type="button"
            :disabled="busy"
            aria-describedby="delete-copy"
            @click="selectedIsDeleted ? store.undeleteUser() : store.deleteUser()"
          >
            {{ selectedIsDeleted ? '恢复 User' : '软删除 User' }}
          </button>
          <p id="delete-copy">Delete 返回 tombstone；Undelete 清除 delete_time 并轮换 ETag。</p>
        </div>
      </div>
      <p v-else class="empty-detail">从列表读取一个 User，或先创建新资源。</p>

      <details class="payload" :open="Boolean(selected)">
        <summary>ProtoJSON 响应</summary>
        <pre>{{ selectedJson }}</pre>
      </details>
    </section>
  </div>
</template>
