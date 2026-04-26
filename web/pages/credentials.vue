<script setup lang="ts">
import { adminApi } from '~/composables/useAdminApi'
import {
  CREDENTIAL_PAGE_SIZE_OPTIONS,
  codexCredentialAccountID,
  codexCredentialEmail,
  formatPercent,
  formatTime,
  isPastTime,
  isZeroTime,
  normalizePlanType,
  planTypeText,
  statusText,
  toneForStatus,
} from '~/lib/admin'
import type {
  CredentialHandlerKey,
  CodexItem,
  CredentialItem,
  GeminiCredentialItem,
  UiTone,
} from '~/types/admin'

definePageMeta({
  navKey: 'credentials',
})

const admin = useAdminApp()
const confirm = useConfirmDialog()
const importJobs = useImportJobs()

const rows = ref<CredentialItem[]>([])
const rowsHandlerKey = ref('')
const total = ref(0)
const page = ref(1)
const pageSize = ref(6)
const loading = ref(false)
const searchInput = ref('')
const searchQuery = ref('')
const statusFilter = ref<'all' | 'enabled' | 'disabled'>('all')
const planFilter = ref('all')
const sortBy = ref('')
const sortOrder = ref<'desc' | 'asc'>('desc')
const selectedIds = ref<string[]>([])
const actionBusy = ref(false)

const importOpen = ref(false)
const importTokens = ref('')
const importError = ref('')

const credentialHandlerKey = computed<CredentialHandlerKey>(() => admin.activeHandler.value?.key || '')
const credentialEndpoint = computed(() => admin.activeHandler.value?.credential_endpoint || '')
const activeHandlerLabel = computed(() => admin.activeHandler.value?.label || '当前处理器')
const activeCredentialField = computed(() => (
  admin.activeHandler.value?.credential_fields?.find((field) => field.preferred)
  || admin.activeHandler.value?.credential_fields?.[0]
  || null
))
const isCodexHandler = computed(() => credentialHandlerKey.value === 'codex')
const isGeminiHandler = computed(() => credentialHandlerKey.value === 'gemini')
const knownStatusOptions = ['enabled', 'disabled'] as const

const codexRows = computed(() => rows.value.filter(isCodexItem))
const geminiRows = computed(() => rows.value.filter(isGeminiItem))
const genericRows = computed(() => rows.value.filter((item) => !isCodexItem(item) && !isGeminiItem(item)))
const rowsMatchActiveHandler = computed(() => rowsHandlerKey.value === credentialHandlerKey.value)
const showHandlerLoadingState = computed(() => (
  Boolean(admin.activeHandler.value?.supports_credentials)
  && (!rowsMatchActiveHandler.value || (loading.value && !rows.value.length))
))

const importLines = computed(() => (
  importTokens.value
    .split('\n')
    .map((line) => line.trim())
    .filter(Boolean)
))
const importInputLabel = computed(() => activeCredentialField.value?.label || '凭据列表')
const importInputPlaceholder = computed(() => activeCredentialField.value?.placeholder || '每行填写一个凭据')

const availablePlanTypes = computed(() => {
  const planTypes = new Set<string>()
  admin.activeHandler.value?.plan_list?.forEach((plan) => {
    const planType = normalizePlanType(plan)
    if (planType) {
      planTypes.add(planType)
    }
  })
  rows.value.forEach((item) => {
    const planType = normalizePlanType(item.plan_type)
    if (planType) {
      planTypes.add(planType)
    }
  })
  return ['all', ...planTypes]
})

const availableStatusFilters = computed(() => {
  const statuses = new Set<string>(knownStatusOptions)
  admin.activeHandler.value?.credential_status_options?.forEach((status) => {
    if (status === 'enabled' || status === 'disabled') {
      statuses.add(status)
    }
  })
  rows.value.forEach((item) => {
    if (item.status === 'enabled' || item.status === 'disabled') {
      statuses.add(item.status)
    }
  })

  const filters: Array<{ value: typeof statusFilter.value, label: string }> = [
    { value: 'all', label: '全部状态' },
  ]
  if (statuses.has('enabled')) {
    filters.push({ value: 'enabled', label: '启用' })
  }
  if (statuses.has('disabled')) {
    filters.push({ value: 'disabled', label: '停用' })
  }
  return filters
})

