import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import { ApiError, kratosMessage, parseKratosError } from '@servora/proto-utils/client'
import {
  advancePager,
  applyPager,
  buildFilter,
  buildOrderBy,
  firstPage,
  makeUpdateMask,
  type Pager,
} from '@servora/proto-utils/crud'

import type { ListUsersRequest } from '@/api/generated/example/service/v1'
import {
  UserFields,
  UserName,
  UserUpdateFields,
} from '@/api/generated/example/service/v1/user.crud'
import { userApi, type User } from '@/api/userApi'

type CreateUserInput = Readonly<{
  userId: string
  displayName: string
  email: string
  tenantPlan: string
  nickname: string
  temporaryPassword: string
}>

type UpdateUserInput = Readonly<{
  displayName: string
  nickname: string
  clearNickname: boolean
}>

function userMessage(overrides: Partial<User>): User {
  const message: User = {
    createTime: undefined,
    deleteTime: undefined,
    etag: undefined,
    name: undefined,
    purgeTime: undefined,
    updateTime: undefined,
  }
  Object.assign(message, overrides)
  return message
}

function failureMessage(error: unknown): string {
  if (error instanceof ApiError) {
    const payload = parseKratosError(error)
    return payload ? `${payload.reason} · ${payload.message}` : kratosMessage(error)
  }
  return error instanceof Error ? error.message : String(error)
}


export const useUsersStore = defineStore('users', () => {
  const tenant = ref('demo')
  const emailFilter = ref('')
  const showDeleted = ref(false)
  const users = ref<User[]>([])
  const selected = ref<User | null>(null)
  const pager = ref<Pager>(firstPage)
  const totalSize = ref<string | undefined>()
  const busy = ref(false)
  const listState = ref<'idle' | 'loading' | 'success' | 'error'>('idle')
  const error = ref('')
  const status = ref('等待操作')

  const parent = computed(() => `tenants/${tenant.value.trim()}`)
  const hasNextPage = computed(() => pager.value.pageToken !== undefined && !pager.value.exhausted)

  async function execute<T>(label: string, action: () => Promise<T>): Promise<T | undefined> {
    busy.value = true
    error.value = ''
    status.value = `${label}…`
    try {
      const result = await action()
      status.value = `${label}完成`
      return result
    } catch (cause: unknown) {
      error.value = failureMessage(cause)
      status.value = `${label}失败`
      return undefined
    } finally {
      busy.value = false
    }
  }

  function assertTenant(): string {
    const value = tenant.value.trim()
    if (value.length === 0 || value.includes('/')) {
      throw new RangeError('Tenant 必须非空且不能包含斜杠')
    }
    return value
  }


  async function listUsers(append = false): Promise<boolean> {
    if (!append) {
      pager.value = firstPage
      users.value = []
      totalSize.value = undefined
    }
    listState.value = 'loading'
    const response = await execute('加载 User 列表', async () => {
      const currentTenant = assertTenant()
      const filter = emailFilter.value.trim()
        ? buildFilter(UserFields, {
            field: UserFields.email,
            operator: '=',
            value: emailFilter.value.trim(),
          })
        : undefined
      const orderBy = buildOrderBy(UserFields, [
        { field: UserFields.createTime, direction: 'desc' },
      ])
      const request = applyPager(pager.value, {
        filter,
        includeTotal: true,
        orderBy,
        pageSize: 20,
        pageToken: undefined,
        parent: `tenants/${currentTenant}`,
        showDeleted: showDeleted.value,
        skip: undefined,
      } satisfies ListUsersRequest)
      return userApi.ListUsers(request)
    })
    if (!response) {
      listState.value = 'error'
      return false
    }
    const page = response.users ?? []
    users.value = append ? [...users.value, ...page] : page
    totalSize.value = response.totalSize
    pager.value = advancePager(response.nextPageToken)
    listState.value = 'success'
    return true
  }

  async function createUser(input: CreateUserInput): Promise<User | undefined> {
    return execute('创建 User', async () => {
      const currentTenant = assertTenant()
      const userId = input.userId.trim()
      const expectedName = UserName.format({ tenant: currentTenant, user: userId })
      const created = await userApi.CreateUser({
        parent: `tenants/${currentTenant}`,
        userId,
        user: userMessage({
          displayName: input.displayName.trim(),
          email: input.email.trim(),
          nickname: input.nickname.trim() || undefined,
          temporaryPassword: input.temporaryPassword || undefined,
          tenantPlan: input.tenantPlan.trim() || undefined,
        }),
      })
      if (created.name !== expectedName) {
        throw new Error(`服务返回了非预期资源名：${created.name ?? '<empty>'}`)
      }
      selected.value = created
      await listUsers()
      return created
    })
  }

  async function getUser(name: string): Promise<User | undefined> {
    return execute('读取 User', async () => {
      UserName.parse(name)
      const user = await userApi.GetUser({ name })
      selected.value = user
      return user
    })
  }

  async function updateUser(input: UpdateUserInput): Promise<User | undefined> {
    return execute('更新 User', async () => {
      const current = selected.value
      if (!current?.name) {
        throw new Error('请先选择一个 User')
      }
      const values: { displayName?: string; nickname?: string } = {
        displayName: input.displayName,
        nickname: input.clearNickname ? undefined : input.nickname,
      }
      const updateMask = makeUpdateMask(UserUpdateFields, values)
      const updated = await userApi.UpdateUser({
        allowMissing: false,
        updateMask,
        user: userMessage({ name: current.name, etag: current.etag, ...values }),
      })
      selected.value = updated
      await listUsers()
      return updated
    })
  }

  async function deleteUser(): Promise<User | undefined> {
    return execute('删除 User', async () => {
      const current = selected.value
      if (!current?.name) {
        throw new Error('请先选择一个 User')
      }
      const deleted = await userApi.DeleteUser({
        allowMissing: false,
        etag: current.etag,
        name: current.name,
      })
      selected.value = deleted
      await listUsers()
      return deleted
    })
  }

  async function undeleteUser(): Promise<User | undefined> {
    return execute('恢复 User', async () => {
      const name = selected.value?.name
      if (!name) {
        throw new Error('请先选择一个 User')
      }
      const restored = await userApi.UndeleteUser({ name })
      selected.value = restored
      await listUsers()
      return restored
    })
  }

  return {
    busy,
    createUser,
    deleteUser,
    emailFilter,
    error,
    getUser,
    hasNextPage,
    listState,
    listUsers,
    parent,
    selected,
    showDeleted,
    status,
    tenant,
    totalSize,
    undeleteUser,
    updateUser,
    users,
  }
})
