<script setup lang="ts">
import { adminApi } from '~/composables/useAdminApi'
import { copyText, formatTime, roleText } from '~/lib/admin'
import type { AuthKeyItem } from '~/types/admin'

definePageMeta({
  navKey: 'keys',
})

const admin = useAdminApp()
const confirm = useConfirmDialog()

const items = ref<AuthKeyItem[]>([])
const loading = ref(false)
const actionBusy = ref(false)
const search = ref('')
const roleFilter = ref<'all' | 'admin' | 'user'>('all')
const roleDrafts = ref<Record<string, string>>({})
const noteDrafts = ref<Record<string, string>>({})

const modalOpen = ref(false)
const modalKey = ref('')
const modalRole = ref('user')
const modalNote = ref('')
const modalError = ref('')

const ROLE_OPTIONS = [
  { title: '普通成员', value: 'user' },
  { title: '管理员', value: 'admin' },
]

const filteredItems = computed(() => {
  const query = search.value.trim().toLowerCase()
  return items.value.filter((item) => {
    if (roleFilter.value !== 'all' && item.role !== roleFilter.value) {
      return false
    }
    if (!query) {
      return true
    }
    return [item.key, item.role, item.note]
      .some((value) => String(value || '').toLowerCase().includes(query))
  })
})

const changedCount = computed(() => items.value.filter((item) => itemChanged(item)).length)

const summaryTiles = computed(() => [
  {
    label: '全部密钥',
    value: items.value.length,
    helper: '后台和 API 共用的密钥总数',
    icon: 'mdi-key-outline',
  },
  {
    label: '管理员',
    value: items.value.filter((item) => item.role === 'admin').length,
    helper: '拥有后台管理权限',
    icon: 'mdi-shield-account-outline',
  },
  {
    label: '未保存修改',
    value: changedCount.value,
    helper: '角色或备注发生变化',
    icon: 'mdi-content-save-alert-outline',
  },
])

function syncDrafts(nextItems: AuthKeyItem[]) {
  roleDrafts.value = Object.fromEntries(nextItems.map((item) => [item.key, item.role]))
  noteDrafts.value = Object.fromEntries(nextItems.map((item) => [item.key, item.note || '']))
}

async function loadAuthKeys() {
  loading.value = true
  try {
    items.value = await adminApi.listAuthKeys(admin.token.value)
    syncDrafts(items.value)
  } catch (error) {
    admin.notify(error instanceof Error ? error.message : '加载密钥失败', 'danger')
  } finally {
    loading.value = false
  }
}

function openCreateModal() {
  modalOpen.value = true
  modalKey.value = ''
  modalRole.value = 'user'
  modalNote.value = ''
  modalError.value = ''
}

function closeModal() {
  modalOpen.value = false
  modalError.value = ''
}

async function createAuthKey() {
  actionBusy.value = true
  modalError.value = ''

  try {
    const payload: { key?: string; role: string; note: string } = {
      role: modalRole.value,
      note: modalNote.value.trim(),
    }

    if (modalKey.value.trim()) {
      payload.key = modalKey.value.trim()
    }

    await adminApi.createAuthKey(admin.token.value, payload)
    closeModal()
    admin.notify('密钥已创建')
    await Promise.all([
      admin.loadOverview(admin.token.value, true),
      loadAuthKeys(),
    ])
  } catch (error) {
    modalError.value = error instanceof Error ? error.message : '创建密钥失败'
  } finally {
    actionBusy.value = false
  }
}

function selectedRole(item: AuthKeyItem) {
  return roleDrafts.value[item.key] || item.role
}

function roleChanged(item: AuthKeyItem) {
  return selectedRole(item) !== item.role
}

function selectedNote(item: AuthKeyItem) {
  return noteDrafts.value[item.key] ?? item.note ?? ''
}

function noteChanged(item: AuthKeyItem) {
  return selectedNote(item).trim() !== (item.note || '').trim()
}

function itemChanged(item: AuthKeyItem) {
  return roleChanged(item) || noteChanged(item)
}

