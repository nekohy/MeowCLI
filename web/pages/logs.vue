<script setup lang="ts">
import { adminApi } from '~/composables/useAdminApi'
import { PAGE_SIZE_OPTIONS } from '~/lib/admin'
import { hasLogError, logItemKey, logMetaItems } from '~/lib/logs'
import type { LogItem, LogStatusCount } from '~/types/admin'

definePageMeta({
  navKey: 'logs',
})

const admin = useAdminApp()

const items = ref<LogItem[]>([])
const total = ref(0)
const summary = ref<{ total: number, status_codes: LogStatusCount[] }>({ total: 0, status_codes: [] })
const page = ref(1)
const pageSize = ref(25)
const loading = ref(false)
const searchInput = ref('')
const searchQuery = ref('')
const handlerFilter = ref('all')
const statusCodeFilter = ref('all')
const autoRefreshStorageKey = 'meowcli-logs-auto-refresh-ms'
const defaultAutoRefreshIntervalMs = 0
const autoRefreshIntervalMs = ref(defaultAutoRefreshIntervalMs)
const autoRefreshOptions = [
  { label: '关闭', value: 0 },
  { label: '5s', value: 5000 },
  { label: '30s', value: 30000 },
  { label: '1m', value: 60000 },
  { label: '5m', value: 300000 },
]

const errorLogsTotal = computed(() =>
  summary.value.status_codes
    .filter((item) => item.status_code >= 400)
    .reduce((total, item) => total + item.total, 0),
)

const summaryTiles = computed(() => [
  {
    label: '总日志',
    value: summary.value.total,
    helper: '保留期内全部记录',
    icon: 'mdi-file-document-outline',
  },
  {
    label: '错误日志',
    value: errorLogsTotal.value,
    helper: '当前条件下出现的错误',
    icon: 'mdi-alert-circle-outline',
  },
  {
    label: '筛选结果',
    value: total.value,
    helper: '应用当前搜索与过滤后',
    icon: 'mdi-filter-check-outline',
  },
])

const maxPage = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)))
const pageSizeOptions = PAGE_SIZE_OPTIONS
const autoRefreshLabel = computed(() => (
  autoRefreshOptions.find((option) => option.value === autoRefreshIntervalMs.value)?.label || '关闭'
))
const statusCodeOptions = computed(() => [
  { value: 'all', label: '全部状态码' },
  ...summary.value.status_codes.map((item) => ({
    value: String(item.status_code),
    label: `${item.status_code} (${item.total})`,
  })),
])
let searchTimer: ReturnType<typeof setTimeout> | undefined
let refreshTimer: number | undefined
let latestLoadToken = 0

function currentQueryOptions(nextPage = page.value, nextPageSize = pageSize.value) {
  const statusCode = Number(statusCodeFilter.value)
  const search = searchQuery.value.trim()
  return {
    page: nextPage,
    pageSize: nextPageSize,
    search: search || undefined,
    handler: handlerFilter.value !== 'all' ? handlerFilter.value : undefined,
    statusCode: Number.isFinite(statusCode) ? statusCode : undefined,
  }
}

async function loadLogs(nextPage = page.value, nextPageSize = pageSize.value, quiet = false) {
  const requestToken = ++latestLoadToken
  loading.value = true
  try {
    const data = await adminApi.queryLogs(admin.token.value, currentQueryOptions(nextPage, nextPageSize))
    if (requestToken !== latestLoadToken) {
      return
    }
    items.value = data.data
    total.value = data.total
    summary.value = data.summary
    page.value = data.page
    pageSize.value = data.page_size
  } catch (error) {
    if (requestToken === latestLoadToken) {
      if (!quiet) {
        items.value = []
        total.value = 0
        admin.notify(error instanceof Error ? error.message : '加载日志失败', 'danger')
      }
    }
  } finally {
    if (requestToken === latestLoadToken) {
      loading.value = false
    }
  }
}

function stopAutoRefresh() {
  if (refreshTimer !== undefined) {
    window.clearInterval(refreshTimer)
    refreshTimer = undefined
  }
}

function startAutoRefresh() {
  if (!import.meta.client) {
    return
  }
  stopAutoRefresh()
  if (autoRefreshIntervalMs.value <= 0) {
    return
  }
  refreshTimer = window.setInterval(() => {
    if (admin.authReady.value && document.visibilityState === 'visible' && !loading.value) {
      void loadLogs(page.value, pageSize.value, true)
    }
  }, autoRefreshIntervalMs.value)
}

function restoreAutoRefreshInterval() {
  try {
    const raw = window.localStorage.getItem(autoRefreshStorageKey)
    const stored = Number(raw)
    if (raw !== null && autoRefreshOptions.some((option) => option.value === stored)) {
      autoRefreshIntervalMs.value = stored
    }
  } catch {
    // localStorage 不可用时继续使用当前会话的默认值。
  }
}

function persistAutoRefreshInterval(value: number) {
  try {
    window.localStorage.setItem(autoRefreshStorageKey, String(value))
  } catch {
    // localStorage 不可用时仍保留当前会话设置。
  }
}

function cycleAutoRefreshInterval() {
  const currentIndex = autoRefreshOptions.findIndex((option) => option.value === autoRefreshIntervalMs.value)
  const safeIndex = currentIndex >= 0 ? currentIndex : 0
  const next = autoRefreshOptions[(safeIndex + 1) % autoRefreshOptions.length]!
  autoRefreshIntervalMs.value = next.value
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
      void loadLogs(1, pageSize.value)
    }
  },
  { immediate: true },
)

watch(
  () => [searchQuery.value, handlerFilter.value, statusCodeFilter.value],
  () => {
    if (admin.authReady.value) {
      void loadLogs(1, pageSize.value)
    }
  },
)

