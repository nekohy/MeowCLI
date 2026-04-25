<script setup lang="ts">
import { adminApi } from '~/composables/useAdminApi'
import { PAGE_SIZE_OPTIONS, formatTime } from '~/lib/admin'
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

const summaryTiles = computed(() => [
  {
    label: '总日志',
    value: summary.value.total,
    helper: '保留期内全部记录',
    icon: 'mdi-file-document-outline',
  },
  {
    label: '状态码',
    value: summary.value.status_codes.length,
    helper: '当前条件下出现的状态码',
    icon: 'mdi-numeric',
  },
  {
    label: '筛选结果',
    value: total.value,
    helper: '应用当前搜索与过滤后',
    icon: 'mdi-filter-check-outline',
  },
])

const maxPage = computed(() => Math.max(1, Math.ceil((total.value || 0) / (pageSize.value || 25))))
const pageSizeOptions = PAGE_SIZE_OPTIONS
const statusCodeOptions = computed(() => [
  { value: 'all', label: '全部状态码' },
  ...summary.value.status_codes.map((item) => ({
    value: String(item.status_code),
    label: `${item.status_code} (${item.total})`,
  })),
])
let searchTimer: ReturnType<typeof setTimeout> | undefined
let latestLoadToken = 0

function previewLog(text: string) {
  const line = text
    .split(/\r?\n/)
    .map((item) => item.trim())
    .find(Boolean)
  return line ? line.slice(0, 140) : '无详细文本'
}

function currentQueryOptions(nextPage = page.value, nextPageSize = pageSize.value) {
  const statusCode = Number(statusCodeFilter.value)
  return {
    page: nextPage,
    pageSize: nextPageSize,
    search: searchQuery.value.trim(),
    handler: handlerFilter.value !== 'all' ? handlerFilter.value : undefined,
    statusCode: Number.isFinite(statusCode) ? statusCode : undefined,
  }
}

async function loadLogs(nextPage = page.value, nextPageSize = pageSize.value) {
  const requestToken = ++latestLoadToken
  loading.value = true
  try {
    const data = await adminApi.listLogs(admin.token.value, currentQueryOptions(nextPage, nextPageSize))
    if (requestToken !== latestLoadToken) {
      return
    }
    items.value = data.data || []
    total.value = data.total || 0
    summary.value = {
      total: data.summary?.total || 0,
      status_codes: data.summary?.status_codes || [],
    }
    page.value = data.page || nextPage
    pageSize.value = data.page_size || nextPageSize
  } catch (error) {
    if (requestToken === latestLoadToken) {
      items.value = []
      total.value = 0
      admin.notify(error instanceof Error ? error.message : '加载日志失败', 'danger')
    }
  } finally {
    if (requestToken === latestLoadToken) {
      loading.value = false
    }
  }
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

onBeforeUnmount(() => {
  if (searchTimer) {
    clearTimeout(searchTimer)
  }
})
</script>

<template>
  <div class="page-grid">
    <PageHeader
      title="诊断日志"
      icon="mdi-text-box-search-outline"
    >
      <template #meta>
        <AdminBadge tone="secondary" icon="mdi-counter">
          {{ summary.total }} 条记录
        </AdminBadge>
        <AdminBadge tone="secondary" icon="mdi-numeric">
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
              placeholder="处理器 / 凭据 / 状态码"
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

        <VExpansionPanels
          v-if="items.length"
          variant="accordion"
          class="log-panels"
        >
          <VExpansionPanel
            v-for="item in items"
            :key="`${item.handler}-${item.credential_id}-${item.created_at}-${item.status_code}`"
            elevation="0"
            border
          >
            <VExpansionPanelTitle>
              <div class="activity-title">
                <div class="activity-topline">
                  <AdminBadge :tone="item.status_code < 400 ? 'success' : 'danger'">
                    {{ item.status_code }}
                  </AdminBadge>
                  <span class="font-weight-medium">{{ admin.handlerLookup.value.get(item.handler)?.label || item.handler }}</span>
                  <span class="text-medium-emphasis">{{ item.credential_id || '未记录凭据' }}</span>
                  <span class="text-medium-emphasis">{{ formatTime(item.created_at) }}</span>
                </div>
                <div class="activity-preview">{{ previewLog(item.text) }}</div>
              </div>
            </VExpansionPanelTitle>
            <VExpansionPanelText>
              <pre class="log-text">{{ item.text }}</pre>
            </VExpansionPanelText>
          </VExpansionPanel>
        </VExpansionPanels>

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