async function copyAuthKey(value: string) {
  if (await copyText(value)) {
    admin.notify('密钥已复制')
  } else {
    admin.notify('复制失败，请手动复制', 'warning')
  }
}

function updateAuthKey(item: AuthKeyItem) {
  const nextRole = selectedRole(item)
  const nextNote = selectedNote(item).trim()
  if (nextRole === item.role && nextNote === (item.note || '').trim()) {
    return
  }

  confirm.show({
    title: '保存密钥设置',
    message: `确认保存 ${item.key} 的角色和备注修改吗？`,
    confirmText: '确认保存',
    action: async () => {
      actionBusy.value = true
      try {
        await adminApi.updateAuthKey(admin.token.value, item.key, {
          role: nextRole,
          note: nextNote,
        })
        admin.notify('密钥设置已更新')
        await Promise.all([
          admin.loadOverview(admin.token.value, true),
          loadAuthKeys(),
        ])
      } catch (error) {
        admin.notify(error instanceof Error ? error.message : '更新密钥失败', 'danger')
        roleDrafts.value[item.key] = item.role
        noteDrafts.value[item.key] = item.note || ''
      } finally {
        actionBusy.value = false
      }
    },
  })
}

function deleteAuthKey(item: AuthKeyItem) {
  confirm.show({
    title: '删除 API 密钥',
    message: `确认删除密钥 ${item.key} 吗？此操作不可撤销。`,
    confirmText: '确认删除',
    action: async () => {
      actionBusy.value = true
      try {
        await adminApi.deleteAuthKey(admin.token.value, item.key)
        admin.notify('密钥已删除')
        await Promise.all([
          admin.loadOverview(admin.token.value, true),
          loadAuthKeys(),
        ])
      } catch (error) {
        admin.notify(error instanceof Error ? error.message : '删除密钥失败', 'danger')
      } finally {
        actionBusy.value = false
      }
    },
  })
}

onMounted(() => {
  if (admin.authReady.value) {
    void loadAuthKeys()
  }
})

watch(
  () => admin.authReady.value,
  (ready) => {
    if (ready) {
      void loadAuthKeys()
    }
  },
)
</script>

