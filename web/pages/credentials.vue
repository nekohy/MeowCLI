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
import type { CodexItem } from '~/types/admin'

definePageMeta({
  navKey: 'credentials',
})

const admin = useAdminApp()
const confirm = useConfirmDialog()

const rows = ref<CodexItem[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(25)
const loading = ref(false)
const search = ref('')
const statusFilter = ref<'all' | 'enabled' | 'disabled' | 'unsynced'>('all')
const planFilter = ref('all')
const selectedIds = ref<string[]>([])
const actionBusy = ref(false)

const importOpen = ref(false)
const importTokens = ref('')
const importError = ref('')

const importLines = computed(() => (
  importTokens.value
    .split('\n')
    .map((line) => line.trim())
    .filter(Boolean)
))

const filteredRows = computed(() => {
  const query = search.value.trim().toLowerCase()
  return rows.value.filter((item) => {
    if (statusFilter.value === 'enabled' && item.status !== 'enabled') {
      return false
    }
    if (statusFilter.value === 'disabled' && item.status !== 'disabled') {
      return false
    }
    if (statusFilter.value === 'unsynced' && !isUnsynced(item.synced_at)) {
      return false
    }

    const planType = normalizePlanType(item.plan_type)
    if (planFilter.value !== 'all' && planType !== planFilter.value) {
      return false
    }

    if (!query) {
      return true
    }

    return [item.id, item.status, planType, item.access_token, item.refresh_token]
      .some((value) => String(value || '').toLowerCase().includes(query))
  })
})

const summaryTiles = computed(() => [
  {
    label: '当前页',
    value: rows.value.length,
    helper: '本次已加载的凭据数',
    icon: 'mdi-file-document-outline',
  },
  {
    label: '可用',
    value: rows.value.filter((item) => item.status === 'enabled').length,
    helper: '启用且可参与调度',
    icon: 'mdi-check-circle-outline',
  },
  {
    label: '未同步',
    value: rows.value.filter((item) => isUnsynced(item.synced_at)).length,
    helper: '需要重新同步额度',
    icon: 'mdi-sync-alert',
  },
  {
    label: '已选择',
    value: selectedIds.value.length,
    icon: 'mdi-checkbox-multiple-marked-outline',
  },
])

const availablePlanTypes = computed(() => {
  const planTypes = new Set<string>()
  rows.value.forEach((item) => {
    const planType = normalizePlanType(item.plan_type)
    if (planType) {
      planTypes.add(planType)
    }
  })
  return ['all', ...planTypes]
})

const selectedSet = computed(() => new Set(selectedIds.value))
const allVisibleSelected = computed(() => (
  filteredRows.value.length > 0 && filteredRows.value.every((item) => selectedSet.value.has(item.id))
))
const maxPage = computed(() => Math.max(1, Math.ceil((total.value || 0) / (pageSize.value || 25))))
const pageSizeOptions = PAGE_SIZE_OPTIONS

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
  selectedIds.value = filteredRows.value.map((item) => item.id)
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

async function loadCredentials(nextPage = page.value, nextPageSize = pageSize.value) {
  if (!admin.token.value || !admin.activeHandler.value?.supports_credentials) {
    rows.value = []
    total.value = 0
    page.value = 1
    selectedIds.value = []
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
  importError.value = ''

  try {
    if (importLines.value.length === 0) {
      importError.value = '请至少输入一行令牌'
      return
    }

    const result = await adminApi.batchCreateCodex(admin.token.value, { tokens: importLines.value })
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

  confirm.show({
    title: `${statusText(status)}凭据`,
    message: `确认将 ${ids.length} 个凭据设为"${statusText(status)}"吗？`,
    confirmText: `确认${statusText(status)}`,
    confirmVariant: 'secondary',
    action: async () => {
      actionBusy.value = true
      try {
        const result = await adminApi.batchUpdateCodexStatus(admin.token.value, { ids, status })
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
        const result = await adminApi.batchDeleteCodex(admin.token.value, { ids })
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

watch(
  () => admin.selectedHandler.value,
  () => {
    statusFilter.value = 'all'
    planFilter.value = 'all'
    search.value = ''
    if (admin.authReady.value) {
      void loadCredentials(1, pageSize.value)
    }
  },
)
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
          导入凭据
        </AdminButton>
      </template>
    </PageHeader>

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
              :color="tile.color"
            />
          </div>

          <div class="toolbar-panel">
            <div class="filter-toolbar">
              <VTextField
                v-model="search"
                class="filter-grow"
                label="搜索"
                placeholder="ID / 状态 / 套餐 / Token"
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
              <VChip value="unsynced" filter>未同步</VChip>
            </VChipGroup>

            <VChipGroup v-model="planFilter" mandatory color="secondary">
              <VChip value="all" filter>全部套餐</VChip>
              <VChip
                v-for="plan in availablePlanTypes.filter((item) => item !== 'all')"
                :key="plan"
                :value="plan"
                filter
              >
                {{ plan }}
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

          <div v-if="filteredRows.length" class="d-grid ga-4">
            <div class="d-flex align-center justify-space-between flex-wrap ga-3">
              <VCheckboxBtn
                :model-value="allVisibleSelected"
                label="选中所有筛选结果"
                @update:model-value="toggleSelectAll"
              />
              <div class="text-body-2 text-medium-emphasis">
                共 {{ total }} 条，当前第 {{ page }} / {{ maxPage }} 页
              </div>
            </div>

            <div class="stack-list">
              <VCard
                v-for="item in filteredRows"
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
                </VCardText>
              </VCard>
            </div>
          </div>

          <EmptyState
            v-else
            title="当前筛选没有结果"
            description="换一个处理器、清空筛选，或者先导入新凭据。"
            icon="mdi-key-plus"
          >
            <template #action>
              <AdminButton prepend-icon="mdi-import" @click="importOpen = true">导入凭据</AdminButton>
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
      :title="admin.activeHandler.value ? `导入 ${admin.activeHandler.value.label} 凭据` : '导入凭据'"
      description="一行一个令牌，支持 Refresh Token 或 Access Token。"
      max-width="720"
      @close="closeImportModal"
    >
      <div class="d-grid ga-4">
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