const defaultSortOption = { title: '默认', value: '' }

const codexSortOptions = [
  { title: 'Score', value: 'default_score' },
  { title: 'Spark Score', value: 'spark_score' },
  { title: '5h额度', value: 'default_quota_5h' },
  { title: '7d额度', value: 'default_quota_7d' },
  { title: '错误率', value: 'default_error_rate' },
  { title: 'Spark错误率', value: 'spark_error_rate' },
  { title: 'Spark 5h额度', value: 'spark_quota_5h' },
  { title: 'Spark 7d额度', value: 'spark_quota_7d' },
  { title: '退避截止', value: 'throttled_until' },
]

const geminiSortOptions = [
  { title: 'Pro Score', value: 'pro_score' },
  { title: 'Pro错误率', value: 'pro_error_rate' },
  { title: 'Pro额度', value: 'pro_quota' },
  { title: 'Flash Score', value: 'flash_score' },
  { title: 'Flash错误率', value: 'flash_error_rate' },
  { title: 'Flash额度', value: 'flash_quota' },
  { title: 'Lite Score', value: 'flashlite_score' },
  { title: 'Lite错误率', value: 'flashlite_error_rate' },
  { title: 'Lite额度', value: 'flashlite_quota' },
  { title: '退避截止', value: 'throttled_until' },
]

const sortOrderOptions = [
  { title: '降序', value: 'desc' },
  { title: '升序', value: 'asc' },
]

const credentialSortOptions = computed(() => [
  defaultSortOption,
  ...(isGeminiHandler.value ? geminiSortOptions : codexSortOptions),
])

const hasActiveFilters = computed(() => (
  Boolean(searchInput.value.trim())
  || statusFilter.value !== 'all'
  || planFilter.value !== 'all'
))
const emptyStateTitle = computed(() => {
  if (hasActiveFilters.value) {
    return '当前条件下没有匹配的凭据'
  }
  return `还没有可管理的 ${activeHandlerLabel.value} 凭据`
})
const emptyStateDescription = computed(() => {
  if (hasActiveFilters.value) {
    return isGeminiHandler.value
      ? '调整搜索、状态或套餐筛选，或者先新增一组 Gemini CLI 凭据'
      : '调整搜索、状态或套餐筛选，或者先导入新凭据'
  }
  return activeCredentialField.value?.help_text || '先导入一批凭据，系统才会开始调度和额度同步'
})
const selectedSet = computed(() => new Set(selectedIds.value))
const allVisibleSelected = computed(() => (
  rows.value.length > 0 && rows.value.every((item) => selectedSet.value.has(item.id))
))
const maxPage = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)))
const pageSizeOptions = CREDENTIAL_PAGE_SIZE_OPTIONS
const importDescription = computed(() => (
  activeCredentialField.value?.help_text || '一行一个凭据，保存后会纳入当前处理器调度'
))

let searchTimer: ReturnType<typeof setTimeout> | undefined
let latestLoadToken = 0

function isCodexItem(item: CredentialItem): item is CodexItem {
  return item.handler === 'codex'
}

function isGeminiItem(item: CredentialItem): item is GeminiCredentialItem {
  return item.handler === 'gemini'
}

function genericDetailEntries(item: CredentialItem) {
  return [
    { label: '凭据 ID', value: item.id },
    { label: '套餐类型', value: planTypeText(item.plan_type || '') },
    { label: 'AT到期', value: item.expired ? formatTime(String(item.expired)) : '-' },
    { label: '最近同步', value: item.synced_at ? formatTime(String(item.synced_at)) : '-' },
    { label: '退避截止', value: item.throttled_until && !isPastTime(String(item.throttled_until)) ? formatTime(String(item.throttled_until)) : '-' },
  ].filter((entry) => entry.value && entry.value !== 'unknown')
}