<template>
  <div class="page-grid">
    <PageHeader
      eyebrow="访问控制"
      title="API 密钥"
      icon="mdi-shield-key-outline"
    >
      <template #meta>
        <AdminBadge tone="secondary" icon="mdi-counter">
          共 {{ items.length }} 个密钥
        </AdminBadge>
        <AdminBadge v-if="changedCount" tone="warning" icon="mdi-content-save-alert-outline">
          {{ changedCount }} 处未保存
        </AdminBadge>
      </template>
      <template #actions>
        <AdminButton prepend-icon="mdi-plus" @click="openCreateModal">新建密钥</AdminButton>
      </template>
    </PageHeader>

    <SectionCard
      title="筛选与状态"
      eyebrow="浏览"
      icon="mdi-filter-variant"
    >
      <div class="d-grid ga-5">
        <div class="summary-grid">
          <MetricCard
            v-for="tile in summaryTiles"
            :key="tile.label"
            :label="tile.label"
            :value="tile.value"
            :helper="tile.helper"
            :icon="tile.icon"
            :color="tile.color"
          />
        </div>

        <div class="toolbar-panel">
          <div class="filter-toolbar">
            <VTextField
              v-model="search"
              class="filter-grow"
              label="搜索"
              placeholder="密钥 / 角色 / 备注"
              prepend-inner-icon="mdi-magnify"
              clearable
            />
          </div>

          <VChipGroup v-model="roleFilter" mandatory color="primary">
            <VChip value="all" filter>全部角色</VChip>
            <VChip value="admin" filter>管理员</VChip>
            <VChip value="user" filter>普通成员</VChip>
          </VChipGroup>
        </div>
      </div>
    </SectionCard>

    <SectionCard
      title="密钥列表"
      :eyebrow="`${filteredItems.length} 条结果`"
      icon="mdi-key-outline"
    >
      <div v-if="filteredItems.length" class="key-grid">
        <VCard
          v-for="item in filteredItems"
          :key="item.key"
          color="surface-container"
          variant="flat"
        >
          <VCardText class="key-card-body">
            <div class="key-card-top">
              <div class="stack-card-copy">
                <div class="stack-card-meta">
                  <AdminBadge :tone="item.role === 'admin' ? 'warning' : 'success'">
                    {{ roleText(item.role) }}
                  </AdminBadge>
                  <AdminBadge v-if="itemChanged(item)" tone="accent">
                    有未保存修改
                  </AdminBadge>
                </div>
                <div class="key-inline-row">
                  <code
                    class="key-code-surface key-code-clickable"
                    title="点击复制"
                    @click="copyAuthKey(item.key)"
                  >{{ item.key }}</code>
                  <AdminButton
                    variant="danger"
                    size="sm"
                    :disabled="actionBusy"
                    @click="deleteAuthKey(item)"
                  >
                    删除
                  </AdminButton>
                </div>
                <div class="text-body-2 text-medium-emphasis">
                  创建于 {{ formatTime(item.created_at) }}
                </div>
              </div>
            </div>

            <VRow>
              <VCol cols="12" md="4">
                <VSelect
                  :model-value="selectedRole(item)"
                  label="角色"
                  :items="ROLE_OPTIONS"
                  @update:model-value="(value) => roleDrafts[item.key] = String(value || '')"
                />
              </VCol>
              <VCol cols="12" md="8">
                <VTextField
                  :model-value="selectedNote(item)"
                  label="备注"
                  placeholder="例如：CI / 本地开发 / 管理员"
                  prepend-inner-icon="mdi-note-outline"
                  @update:model-value="(value) => noteDrafts[item.key] = String(value || '')"
                />
              </VCol>
            </VRow>

            <AdminButton
              variant="secondary"
              :disabled="actionBusy || !itemChanged(item)"
              @click="updateAuthKey(item)"
            >
              保存修改
            </AdminButton>
          </VCardText>
        </VCard>
      </div>

      <EmptyState
        v-else
        title="还没有 API 密钥"
        description="创建一个密钥后，就可以用于后台或接口访问。"
        icon="mdi-key-plus"
      />
    </SectionCard>

    <ModalDialog
      :open="modalOpen"
      title="新建 API 密钥"
      description="不填写自定义密钥时，系统会自动生成。"
      @close="closeModal"
    >
      <div class="d-grid ga-4">
        <VTextField
          v-model="modalKey"
          label="自定义密钥"
          placeholder="留空时自动生成 sk-..."
          prepend-inner-icon="mdi-key-outline"
        />
        <VSelect
          v-model="modalRole"
          label="角色"
          prepend-inner-icon="mdi-account-badge-outline"
          :items="ROLE_OPTIONS"
        />
        <VTextField
          v-model="modalNote"
          label="备注"
          placeholder="例如：CI / 本地开发"
          prepend-inner-icon="mdi-note-outline"
        />
        <VAlert
          v-if="modalError"
          type="error"
          variant="tonal"
          density="comfortable"
          :text="modalError"
        />
      </div>
      <template #footer>
        <AdminButton variant="ghost" @click="closeModal">取消</AdminButton>
        <AdminButton
          prepend-icon="mdi-shield-plus-outline"
          :loading="actionBusy"
          @click="createAuthKey"
        >
          创建密钥
        </AdminButton>
      </template>
    </ModalDialog>

    <ModalDialog
      :open="confirm.open.value"
      :title="confirm.title.value"
      description="操作会立即影响当前密钥权限。"
      @close="confirm.close()"
    >
      <p class="text-body-1">{{ confirm.message.value }}</p>
      <template #footer>
        <AdminButton variant="ghost" :disabled="actionBusy" @click="confirm.close()">取消</AdminButton>
        <AdminButton variant="danger" :loading="actionBusy" @click="confirm.submit()">{{ confirm.text.value }}</AdminButton>
      </template>
    </ModalDialog>
  </div>
</template>
