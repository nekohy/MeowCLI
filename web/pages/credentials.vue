<script setup lang="ts">
import { adminApi } from '~/composables/useAdminApi'
import {
  PAGE_SIZE_OPTIONS,
  formatPercent,
  formatTime,
  isPastTime,
  isUnsynced,
  isZeroTime,
  normalizePlanType,
  planTypeText,
  statusText,
  toneForStatus,
} from '~/lib/admin'
import type {
  CredentialHandlerKey,
  CodexItem,
  CredentialField,
  CredentialItem,
  GeminiCredentialInput,
  GeminiCredentialItem,
} from '~/types/admin'

definePageMeta({
  navKey: 'credentials',
})

const admin = useAdminApp()
const confirm = useConfirmDialog()

const rows = ref<CredentialItem[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(25)
const loading = ref(false)
const searchInput = ref('')
const searchQuery = ref('')
const statusFilter = ref<'all' | 'enabled' | 'disabled' | 'unsynced'>('all')
const planFilter = ref('all')
const selectedIds = ref<string[]>([])
const actionBusy = ref(false)

const importOpen = ref(false)
const importTokens = ref('')
const importError = ref('')
const importForm = ref<Record<string, string>>({})

const credentialHandlerKey = computed<CredentialHandlerKey>(() => (
  admin.activeHandler.value?.key === 'gemini' ? 'gemini' : 'codex'
))
const isCodexHandler = computed(() => credentialHandlerKey.value === 'codex')
const isGeminiHandler = computed(() => credentialHandlerKey.value === 'gemini')
const activeCredentialFields = computed(() => admin.activeHandler.value?.credential_fields || [])

const codexRows = computed(() => rows.value.filter(isCodexItem))
const geminiRows = computed(() => rows.value.filter((item): item is GeminiCredentialItem => !isCodexItem(item)))

const importLines = computed(() => (
  importTokens.value
    .split('\n')
    .map((line) => line.trim())
    .filter(Boolean)
))

const summaryTiles = computed(() => {
  if (isGeminiHandler.value) {
    return [
      {
        label: '当前页',
        value: geminiRows.value.length,
        helper: '本次已加载的凭据数',
        icon: 'mdi-file-document-outline',
      },
      {
        label: '可用',
        value: geminiRows.value.filter((item) => item.status === 'enabled').length,
        helper: '启用且可参与调度',
        icon: 'mdi-check-circle-outline',
      },
      {
        label: '冷却中',
        value: geminiRows.value.filter((item) => item.throttled_until && !isPastTime(item.throttled_until)).length,
        helper: '当前被退避的凭据数',
        icon: 'mdi-timer-sand',
      },
      {
        label: '已选择',
        value: selectedIds.value.length,
        icon: 'mdi-checkbox-multiple-marked-outline',
      },
    ]
  }

  return [
    {
      label: '当前页',
      value: codexRows.value.length,
      helper: '本次已加载的凭据数',
      icon: 'mdi-file-document-outline',
    },
    {
      label: '可用',
      value: codexRows.value.filter((item) => item.status === 'enabled').length,
      helper: '启用且可参与调度',
      icon: 'mdi-check-circle-outline',
    },
    {
      label: '未同步',
      value: codexRows.value.filter((item) => isUnsynced(item.synced_at)).length,
      helper: '需要重新同步额度',
      icon: 'mdi-sync-alert',
    },
    {
      label: '已选择',
      value: selectedIds.value.length,
      icon: 'mdi-checkbox-multiple-marked-outline',
    },
  ]
})

const availablePlanTypes = computed(() => {
  const configured = admin.activeHandler.value?.plan_list || []
  if (configured.length) {
    return ['all', ...configured]
  }
  const planTypes = new Set<string>()
  rows.value.forEach((item) => {
    const planType = normalizePlanType(item.plan_type)
    if (planType) {
      planTypes.add(planType)
    }
  })
  return ['all', ...planTypes]
})

const hasActiveFilters = computed(() => (
  Boolean(searchInput.value.trim())
  || statusFilter.value !== 'all'
  || planFilter.value !== 'all'
))
const emptyStateTitle = computed(() => {
  if (hasActiveFilters.value) {
    return '当前条件下没有匹配的凭据'
  }
  return isGeminiHandler.value ? '还没有可管理的 Gemini CLI 凭据' : '还没有可管理的凭据'
})
const emptyStateDescription = computed(() => {
  if (hasActiveFilters.value) {
    return isGeminiHandler.value
      ? '调整搜索、状态或套餐筛选，或者先新增一组 Gemini CLI 凭据。'
      : '调整搜索、状态或套餐筛选，或者先导入新凭据。'
  }
  return isGeminiHandler.value
    ? '先手工导入 refresh token，系统会在保存后自动纳入调度。'
    : '先导入一批凭据，系统才会开始调度和额度同步。'
})
const selectedSet = computed(() => new Set(selectedIds.value))
const allVisibleSelected = computed(() => (
  rows.value.length > 0 && rows.value.every((item) => selectedSet.value.has(item.id))
))
const maxPage = computed(() => Math.max(1, Math.ceil((total.value || 0) / (pageSize.value || 25))))
const pageSizeOptions = PAGE_SIZE_OPTIONS
const importDescription = computed(() => (
  isGeminiHandler.value
    ? '手工导入一组 Gemini CLI refresh token，保存后即可加入凭据池。'
    : '一行一个令牌，支持 Refresh Token 或 Access Token。'
))

let searchTimer: ReturnType<typeof setTimeout> | undefined
let latestLoadToken = 0

function isCodexItem(item: CredentialItem): item is CodexItem {
  return item.handler === 'codex'
}

function resetImportForm() {
  importForm.value = Object.fromEntries(
    activeCredentialFields.value.map((field) => [field.key, '']),
  )
}

function closeImportModal() {
  importOpen.value = false
  importTokens.value = ''
  importError.value = ''
  resetImportForm()
}

function toggleSelectAll() {
  if (allVisibleSelected.value) {
    selectedIds.value = []
    return
  }
  selectedIds.value = rows.value.map((item) => item.id)
}

function toggleSelectOne(id: string) {
  selectedIds.value = selectedSet.value.has(id)
    ? selectedIds.value.filter((value) => value !== id)
    : [...selectedIds.value, id]
}

function quotaPercentValue(item: CodexItem, quotaKey: 'quota_5h' | 'quota_7d', resetKey: 'reset_5h' | 'reset_7d') {
  if (isUnsynced(item.synced_at)) {
    return null
  }
  if (item[quotaKey] === 1 && isZeroTime(item[resetKey])) {
    return null
  }
  return Math.max(0, Math.min(100, Math.round((item[quotaKey] || 0) * 100)))
}

function quotaTone(percent: number | null) {
  if (percent === null) {
    return 'secondary'
  }
  if (percent >= 65) {
    return 'success'
  }
  if (percent >= 30) {
    return 'warning'
  }
  return 'danger'
}

function renderQuotaValue(item: CodexItem, quotaKey: 'quota_5h' | 'quota_7d', resetKey: 'reset_5h' | 'reset_7d') {
  if (isUnsynced(item.synced_at)) {
    return '未同步'
  }
  if (item[quotaKey] === 1 && isZeroTime(item[resetKey])) {
    return '不适用'
  }
  return formatPercent(item[quotaKey])
}

function currentQueryOptions(nextPage = page.value, nextPageSize = pageSize.value) {
  return {
    page: nextPage,
    pageSize: nextPageSize,
    search: searchQuery.value.trim(),
    status: statusFilter.value === 'enabled' || statusFilter.value === 'disabled'
      ? statusFilter.value
      : undefined,
    planType: planFilter.value !== 'all' ? planFilter.value : undefined,
    unsynced: isCodexHandler.value && statusFilter.value === 'unsynced' ? true : undefined,
  }
}

async function loadCredentials(nextPage = page.value, nextPageSize = pageSize.value) {
  const requestToken = ++latestLoadToken
  if (!admin.token.value || !admin.activeHandler.value?.supports_credentials) {
    rows.value = []
    total.value = 0
    page.value = 1
    selectedIds.value = []
    loading.value = false
    return
  }

  loading.value = true
  try {
    const data = await adminApi.listCredentials(admin.token.value, credentialHandlerKey.value, currentQueryOptions(nextPage, nextPageSize))
    if (requestToken !== latestLoadToken) {
      return
    }
    rows.value = data.data || []
    total.value = data.total || 0
    page.value = data.page || nextPage
    pageSize.value = data.page_size || nextPageSize
    selectedIds.value = []
  } catch (error) {
    if (requestToken === latestLoadToken) {
      admin.notify(error instanceof Error ? error.message : '加载凭据失败', 'danger')
    }
  } finally {
    if (requestToken === latestLoadToken) {
      loading.value = false
    }
  }
}

function buildGeminiCredentialPayload(): GeminiCredentialInput {
  const next: GeminiCredentialInput = {
    refresh_token: (importForm.value.refresh_token || '').trim(),
  }

  for (const key of ['id'] as const) {
    const value = (importForm.value[key] || '').trim()
    if (value) {
      next[key] = value
    }
  }

  if (!next.refresh_token) {
    throw new Error('请填写 refresh token')
  }

  return next
}

async function createCredential() {
  actionBusy.value = true
  importError.value = ''

  try {
    if (!isGeminiHandler.value && importLines.value.length === 0) {
      importError.value = '请至少输入一行令牌'
      return
    }

    const result = isGeminiHandler.value
      ? await adminApi.createCredentials(admin.token.value, 'gemini', {
          credentials: [buildGeminiCredentialPayload()],
        })
      : await adminApi.createCredentials(admin.token.value, 'codex', {
          tokens: importLines.value,
        })

    const createdCount = result.created?.length || 0
    const errorCount = result.errors?.length || 0

    if (errorCount > 0) {
      const details = result.errors
        .map((entry: { input: string; error: string }) => `${entry.input}：${entry.error}`)
        .join('\n')
      importError.value = `${errorCount} 条失败：\n${details}`
      if (createdCount > 0) {
        admin.notify(`导入完成：${createdCount} 条成功，${errorCount} 条失败`, 'warning')
      }
    } else {
      closeImportModal()
      admin.notify(isGeminiHandler.value ? `已新增 ${createdCount} 条 Gemini CLI 凭据` : `已导入 ${createdCount} 条凭据`)
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

  confirm.show({
    title: `${statusText(status)}凭据`,
    message: `确认将 ${ids.length} 个凭据设为"${statusText(status)}"吗？`,
    confirmText: `确认${statusText(status)}`,
    confirmVariant: 'secondary',
    action: async () => {
      actionBusy.value = true
      try {
        const result = await adminApi.updateCredentialStatus(admin.token.value, credentialHandlerKey.value, { ids, status })
        const updatedCount = result.updated?.length || 0
        const errorCount = result.errors?.length || 0
        admin.notify(
          errorCount > 0
            ? `处理完成：${updatedCount} 条成功，${errorCount} 条失败`
            : `已更新 ${updatedCount} 条凭据`,
          errorCount > 0 ? 'warning' : 'success',
        )
        await Promise.all([
          admin.loadOverview(admin.token.value, true),
          loadCredentials(page.value, pageSize.value),
        ])
      } catch (error) {
        admin.notify(error instanceof Error ? error.message : '更新状态失败', 'danger')
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

  confirm.show({
    title: '删除凭据',
    message: `确认删除 ${ids.length} 个凭据吗？此操作不可撤销。`,
    confirmText: '确认删除',
    action: async () => {
      actionBusy.value = true
      try {
        const result = await adminApi.deleteCredentials(admin.token.value, credentialHandlerKey.value, { ids })
        const deletedCount = result.deleted?.length || 0
        const errorCount = result.errors?.length || 0
        admin.notify(
          errorCount > 0
            ? `删除完成：${deletedCount} 条成功，${errorCount} 条失败`
            : `已删除 ${deletedCount} 条凭据`,
          errorCount > 0 ? 'warning' : 'success',
        )
        await Promise.all([
          admin.loadOverview(admin.token.value, true),
          loadCredentials(1, pageSize.value),
        ])
      } catch (error) {
        admin.notify(error instanceof Error ? error.message : '删除失败', 'danger')
      } finally {
        actionBusy.value = false
      }
    },
  })
}

function fieldInputType(field: CredentialField) {
  switch (field.kind) {
    case 'password':
      return 'password'
    case 'url':
      return 'url'
    default:
      return 'text'
  }
}

onMounted(() => {
  resetImportForm()
  if (admin.authReady.value) {
    void loadCredentials()
  }
})

watch(searchInput, (value) => {
  if (searchTimer) {
    clearTimeout(searchTimer)
  }
  searchTimer = setTimeout(() => {
    searchQuery.value = value.trim()
  }, 250)
})

watch(
  () => admin.authReady.value,
  (ready) => {
    if (ready) {
      void loadCredentials(1, pageSize.value)
    }
  },
)

watch(
  () => activeCredentialFields.value.map((field) => field.key).join(','),
  () => {
    resetImportForm()
  },
)

watch(
  () => admin.selectedHandler.value,
  () => {
    statusFilter.value = 'all'
    planFilter.value = 'all'
    searchInput.value = ''
    searchQuery.value = ''
    resetImportForm()
    if (admin.authReady.value) {
      void loadCredentials(1, pageSize.value)
    }
  },
)

watch(
  () => [searchQuery.value, statusFilter.value, planFilter.value, credentialHandlerKey.value],
  () => {
    if (admin.authReady.value && admin.activeHandler.value?.supports_credentials) {
      void loadCredentials(1, pageSize.value)
    }
  },
)

onBeforeUnmount(() => {
  if (searchTimer) {
    clearTimeout(searchTimer)
  }
})
</script>

<template>
  <div class="page-grid">
    <PageHeader
      eyebrow="令牌池"
      title="凭据管理"
      icon="mdi-key-chain-variant"
    >
      <template #meta>
        <AdminBadge tone="secondary" icon="mdi-counter">
          总量 {{ total }}
        </AdminBadge>
        <AdminBadge v-if="selectedIds.length" tone="accent" icon="mdi-checkbox-multiple-marked-outline">
          已选 {{ selectedIds.length }}
        </AdminBadge>
      </template>
      <template #actions>
        <AdminButton
          v-if="admin.activeHandler.value?.supports_credentials"
          prepend-icon="mdi-import"
          @click="importOpen = true"
        >
          {{ isGeminiHandler ? '新增凭据' : '导入凭据' }}
        </AdminButton>
      </template>
    </PageHeader>

    <SectionCard
      title="服务处理器"
      eyebrow="切换"
      icon="mdi-cpu-64-bit"
    >
      <HandlerSwitchGrid
        :handlers="admin.handlers.value"
        :selected="admin.selectedHandler.value"
        @select="admin.selectedHandler.value = $event"
      />
    </SectionCard>

    <SectionCard
      title="凭据列表"
      :eyebrow="admin.activeHandler.value?.label || '当前处理器'"
      icon="mdi-table-large"
    >
      <div class="d-grid ga-5">
        <template v-if="admin.activeHandler.value?.supports_credentials">
          <div class="summary-grid">
            <MetricCard
              v-for="tile in summaryTiles"
              :key="tile.label"
              :label="tile.label"
              :value="tile.value"
              :helper="tile.helper"
              :icon="tile.icon"
            />
          </div>

          <div class="toolbar-panel">
            <div class="filter-toolbar">
              <VTextField
                v-model="searchInput"
                class="filter-grow"
                label="搜索"
                :placeholder="isGeminiHandler ? '凭据 ID / 邮箱 / 状态' : '凭据 ID / 状态 / 套餐'"
                prepend-inner-icon="mdi-magnify"
                clearable
              />
              <VSelect
                v-model="pageSize"
                class="filter-select"
                label="每页条数"
                :items="pageSizeOptions"
                @update:model-value="(value) => loadCredentials(1, Number(value))"
              />
            </div>

            <VChipGroup v-model="statusFilter" mandatory color="primary">
              <VChip value="all" filter>全部状态</VChip>
              <VChip value="enabled" filter>启用</VChip>
              <VChip value="disabled" filter>停用</VChip>
              <VChip v-if="isCodexHandler" value="unsynced" filter>未同步</VChip>
            </VChipGroup>

            <VChipGroup v-if="availablePlanTypes.length > 1" v-model="planFilter" mandatory color="secondary">
              <VChip value="all" filter>全部套餐</VChip>
              <VChip
                v-for="plan in availablePlanTypes.filter((item) => item !== 'all')"
                :key="plan"
                :value="plan"
                filter
              >
                {{ planTypeText(plan) }}
              </VChip>
            </VChipGroup>
          </div>

          <div v-if="selectedIds.length" class="selection-bar">
            <div class="text-body-1">已选择 {{ selectedIds.length }} 条凭据</div>
            <div class="d-flex flex-wrap ga-2">
              <AdminButton variant="secondary" size="sm" @click="batchSetStatus('enabled')">启用</AdminButton>
              <AdminButton variant="secondary" size="sm" @click="batchSetStatus('disabled')">停用</AdminButton>
              <AdminButton variant="danger" size="sm" @click="batchDelete">删除</AdminButton>
            </div>
          </div>

          <div v-if="rows.length" class="d-grid ga-4">
            <div class="d-flex align-center justify-space-between flex-wrap ga-3">
              <VCheckboxBtn
                :model-value="allVisibleSelected"
                label="选中当前页全部结果"
                @update:model-value="toggleSelectAll"
              />
              <div class="text-body-2 text-medium-emphasis">
                共 {{ total }} 条，当前第 {{ page }} / {{ maxPage }} 页
              </div>
            </div>

            <div class="stack-list">
              <template v-if="isCodexHandler">
                <VCard
                  v-for="item in codexRows"
                  :key="item.id"
                  color="surface-container"
                  variant="flat"
                >
                  <VCardText class="stack-card-body">
                    <div class="stack-card-top">
                      <div class="d-flex align-start ga-3" style="min-width: 0">
                        <VCheckboxBtn
                          :model-value="selectedSet.has(item.id)"
                          @update:model-value="() => toggleSelectOne(item.id)"
                        />
                        <div class="stack-card-copy">
                          <div class="stack-card-title">{{ item.id }}</div>
                          <div class="stack-card-meta">
                            <AdminBadge :tone="toneForStatus(item.status)">
                              {{ statusText(item.status) }}
                            </AdminBadge>
                            <AdminBadge tone="secondary" subtle icon="mdi-star-circle-outline">
                              {{ planTypeText(item.plan_type) }}
                            </AdminBadge>
                            <AdminBadge
                              :tone="isUnsynced(item.synced_at) ? 'warning' : 'success'"
                              subtle
                              icon="mdi-sync"
                            >
                              {{ isUnsynced(item.synced_at) ? '待同步' : '已同步' }}
                            </AdminBadge>
                          </div>
                        </div>
                      </div>
                    </div>

                    <div class="quota-grid">
                      <div class="quota-card">
                        <div class="quota-row">
                          <div class="quota-label text-medium-emphasis">5 小时额度</div>
                          <AdminBadge :tone="quotaTone(quotaPercentValue(item, 'quota_5h', 'reset_5h'))" subtle>
                            {{ renderQuotaValue(item, 'quota_5h', 'reset_5h') }}
                          </AdminBadge>
                        </div>
                        <VProgressLinear
                          :model-value="quotaPercentValue(item, 'quota_5h', 'reset_5h') ?? 0"
                          :color="quotaTone(quotaPercentValue(item, 'quota_5h', 'reset_5h'))"
                          rounded
                          height="10"
                        />
                        <div class="quota-caption text-medium-emphasis">
                          重置时间 {{ formatTime(item.reset_5h) }}
                        </div>
                      </div>

                      <div class="quota-card">
                        <div class="quota-row">
                          <div class="quota-label text-medium-emphasis">7 天额度</div>
                          <AdminBadge :tone="quotaTone(quotaPercentValue(item, 'quota_7d', 'reset_7d'))" subtle>
                            {{ renderQuotaValue(item, 'quota_7d', 'reset_7d') }}
                          </AdminBadge>
                        </div>
                        <VProgressLinear
                          :model-value="quotaPercentValue(item, 'quota_7d', 'reset_7d') ?? 0"
                          :color="quotaTone(quotaPercentValue(item, 'quota_7d', 'reset_7d'))"
                          rounded
                          height="10"
                        />
                        <div class="quota-caption text-medium-emphasis">
                          重置时间 {{ formatTime(item.reset_7d) }}
                        </div>
                      </div>
                    </div>

                    <div class="detail-grid">
                      <div class="detail-block">
                        <div class="detail-label text-medium-emphasis">账号到期</div>
                        <div class="detail-value">{{ formatTime(item.expired) }}</div>
                      </div>
                      <div class="detail-block">
                        <div class="detail-label text-medium-emphasis">套餐到期</div>
                        <div class="detail-value">{{ formatTime(item.plan_expired) }}</div>
                      </div>
                      <div class="detail-block">
                        <div class="detail-label text-medium-emphasis">最近同步</div>
                        <div class="detail-value">{{ isUnsynced(item.synced_at) ? '未同步' : formatTime(item.synced_at) }}</div>
                      </div>
                      <div class="detail-block">
                        <div class="detail-label text-medium-emphasis">退避截止</div>
                        <div class="detail-value">{{ isPastTime(item.throttled_until) ? '-' : formatTime(item.throttled_until) }}</div>
                      </div>
                    </div>

                    <div v-if="item.status === 'disabled' && item.reason" class="reason-block">
                      <div class="reason-label">停用原因</div>
                      <div class="reason-value">{{ item.reason }}</div>
                    </div>
                  </VCardText>
                </VCard>
              </template>

              <template v-else>
                <VCard
                  v-for="item in geminiRows"
                  :key="item.id"
                  color="surface-container"
                  variant="flat"
                >
                  <VCardText class="stack-card-body">
                    <div class="stack-card-top">
                      <div class="d-flex align-start ga-3" style="min-width: 0">
                        <VCheckboxBtn
                          :model-value="selectedSet.has(item.id)"
                          @update:model-value="() => toggleSelectOne(item.id)"
                        />
                        <div class="stack-card-copy">
                          <div class="stack-card-title">{{ item.email || item.id }}</div>
                          <div class="stack-card-meta">
                            <AdminBadge :tone="toneForStatus(item.status)">
                              {{ statusText(item.status) }}
                            </AdminBadge>
                            <AdminBadge tone="secondary" subtle icon="mdi-identifier">
                              {{ item.id }}
                            </AdminBadge>
                            <AdminBadge tone="secondary" subtle icon="mdi-star-circle-outline">
                              {{ planTypeText(item.plan_type) }}
                            </AdminBadge>
                          </div>
                        </div>
                      </div>
                    </div>

                    <div class="detail-grid">
                      <div class="detail-block">
                        <div class="detail-label text-medium-emphasis">账号邮箱</div>
                        <div class="detail-value">{{ item.email || '-' }}</div>
                      </div>
                      <div class="detail-block">
                        <div class="detail-label text-medium-emphasis">项目 ID</div>
                        <div class="detail-value">{{ item.project_id || '-' }}</div>
                      </div>
                    </div>

                    <div class="detail-grid">
                      <div class="detail-block">
                        <div class="detail-label text-medium-emphasis">到期时间</div>
                        <div class="detail-value">{{ formatTime(item.expired) }}</div>
                      </div>
                      <div class="detail-block">
                        <div class="detail-label text-medium-emphasis">最近同步</div>
                        <div class="detail-value">{{ item.synced_at ? formatTime(item.synced_at) : '-' }}</div>
                      </div>
                    </div>

                    <div class="detail-grid">
                      <div class="detail-block">
                        <div class="detail-label text-medium-emphasis">退避截止</div>
                        <div class="detail-value">{{ item.throttled_until && !isPastTime(item.throttled_until) ? formatTime(item.throttled_until) : '-' }}</div>
                      </div>
                    </div>

                    <div v-if="item.status === 'disabled' && item.reason" class="reason-block">
                      <div class="reason-label">停用原因</div>
                      <div class="reason-value">{{ item.reason }}</div>
                    </div>
                  </VCardText>
                </VCard>
              </template>
            </div>
          </div>

          <EmptyState
            v-else
            :title="emptyStateTitle"
            :description="emptyStateDescription"
            icon="mdi-key-plus"
          >
            <template #action>
              <AdminButton prepend-icon="mdi-import" @click="importOpen = true">
                {{ isGeminiHandler ? '新增凭据' : '导入凭据' }}
              </AdminButton>
            </template>
          </EmptyState>

          <div class="pagination-bar">
            <div class="text-body-2 text-medium-emphasis">
              共 {{ total }} 条，当前第 {{ page }} / {{ maxPage }} 页
            </div>
            <VPagination
              :model-value="page"
              :length="maxPage"
              density="comfortable"
              total-visible="7"
              @update:model-value="(value) => loadCredentials(Number(value), pageSize)"
            />
          </div>
        </template>

        <EmptyState
          v-else
          title="该处理器暂不支持凭据导入"
          description="可以切换到其他处理器，或前往模型页面查看映射能力。"
          icon="mdi-key-remove"
        />
      </div>
    </SectionCard>

    <ModalDialog
      :open="importOpen"
      :title="admin.activeHandler.value ? `${isGeminiHandler ? '新增' : '导入'} ${admin.activeHandler.value.label} 凭据` : '导入凭据'"
      :description="importDescription"
      max-width="720"
      @close="closeImportModal"
    >
      <div class="d-grid ga-4">
        <template v-if="isGeminiHandler">
          <div
            v-for="field in activeCredentialFields"
            :key="field.key"
            class="d-grid ga-2"
          >
            <VTextarea
              v-if="field.kind === 'textarea'"
              v-model="importForm[field.key]"
              :rows="field.preferred ? 6 : 4"
              :label="field.label"
              :placeholder="field.placeholder"
            />
            <VTextField
              v-else
              v-model="importForm[field.key]"
              :type="fieldInputType(field)"
              :label="field.label"
              :placeholder="field.placeholder"
              variant="outlined"
              density="comfortable"
            />
            <div v-if="field.help_text" class="text-caption text-medium-emphasis">
              {{ field.help_text }}
            </div>
          </div>
        </template>

        <template v-else>
          <VTextarea
            v-model="importTokens"
            rows="8"
            label="令牌列表"
            placeholder="每行填写一个令牌"
            prepend-inner-icon="mdi-text-box-plus-outline"
          />

          <div class="d-flex flex-wrap ga-2">
            <AdminBadge tone="secondary" subtle icon="mdi-counter">
              待导入 {{ importLines.length }} 条
            </AdminBadge>
            <AdminBadge tone="neutral" subtle icon="mdi-information-outline">
              自动识别令牌类型
            </AdminBadge>
          </div>
        </template>

        <VAlert
          v-if="importError"
          type="error"
          variant="tonal"
          density="comfortable"
          :text="importError"
          style="white-space: pre-wrap"
        />
      </div>
      <template #footer>
        <AdminButton variant="ghost" @click="closeImportModal">取消</AdminButton>
        <AdminButton
          prepend-icon="mdi-arrow-up-bold-circle-outline"
          :loading="actionBusy"
          @click="createCredential"
        >
          {{ isGeminiHandler ? '保存凭据' : '开始导入' }}
        </AdminButton>
      </template>
    </ModalDialog>

    <ModalDialog
      :open="confirm.open.value"
      :title="confirm.title.value"
      description="操作会立即提交到后台。"
      @close="confirm.close()"
    >
      <p class="text-body-1">{{ confirm.message.value }}</p>
      <template #footer>
        <AdminButton variant="ghost" :disabled="actionBusy" @click="confirm.close()">取消</AdminButton>
        <AdminButton
          :variant="confirm.variant.value"
          :loading="actionBusy"
          @click="confirm.submit()"
        >
          {{ confirm.text.value }}
        </AdminButton>
      </template>
    </ModalDialog>
  </div>
</template>
