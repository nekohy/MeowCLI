<script setup lang="ts">
import { adminApi } from '~/composables/useAdminApi'
import { PAGE_SIZE_OPTIONS, formatTime } from '~/lib/admin'
import type { LogItem } from '~/types/admin'

definePageMeta({
  navKey: 'logs',
})

const admin = useAdminApp()

const items = ref<LogItem[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(25)
const loading = ref(false)
const search = ref('')
const handlerFilter = ref('all')
const severityFilter = ref<'all' | 'errors' | 'success'>('all')

const filteredItems = computed(() => {
  const query = search.value.trim().toLowerCase()
  return items.value.filter((item) => {
    if (handlerFilter.value !== 'all' && item.handler !== handlerFilter.value) {
      return false
    }
    if (severityFilter.value === 'errors' && item.status_code < 400) {
      return false
    }
    if (severityFilter.value === 'success' && item.status_code >= 400) {
      return false
    }
    if (!query) {
      return true
    }
    return [item.handler, item.credential_id, item.text, item.status_code]
      .some((value) => String(value || '').toLowerCase().includes(query))
  })
})

const summaryTiles = computed(() => [
  {
    label: '本页日志',
    value: items.value.length,
    helper: '当前页拉取到的记录数',
    icon: 'mdi-file-document-outline',
  },
  {
    label: '错误请求',
    value: items.value.filter((item) => item.status_code >= 400).length,
    helper: 'HTTP 400 及以上',
    icon: 'mdi-alert-circle-outline',
  },
  {
    label: '筛选结果',
    value: filteredItems.value.length,
    helper: '应用当前搜索与过滤后',
    icon: 'mdi-filter-check-outline',
  },
])

const maxPage = computed(() => Math.max(1, Math.ceil((total.value || 0) / (pageSize.value || 25))))
const pageSizeOptions = PAGE_SIZE_OPTIONS

function previewLog(text: string) {
  const line = text
    .split(/\r?\n/)
    .map((item) => item.trim())
    .find(Boolean)
  return line ? line.slice(0, 140) : '无详细文本'
}

async function loadLogs(nextPage = page.value, nextPageSize = pageSize.value) {
  loading.value = true
  try {
    const data = await adminApi.listLogs(admin.token.value, { page: nextPage, pageSize: nextPageSize })
    items.value = data.data || []
    total.value = data.total || 0
    page.value = data.page || nextPage
    pageSize.value = data.page_size || nextPageSize
  } catch (error) {
    admin.notify(error instanceof Error ? error.message : '加载日志失败', 'danger')
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  if (admin.authReady.value) {
    void loadLogs()
  }
})

watch(
  () => admin.authReady.value,
  (ready) => {
    if (ready) {
      void loadLogs(1, pageSize.value)
    }
  },
)
</script>

<template>
  <div class="page-grid">
    <PageHeader
      title="诊断日志"
      icon="mdi-text-box-search-outline"
    >
      <template #meta>
        <AdminBadge tone="secondary" icon="mdi-counter">
          {{ total }} 条记录
        </AdminBadge>
        <AdminBadge tone="danger" icon="mdi-alert-circle-outline">
          {{ items.filter((item) => item.status_code >= 400).length }} 错误
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
              v-model="search"
              class="filter-grow"
              label="搜索"
              placeholder="处理器 / 凭据 / 状态码"
              prepend-inner-icon="mdi-magnify"
              clearable
            />
          </div>

          <VChipGroup v-model="severityFilter" mandatory color="primary">
            <VChip value="all" filter size="small">全部结果</VChip>
            <VChip value="errors" filter size="small">错误请求</VChip>
            <VChip value="success" filter size="small">成功请求</VChip>
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
          v-if="filteredItems.length"
          variant="accordion"
          class="log-panels"
        >
          <VExpansionPanel
            v-for="item in filteredItems"
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
          title="这一页没有匹配的日志"
          description="可以切换页码、调整筛选，或等待新的请求进入"
          icon="mdi-text-box-remove-outline"
        />
      </div>
    </SectionCard>
  </div>
</template>
