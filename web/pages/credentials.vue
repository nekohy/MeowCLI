<script setup lang="ts">
import { adminApi } from '~/composables/useAdminApi'
import {
  apiTypesText,
  formatPercent,
  formatTime,
  isUnsynced,
  isZeroTime,
  normalizePlanType,
  planTypeText,
  statusText,
  toneForStatus,
} from '~/lib/admin'
import type { CodexItem } from '~/types/admin'

definePageMeta({
  navKey: 'credentials',
})

const admin = useAdminApp()

const rows = ref<CodexItem[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(25)
const loading = ref(false)
const search = ref('')
const selectedIds = ref<string[]>([])
const actionBusy = ref(false)

const importOpen = ref(false)
const importTokens = ref('')
const importError = ref('')

const confirmOpen = ref(false)
const confirmTitle = ref('')
const confirmMessage = ref('')
const confirmText = ref('确认')
const confirmVariant = ref<'secondary' | 'danger'>('danger')
let confirmAction: null | (() => Promise<void>) = null

const filteredRows = computed(() => {
  const query = search.value.trim().toLowerCase()
  if (!query) {
    return rows.value
  }
  return rows.value.filter((item) => (
    [item.id, item.status, normalizePlanType(item.plan_type)]
      .some((value) => String(value || '').toLowerCase().includes(query))
  ))
})

const selectedSet = computed(() => new Set(selectedIds.value))
const allVisibleSelected = computed(() => filteredRows.value.length > 0 && filteredRows.value.every((item) => selectedSet.value.has(item.id)))
const maxPage = computed(() => Math.max(1, Math.ceil((total.value || 0) / (pageSize.value || 25))))

function closeImportModal() {
  importOpen.value = false
  importTokens.value = ''
  importError.value = ''
}

function openConfirm(options: {
  title: string
  message: string
  confirmText: string
  confirmVariant?: 'secondary' | 'danger'
  action: () => Promise<void>
}) {
  confirmTitle.value = options.title
  confirmMessage.value = options.message
  confirmText.value = options.confirmText
  confirmVariant.value = options.confirmVariant || 'danger'
  confirmAction = options.action
  confirmOpen.value = true
}

function closeConfirm() {
  if (actionBusy.value) {
    return
  }
  confirmOpen.value = false
  confirmAction = null
}

async function submitConfirm() {
  if (!confirmAction || actionBusy.value) {
    return
  }
  const action = confirmAction
  closeConfirm()
  await action()
}

function toggleSelectAll() {
  if (allVisibleSelected.value) {
    selectedIds.value = []
    return
  }
  selectedIds.value = filteredRows.value.map((item) => item.id)
}

function toggleSelectOne(id: string) {
  selectedIds.value = selectedSet.value.has(id)
    ? selectedIds.value.filter((value) => value !== id)
    : [...selectedIds.value, id]
}

async function loadCredentials(nextPage = page.value, nextPageSize = pageSize.value) {
  if (!admin.token.value || !admin.activeHandler.value?.supports_credentials) {
    return
  }

  loading.value = true
  try {
    const data = await adminApi.listCodex(admin.token.value, { page: nextPage, pageSize: nextPageSize })
    rows.value = data.data || []
    total.value = data.total || 0
    page.value = data.page || nextPage
    pageSize.value = data.page_size || nextPageSize
    selectedIds.value = []
  } catch (error) {
    admin.notify(error instanceof Error ? error.message : '加载凭据失败', 'danger')
  } finally {
    loading.value = false
  }
}

async function createCredential() {
  actionBusy.value = true
  try {
    const lines = importTokens.value
      .split('\n')
      .map((line) => line.trim())
      .filter(Boolean)

    if (lines.length === 0) {
      importError.value = '请至少输入一行令牌'
      actionBusy.value = false
      return
    }

    const result = await adminApi.batchCreateCodex(admin.token.value, { tokens: lines })

    const createdCount = result.created?.length || 0
    const errorCount = result.errors?.length || 0
    if (errorCount > 0) {
      const details = result.errors.map((e: { input: string; error: string }) => `${e.input}：${e.error}`).join('\n')
      importError.value = `${errorCount} 条失败：\n${details}`
      if (createdCount > 0) {
        admin.notify(`导入完成：${createdCount} 条成功，${errorCount} 条失败`, 'warning')
      }
    } else {
      closeImportModal()
      admin.notify(`已导入 ${createdCount} 条凭据`)
    }

    await Promise.all([
      admin.loadOverview(admin.token.value, true),
      loadCredentials(1, pageSize.value),
    ])
  } catch (error) {
    importError.value = error instanceof Error ? error.message : '导入凭据失败'
  } finally {
    actionBusy.value = false
  }
}

function batchSetStatus(status: string) {
  const ids = [...selectedIds.value]
  if (!ids.length) {
    return
  }

  openConfirm({
    title: `批量${statusText(status)}凭据`,
    message: `确认将 ${ids.length} 个凭据设为“${statusText(status)}”吗？`,
    confirmText: `确认${statusText(status)}`,
    confirmVariant: 'secondary',
    action: async () => {
      actionBusy.value = true
      try {
        const result = await adminApi.batchUpdateCodexStatus(admin.token.value, { ids, status })
        const updatedCount = result.updated?.length || 0
        const errorCount = result.errors?.length || 0
        if (errorCount > 0) {
          admin.notify(`处理完成：${updatedCount} 条成功，${errorCount} 条失败`, 'warning')
        } else {
          admin.notify(`已更新 ${updatedCount} 条凭据`)
        }
        await Promise.all([
          admin.loadOverview(admin.token.value, true),
          loadCredentials(page.value, pageSize.value),
        ])
      } catch (error) {
        admin.notify(error instanceof Error ? error.message : '批量更新状态失败', 'danger')
      } finally {
        actionBusy.value = false
      }
    },
  })
}

function batchDelete() {
  const ids = [...selectedIds.value]
  if (!ids.length) {
    return
  }

  openConfirm({
    title: '批量删除凭据',
    message: `确认删除 ${ids.length} 个凭据吗？此操作不可撤销。`,
    confirmText: '确认删除',
    action: async () => {
      actionBusy.value = true
      try {
        const result = await adminApi.batchDeleteCodex(admin.token.value, { ids })
        const deletedCount = result.deleted?.length || 0
        const errorCount = result.errors?.length || 0
        if (errorCount > 0) {
          admin.notify(`删除完成：${deletedCount} 条成功，${errorCount} 条失败`, 'warning')
        } else {
          admin.notify(`已删除 ${deletedCount} 条凭据`)
        }
        await Promise.all([
          admin.loadOverview(admin.token.value, true),
          loadCredentials(1, pageSize.value),
        ])
      } catch (error) {
        admin.notify(error instanceof Error ? error.message : '批量删除失败', 'danger')
      } finally {
        actionBusy.value = false
      }
    },
  })
}

function renderQuotaValue(item: typeof rows.value[number], quotaKey: 'quota_5h' | 'quota_7d', resetKey: 'reset_5h' | 'reset_7d') {
  if (isUnsynced(item.synced_at)) {
    return '未同步'
  }
  if (item[quotaKey] === 1 && isZeroTime(item[resetKey])) {
    return '不适用'
  }
  return formatPercent(item[quotaKey])
}

onMounted(() => {
  if (admin.authReady.value) {
    void loadCredentials()
  }
})

watch(
  () => admin.authReady.value,
  (ready) => {
    if (ready) {
      void loadCredentials(1, pageSize.value)
    }
  },
)
</script>

<template>
  <div class="page-grid">
    <PageHeader
      eyebrow="CLI 令牌池"
      title="凭据管理"
      :description="admin.activeHandler.value ? `${admin.activeHandler.value.label} 当前共有 ${admin.activeHandler.value.credentials_total || 0} 个凭据。` : '导入、筛选和批量处理各 CLI 的凭据。'"
    >
      <template #actions>
        <AdminButton variant="secondary" :disabled="loading" @click="loadCredentials(page, pageSize)">
          {{ loading ? '刷新中...' : '刷新列表' }}
        </AdminButton>
        <AdminButton v-if="admin.activeHandler.value?.supports_credentials" @click="importOpen = true">
          导入凭据
        </AdminButton>
      </template>
    </PageHeader>

    <SectionCard title="处理器" eyebrow="切换">
      <div class="chip-row">
        <button
          v-for="handler in admin.handlers.value"
          :key="handler.key"
          type="button"
          :class="['chip', { 'is-active': admin.selectedHandler.value === handler.key }]"
          @click="admin.selectedHandler.value = handler.key"
        >
          {{ handler.label }}
        </button>
      </div>

      <div v-if="admin.activeHandler.value" class="handler-summary">
        <div>
          <h3>{{ admin.activeHandler.value.label }}</h3>
          <p>{{ admin.activeHandler.value.summary || '暂无说明' }}</p>
        </div>
        <div class="handler-summary-meta">
          <AdminBadge :tone="toneForStatus(admin.activeHandler.value.status)">
            {{ statusText(admin.activeHandler.value.status) }}
          </AdminBadge>
          <span>接口 {{ apiTypesText(admin.activeHandler.value.supported_api_types) }}</span>
          <span>{{ admin.activeHandler.value.credentials_total || 0 }} 个凭据</span>
          <span>{{ admin.activeHandler.value.models_total || 0 }} 个模型</span>
        </div>
      </div>
    </SectionCard>

    <SectionCard title="凭据列表" :eyebrow="admin.activeHandler.value?.label || '当前处理器'">
      <template #actions>
        <div class="toolbar-inline">
          <input v-model="search" class="search-input" placeholder="搜索凭据 ID / 状态 / 套餐">
          <select
            class="select-input"
            :value="pageSize"
            @change="loadCredentials(1, Number(($event.target as HTMLSelectElement).value))"
          >
            <option :value="25">25 条 / 页</option>
            <option :value="50">50 条 / 页</option>
            <option :value="100">100 条 / 页</option>
          </select>
        </div>
      </template>

      <template v-if="admin.activeHandler.value?.supports_credentials">
        <div v-if="selectedIds.length" class="batch-bar">
          <span>已选择 {{ selectedIds.length }} 条</span>
          <AdminButton variant="ghost" size="sm" :disabled="actionBusy" @click="batchSetStatus('enabled')">批量启用</AdminButton>
          <AdminButton variant="ghost" size="sm" :disabled="actionBusy" @click="batchSetStatus('disabled')">批量停用</AdminButton>
          <AdminButton variant="danger" size="sm" :disabled="actionBusy" @click="batchDelete">批量删除</AdminButton>
        </div>

        <div v-if="filteredRows.length" class="table-shell">
          <table>
            <thead>
              <tr>
                <th><input type="checkbox" :checked="allVisibleSelected" @change="toggleSelectAll"></th>
                <th>账户</th>
                <th>状态</th>
                <th>套餐</th>
                <th>5 小时配额</th>
                <th>7 天配额</th>
                <th>同步状态</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="item in filteredRows" :key="item.id">
                <td>
                  <input type="checkbox" :checked="selectedSet.has(item.id)" @change="toggleSelectOne(item.id)">
                </td>
                <td>
                  <div class="table-title">{{ item.id }}</div>
                  <div class="table-subtitle">到期 {{ formatTime(item.expired) }}</div>
                </td>
                <td>
                  <AdminBadge :tone="toneForStatus(item.status)">
                    {{ statusText(item.status) }}
                  </AdminBadge>
                </td>
                <td>
                  <div class="table-title">{{ planTypeText(item.plan_type) }}</div>
                  <div class="table-subtitle">{{ formatTime(item.plan_expired) }}</div>
                </td>
                <td>{{ renderQuotaValue(item, 'quota_5h', 'reset_5h') }}</td>
                <td>{{ renderQuotaValue(item, 'quota_7d', 'reset_7d') }}</td>
                <td>
                  <template v-if="isUnsynced(item.synced_at)">
                    <span class="text-muted">未同步</span>
                  </template>
                  <template v-else>
                    <div class="table-title">{{ formatTime(item.synced_at) }}</div>
                    <div class="table-subtitle">退避到 {{ formatTime(item.throttled_until) }}</div>
                  </template>
                </td>
              </tr>
            </tbody>
          </table>
        </div>

        <EmptyState
          v-else
          title="当前处理器还没有凭据"
          description="导入令牌后，就可以在这里查看配额与同步状态。"
        >
          <template #action>
            <AdminButton @click="importOpen = true">导入凭据</AdminButton>
          </template>
        </EmptyState>

        <div class="pager">
          <AdminButton variant="ghost" size="sm" :disabled="page <= 1 || loading" @click="loadCredentials(Math.max(1, page - 1), pageSize)">
            上一页
          </AdminButton>
          <span>第 {{ page }} / {{ maxPage }} 页</span>
          <AdminButton variant="ghost" size="sm" :disabled="page >= maxPage || loading" @click="loadCredentials(Math.min(maxPage, page + 1), pageSize)">
            下一页
          </AdminButton>
        </div>
      </template>

      <EmptyState
        v-else
        title="该处理器暂不支持凭据导入"
        description="可以切换到其他处理器继续管理，或前往模型页面查看能力映射。"
      />
    </SectionCard>

    <ModalDialog :open="importOpen" :title="admin.activeHandler.value ? `导入 ${admin.activeHandler.value.label} 凭据` : '导入凭据'" @close="closeImportModal">
      <div class="form-grid">
        <label class="form-field">
          <span>令牌列表<small>一行一个</small></span>
          <textarea v-model="importTokens" rows="8" placeholder="每行填写一个 Refresh Token 或 Access Token" />
          <small>自动识别 Refresh Token 或 Access Token。</small>
        </label>
        <div v-if="importError" class="form-error" style="white-space: pre-wrap">{{ importError }}</div>
      </div>
      <template #footer>
        <AdminButton variant="ghost" @click="closeImportModal">取消</AdminButton>
        <AdminButton :disabled="actionBusy" @click="createCredential">
          {{ actionBusy ? '导入中...' : '开始导入' }}
        </AdminButton>
      </template>
    </ModalDialog>

    <ModalDialog :open="confirmOpen" :title="confirmTitle" @close="closeConfirm">
      <p>{{ confirmMessage }}</p>
      <template #footer>
        <AdminButton variant="ghost" :disabled="actionBusy" @click="closeConfirm">取消</AdminButton>
        <AdminButton :variant="confirmVariant === 'secondary' ? 'secondary' : 'danger'" :disabled="actionBusy" @click="submitConfirm">
          {{ confirmText }}
        </AdminButton>
      </template>
    </ModalDialog>
  </div>
</template>