function closeImportModal() {
  importOpen.value = false
  importTokens.value = ''
  importError.value = ''
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

function codexQuotaPercentValue(metric: CodexItem['default'], quotaKey: 'quota_5h' | 'quota_7d', resetKey: 'reset_5h' | 'reset_7d') {
  if (metric[quotaKey] === 1 && isZeroTime(metric[resetKey])) {
    return null
  }
  return Math.max(0, Math.min(100, Math.round((metric[quotaKey] || 0) * 100)))
}

function geminiQuotaPercentValue(metric: GeminiCredentialItem['pro']) {
  if (metric.quota === 1 && isZeroTime(metric.reset)) {
    return null
  }
  return Math.max(0, Math.min(100, Math.round((metric.quota || 0) * 100)))
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

function renderCodexQuotaValue(metric: CodexItem['default'], quotaKey: 'quota_5h' | 'quota_7d', resetKey: 'reset_5h' | 'reset_7d') {
  if (metric[quotaKey] === 1 && isZeroTime(metric[resetKey])) {
    return '不适用'
  }
  return formatPercent(metric[quotaKey])
}

function renderGeminiQuotaValue(metric: GeminiCredentialItem['pro']) {
  if (metric.quota === 1 && isZeroTime(metric.reset)) {
    return '不适用'
  }
  return formatPercent(metric.quota)
}

function codexQuotaCard(label: string, metric: CodexItem['default'], quotaKey: 'quota_5h' | 'quota_7d', resetKey: 'reset_5h' | 'reset_7d') {
  const percent = codexQuotaPercentValue(metric, quotaKey, resetKey)
  return {
    label,
    percent,
    tone: quotaTone(percent),
    value: renderCodexQuotaValue(metric, quotaKey, resetKey),
    reset: metric[resetKey],
  }
}

function codexQuotaCards(item: CodexItem) {
  const cards = [
    codexQuotaCard('5 小时额度', item.default, 'quota_5h', 'reset_5h'),
    codexQuotaCard('7 天额度', item.default, 'quota_7d', 'reset_7d'),
  ]
  if (isSparkAvailable(item)) {
    cards.push(
      codexQuotaCard('Spark 5h', item.spark, 'quota_5h', 'reset_5h'),
      codexQuotaCard('Spark 7d', item.spark, 'quota_7d', 'reset_7d'),
    )
  }
  return cards
}

function geminiQuotaCard(label: string, metric: GeminiCredentialItem['pro']) {
  const percent = geminiQuotaPercentValue(metric)
  return {
    label,
    percent,
    tone: quotaTone(percent),
    value: renderGeminiQuotaValue(metric),
    reset: metric.reset,
  }
}

function geminiQuotaCards(item: GeminiCredentialItem) {
  return [
    geminiQuotaCard('Pro 额度', item.pro),
    geminiQuotaCard('Flash 额度', item.flash),
    geminiQuotaCard('Lite 额度', item.flashlite),
  ]
}

function isSparkAvailable(item: CodexItem) {
  return item.spark.available
}

function errorRateTone(rate: number): UiTone {
  if (rate <= 0) return 'success'
  if (rate < 0.2) return 'warning'
  return 'danger'
}

function errorRateFromWeight(metric: { weight: number }) {
  return Math.max(0, Math.min(1, 1 - metric.weight))
}

function statusIcon(status: string) {
  return status === 'enabled' ? 'mdi-check' : 'mdi-close-circle'
}

function formatScore(value: number) {
  if (typeof value !== 'number' || Number.isNaN(value)) {
    return '-'
  }
  return value.toFixed(2)
}

function scoreBadgeLabel(metric: { score: number; weight: number }) {
  return `Score: ${formatScore(metric.score)}(${formatPercent(errorRateFromWeight(metric))})`
}

function currentQueryOptions(nextPage = page.value, nextPageSize = pageSize.value) {
  const search = searchQuery.value.trim()
  return {
    page: nextPage,
    pageSize: nextPageSize,
    search: search || undefined,
    status: statusFilter.value === 'enabled' || statusFilter.value === 'disabled'
      ? statusFilter.value
      : undefined,
    planType: planFilter.value !== 'all' ? planFilter.value : undefined,
    sortBy: sortBy.value || undefined,
    sortOrder: sortOrder.value,
  }
}

async function loadCredentials(nextPage = page.value, nextPageSize = pageSize.value) {
  const requestToken = ++latestLoadToken
  const handlerKey = credentialHandlerKey.value
  const endpoint = credentialEndpoint.value
  const supportsCredentials = Boolean(admin.activeHandler.value?.supports_credentials)

  if (!admin.token.value || !handlerKey || !supportsCredentials) {
    rows.value = []
    rowsHandlerKey.value = handlerKey
    total.value = 0
    page.value = 1
    selectedIds.value = []
    loading.value = false
    return
  }

  loading.value = true
  try {
    const data = await adminApi.queryCredentials(admin.token.value, endpoint, currentQueryOptions(nextPage, nextPageSize))
    if (requestToken !== latestLoadToken) {
      return
    }
    rows.value = data.data
    rowsHandlerKey.value = handlerKey
    total.value = data.total
    page.value = data.page
    pageSize.value = data.page_size
    selectedIds.value = []
  } catch (error) {
    if (requestToken === latestLoadToken) {
      rows.value = []
      rowsHandlerKey.value = handlerKey
      total.value = 0
      selectedIds.value = []
      admin.notify(error instanceof Error ? error.message : '加载凭据失败', 'danger')
    }
  } finally {
    if (requestToken === latestLoadToken) {
      loading.value = false
    }
  }
}

async function createCredential() {
  actionBusy.value = true
  importError.value = ''

  try {
    if (importLines.value.length === 0) {
      importError.value = '请至少输入一行令牌'
      return
    }

    const job = await adminApi.importCredentials(admin.token.value, credentialEndpoint.value, {
      tokens: importLines.value,
    })

    importJobs.add(job)
    importJobs.ensurePolling(admin.token.value)
    closeImportModal()
    admin.notify(`导入任务已提交：${job.total} 条凭据`, 'success')
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
        const result = await adminApi.updateCredentialStatus(admin.token.value, credentialEndpoint.value, { ids, status })
        const updatedCount = result.updated.length
        const errorCount = result.errors.length
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
    message: `确认删除 ${ids.length} 个凭据吗？此操作不可撤销`,
    confirmText: '确认删除',
    action: async () => {
      actionBusy.value = true
      try {
        const result = await adminApi.deleteCredentials(admin.token.value, credentialEndpoint.value, { ids })
        const deletedCount = result.deleted.length
        const errorCount = result.errors.length
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
  { immediate: true },
)

watch(
  () => admin.selectedHandler.value,
  () => {
    statusFilter.value = 'all'
    planFilter.value = 'all'
    sortBy.value = ''
    sortOrder.value = 'desc'
    searchInput.value = ''
    searchQuery.value = ''
    if (!admin.activeHandler.value?.supports_credentials) {
      void loadCredentials(1, pageSize.value)
    }
  },
)

watch(
  () => [searchQuery.value, statusFilter.value, planFilter.value, sortBy.value, sortOrder.value, credentialHandlerKey.value],
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
      title="凭据管理"
      icon="mdi-key-chain-variant"
    >
      <template #meta>
        <AdminBadge tone="secondary" icon="mdi-key-chain-variant">
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
          导入凭据
        </AdminButton>
      </template>
    </PageHeader>

    <SectionCard
      title="后端服务"
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
      icon="mdi-table-large"
    >
      <Transition name="handler-content-fade" mode="out-in">
        <div
          v-if="showHandlerLoadingState"
          :key="`loading-${credentialHandlerKey}`"
          class="credentials-switch-loading"
          aria-live="polite"
        >
          <VProgressCircular
            indeterminate
            color="primary"
            size="32"
            width="3"
          />
          <div class="credentials-switch-copy">
            <div class="credentials-switch-title">正在切换到 {{ activeHandlerLabel }}</div>
            <div class="text-body-2 text-medium-emphasis">加载该后端服务的凭据列表</div>
          </div>
        </div>

        <div
          v-else-if="admin.activeHandler.value?.supports_credentials"
          :key="`credentials-${credentialHandlerKey}`"
          class="d-grid ga-5"
        >
          <div class="toolbar-panel">
            <VProgressLinear
              :active="loading"
              :model-value="loading ? 100 : 0"
              indeterminate
              color="primary"
              class="credentials-inline-progress"
            />
            <div class="filter-toolbar">
              <VTextField
                v-model="searchInput"
                class="filter-grow"
                label="搜索"
                :placeholder="isGeminiHandler ? '凭据 ID / 邮箱 / 状态' : isCodexHandler ? '邮箱 / Account ID / 状态 / 套餐' : '凭据 ID / 状态 / 套餐'"
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
              <VSelect
                v-model="sortBy"
                class="filter-select"
                label="排序"
                :items="credentialSortOptions"
              />
              <VSelect
                v-model="sortOrder"
                class="filter-select"
                label="排序方向"
                :items="sortOrderOptions"
              />
            </div>

            <VChipGroup v-model="statusFilter" mandatory color="primary">
              <VChip
                v-for="status in availableStatusFilters"
                :key="status.value"
                :value="status.value"
                filter
              >
                {{ status.label }}
              </VChip>
            </VChipGroup>

            <VChipGroup v-if="availablePlanTypes.length > 1 || planFilter !== 'all'" v-model="planFilter" mandatory color="secondary">
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
                          <div class="stack-card-title">{{ codexCredentialEmail(item.id) }}</div>
                          <div class="stack-card-meta">
                            <AdminBadge tone="secondary" subtle icon="mdi-star-circle-outline">
                              {{ planTypeText(item.plan_type) }}
                            </AdminBadge>
                            <AdminBadge :tone="toneForStatus(item.status)" subtle :icon="statusIcon(item.status)">
                              {{ statusText(item.status) }}
                            </AdminBadge>
                            <AdminBadge :tone="errorRateTone(errorRateFromWeight(item.default))" subtle icon="mdi-chart-line">
                              {{ scoreBadgeLabel(item.default) }}
                            </AdminBadge>
                            <AdminBadge v-if="isSparkAvailable(item)" :tone="errorRateTone(errorRateFromWeight(item.spark))" subtle icon="mdi-chart-line">
                              Spark {{ scoreBadgeLabel(item.spark) }}
                            </AdminBadge>
                            <AdminBadge v-else tone="secondary" subtle icon="mdi-cancel">
                              Spark不可用
                            </AdminBadge>
                          </div>
                        </div>
                      </div>
                    </div>

                    <div class="quota-grid">
                      <div v-for="quota in codexQuotaCards(item)" :key="quota.label" class="quota-card">
                        <div class="quota-row">
                          <div class="quota-label text-medium-emphasis">{{ quota.label }}</div>
                          <span :class="'text-' + quota.tone" class="quota-value font-weight-bold">
                            {{ quota.value }}
                          </span>
                        </div>
                        <VProgressLinear
                          :model-value="quota.percent ?? 0"
                          :color="quota.tone"
                          rounded
                          height="8"
                        />
                        <div class="quota-caption text-medium-emphasis">
                          重置 {{ formatTime(quota.reset) }}
                        </div>
                      </div>
                    </div>

                    <div class="detail-grid">
                      <div class="detail-block">
                        <div class="detail-label text-medium-emphasis">AT到期</div>
                        <div class="detail-value">{{ formatTime(item.expired) }}</div>
                      </div>
                      <div class="detail-block">
                        <div class="detail-label text-medium-emphasis">Account ID</div>
                        <div class="detail-value">{{ codexCredentialAccountID(item.id) }}</div>
                      </div>
                      <div class="detail-block">
                        <div class="detail-label text-medium-emphasis">最近同步</div>
                        <div class="detail-value">{{ formatTime(item.synced_at) }}</div>
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

              <template v-else-if="isGeminiHandler">
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
                            <AdminBadge tone="secondary" subtle icon="mdi-star-circle-outline">
                              {{ planTypeText(item.plan_type) }}
                            </AdminBadge>
                            <AdminBadge :tone="toneForStatus(item.status)" subtle :icon="statusIcon(item.status)">
                              {{ statusText(item.status) }}
                            </AdminBadge>
                            <AdminBadge :tone="errorRateTone(errorRateFromWeight(item.pro))" subtle icon="mdi-chart-line">
                              Pro {{ scoreBadgeLabel(item.pro) }}
                            </AdminBadge>
                            <AdminBadge :tone="errorRateTone(errorRateFromWeight(item.flash))" subtle icon="mdi-chart-line">
                              Flash {{ scoreBadgeLabel(item.flash) }}
                            </AdminBadge>
                            <AdminBadge :tone="errorRateTone(errorRateFromWeight(item.flashlite))" subtle icon="mdi-chart-line">
                              Lite {{ scoreBadgeLabel(item.flashlite) }}
                            </AdminBadge>
                          </div>
                        </div>
                      </div>
                    </div>

                    <div class="quota-grid">
                      <div v-for="quota in geminiQuotaCards(item)" :key="quota.label" class="quota-card">
                        <div class="quota-row">
                          <div class="quota-label text-medium-emphasis">{{ quota.label }}</div>
                          <span :class="'text-' + quota.tone" class="quota-value font-weight-bold">
                            {{ quota.value }}
                          </span>
                        </div>
                        <VProgressLinear
                          :model-value="quota.percent ?? 0"
                          :color="quota.tone"
                          rounded
                          height="8"
                        />
                        <div class="quota-caption text-medium-emphasis">
                          重置 {{ formatTime(quota.reset) }}
                        </div>
                      </div>
                    </div>

                    <div class="detail-grid">
                      <div class="detail-block">
                        <div class="detail-label text-medium-emphasis">项目 ID</div>
                        <div class="detail-value">{{ item.project_id || '-' }}</div>
                      </div>
                      <div class="detail-block">
                        <div class="detail-label text-medium-emphasis">AT到期</div>
                        <div class="detail-value">{{ formatTime(item.expired) }}</div>
                      </div>
                      <div class="detail-block">
                        <div class="detail-label text-medium-emphasis">最近同步</div>
                        <div class="detail-value">{{ item.synced_at ? formatTime(item.synced_at) : '-' }}</div>
                      </div>
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

              <template v-else>
                <VCard
                  v-for="item in genericRows"
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
                            <AdminBadge v-if="item.plan_type" tone="secondary" subtle icon="mdi-star-circle-outline">
                              {{ planTypeText(item.plan_type) }}
                            </AdminBadge>
                            <AdminBadge :tone="toneForStatus(item.status)" subtle :icon="statusIcon(item.status)">
                              {{ statusText(item.status) }}
                            </AdminBadge>
                          </div>
                        </div>
                      </div>
                    </div>

                    <div class="detail-grid">
                      <div
                        v-for="entry in genericDetailEntries(item)"
                        :key="entry.label"
                        class="detail-block"
                      >
                        <div class="detail-label text-medium-emphasis">{{ entry.label }}</div>
                        <div class="detail-value">{{ entry.value }}</div>
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
                导入凭据
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
        </div>

        <EmptyState
          v-else
          :key="`unsupported-${credentialHandlerKey}`"
          title="该处理器暂不支持凭据导入"
          description="可以切换到其他处理器，或前往模型页面查看映射能力"
          icon="mdi-key-remove"
        />
      </Transition>
    </SectionCard>

    <ModalDialog
      :open="importOpen"
      :title="admin.activeHandler.value ? `导入 ${admin.activeHandler.value.label} 凭据` : '导入凭据'"
      :description="importDescription"
      max-width="720"
      @close="closeImportModal"
    >
      <div class="d-grid ga-4">
        <VTextarea
          v-model="importTokens"
          rows="8"
          :label="importInputLabel"
          :placeholder="importInputPlaceholder"
          prepend-inner-icon="mdi-text-box-plus-outline"
        />

        <div class="d-flex flex-wrap ga-2">
          <AdminBadge tone="secondary" subtle icon="mdi-text-box-plus-outline">
            待导入 {{ importLines.length }} 条
          </AdminBadge>
          <AdminBadge v-if="activeCredentialField?.help_text" tone="neutral" subtle icon="mdi-information-outline">
            {{ activeCredentialField.help_text }}
          </AdminBadge>
        </div>

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
          开始导入
        </AdminButton>
      </template>
    </ModalDialog>

    <ModalDialog
      :open="confirm.open.value"
      :title="confirm.title.value"
      description="操作会立即提交到后台"
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