watch(autoRefreshIntervalMs, (value) => {
  if (!import.meta.client) {
    return
  }
  persistAutoRefreshInterval(value)
  startAutoRefresh()
})

onMounted(() => {
  if (!import.meta.client) {
    return
  }
  restoreAutoRefreshInterval()
  startAutoRefresh()
})

onBeforeUnmount(() => {
  if (searchTimer) {
    clearTimeout(searchTimer)
  }
  stopAutoRefresh()
})
</script>

<template>
  <div class="page-grid">
    <PageHeader
      title="诊断日志"
      icon="mdi-text-box-search-outline"
    >
      <template #meta>
        <AdminBadge tone="secondary" icon="mdi-file-document-outline">
          {{ summary.total }} 条记录
        </AdminBadge>
        <AdminBadge tone="secondary" icon="mdi-list-status">
          {{ summary.status_codes.length }} 种状态码
        </AdminBadge>
      </template>
    </PageHeader>

    <SectionCard
      title="数据筛选"
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
          />
        </div>

        <div class="toolbar-panel">
          <div class="filter-toolbar">
            <VTextField
              v-model="searchInput"
              class="filter-grow"
              label="搜索"
              placeholder="处理器 / 凭据 / 模型 / 错误 / 状态码"
              prepend-inner-icon="mdi-magnify"
              clearable
            />
            <VSelect
              v-model="pageSize"
              class="filter-select"
              label="每页条数"
              :items="pageSizeOptions"
              @update:model-value="(value) => loadLogs(1, Number(value))"
            />
          </div>

          <VChipGroup v-model="statusCodeFilter" mandatory color="primary">
            <VChip
              v-for="status in statusCodeOptions"
              :key="status.value"
              :value="status.value"
              filter
              size="small"
            >
              {{ status.label }}
            </VChip>
          </VChipGroup>

          <VChipGroup v-model="handlerFilter" mandatory color="secondary">
            <VChip value="all" filter size="small">全部服务</VChip>
            <VChip
              v-for="handler in admin.handlers.value"
              :key="handler.key"
              :value="handler.key"
              filter
              size="small"
            >
              {{ handler.label }}
            </VChip>
          </VChipGroup>
        </div>
      </div>
    </SectionCard>

    <SectionCard
      title="日志列表"
      icon="mdi-format-list-bulleted"
    >
      <template #actions>
        <div class="log-refresh-controls">
          <VBtn
            icon="mdi-refresh"
            color="secondary"
            variant="tonal"
            size="small"
            width="36"
            height="36"
            :loading="loading"
            aria-label="刷新日志"
            @click="loadLogs(page, pageSize)"
          />
          <VBtn
            class="log-auto-refresh-trigger text-none"
            :class="{ 'log-auto-refresh-trigger--off': autoRefreshIntervalMs === 0 }"
            color="secondary"
            variant="tonal"
            size="small"
            height="36"
            aria-label="设置自动刷新间隔"
            @click="cycleAutoRefreshInterval"
          >
            <template #prepend>
              <span class="log-auto-refresh-icon" aria-hidden="true">
                <VIcon
                  class="log-auto-refresh-icon__timer"
                  icon="mdi-timer-sand"
                  size="18"
                />
                <VIcon
                  class="log-auto-refresh-icon__off"
                  icon="mdi-timer-off-outline"
                  size="18"
                />
              </span>
            </template>
            {{ autoRefreshLabel }}
          </VBtn>
        </div>
      </template>
      <div class="d-grid ga-5">
        <div class="pagination-bar">
          <div class="text-body-2 text-medium-emphasis">
            共 {{ total }} 条，当前第 {{ page }} / {{ maxPage }} 页
          </div>
          <VPagination
            :model-value="page"
            :length="maxPage"
            density="comfortable"
            total-visible="5"
            @update:model-value="(value) => loadLogs(Number(value), pageSize)"
          />
        </div>

        <div
          v-if="items.length"
          class="log-list"
        >
          <template
            v-for="item in items"
            :key="logItemKey(item)"
          >
            <VExpansionPanels
              variant="accordion"
              class="log-panels"
            >
              <VExpansionPanel
                elevation="0"
                border
              >
                <VExpansionPanelTitle>
                  <div class="activity-title">
                    <div class="activity-topline">
                      <span
                        class="log-status-pill"
                        :class="item.status_code < 400 ? 'log-status-pill--success' : 'log-status-pill--error'"
                      >
                        <span class="log-status-dot" />
                        {{ item.status_code }}
                      </span>
                      <span class="font-weight-medium">{{ admin.handlerLookup.value.get(item.handler)?.label || item.handler }}</span>
                    </div>
                  </div>
                </VExpansionPanelTitle>
                <VExpansionPanelText>
                  <div class="log-detail-stack">
                    <div class="log-meta-panel">
                      <div
                        v-for="meta in logMetaItems(item)"
                        :key="meta.label"
                        class="log-meta-item"
                        :class="{ 'log-meta-item--wide': meta.wide }"
                      >
                        <span>{{ meta.label }}</span>
                        <strong>{{ meta.value }}</strong>
                      </div>
                    </div>
                    <div v-if="hasLogError(item.error)" class="log-detail-surface">
                      <div class="log-detail-heading">
                        <span>错误响应</span>
                        <span>JSON</span>
                      </div>
                      <pre class="log-text">{{ item.error }}</pre>
                    </div>
                  </div>
                </VExpansionPanelText>
              </VExpansionPanel>
            </VExpansionPanels>
          </template>
        </div>

        <EmptyState
          v-else
          title="没有匹配的日志"
          description="可以调整筛选，或等待新的请求进入"
          icon="mdi-text-box-remove-outline"
        />
      </div>
    </SectionCard>
  </div>
</template>
