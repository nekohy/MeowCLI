<script setup lang="ts">
import { adminApi } from '~/composables/useAdminApi'
import { joinPlanTypeInput, planTypeText, safeStringify, splitPlanTypeInput, statusText } from '~/lib/admin'
import type { ModelItem } from '~/types/admin'

function hasExtra(extra: unknown): boolean {
  if (!extra) return false
  if (typeof extra === 'object' && Object.keys(extra as object).length === 0) return false
  return true
}

definePageMeta({
  navKey: 'models',
})

const admin = useAdminApp()
const confirm = useConfirmDialog()

const items = ref<ModelItem[]>([])
const loading = ref(false)
const search = ref('')
const handlerFilter = ref('all')
const actionBusy = ref(false)

const modalOpen = ref(false)
const modalMode = ref<'create' | 'edit'>('create')
const modalAlias = ref('')
const modalOrigin = ref('')
const modalHandler = ref('gemini')
const modalPlanTypes = ref('')
const modalExtra = ref('{}')
const modalError = ref('')
const planOrderOpen = ref(false)
const planOrderDraft = ref<string[]>([])
const dragIdx = ref<number | null>(null)

const modalHandlerConfig = computed(() => (
  admin.handlers.value.find((handler) => handler.key === modalHandler.value) || null
))
const modalAvailablePlanTypes = computed(() => modalHandlerConfig.value?.plan_list || [])
const modalSelectedPlanTypes = computed(() => splitPlanTypeInput(modalPlanTypes.value))

function defaultPlanTypesForHandler(handlerKey: string) {
  return handlerKey === 'gemini' ? 'ultra,pro,free' : ''
}

const hintNoPlanTypes = computed(() => {
  if (modalHandler.value === 'gemini') return '未限制，默认使用 ultra → pro → free'
  return '未限制，调度器将使用所有可用套餐'
})

function syncPlanOrderDraft() {
  const selected = modalSelectedPlanTypes.value
  const remaining = modalAvailablePlanTypes.value.filter((planType) => !selected.includes(planType))
  planOrderDraft.value = [...selected, ...remaining]
}

const filteredItems = computed(() => {
  const query = search.value.trim().toLowerCase()
  return items.value.filter((item) => {
    if (handlerFilter.value !== 'all' && item.handler !== handlerFilter.value) {
      return false
    }
    if (!query) {
      return true
    }
    return [item.alias, item.origin, item.handler, item.plan_types, safeStringify(item.extra)]
      .some((value) => String(value || '').toLowerCase().includes(query))
  })
})

function modelsForHandler(handlerKey: string) {
  return items.value.filter((item) => item.handler === handlerKey).length
}

async function loadModels() {
  loading.value = true
  try {
    items.value = await adminApi.listModels(admin.token.value)
  } catch (error) {
    admin.notify(error instanceof Error ? error.message : '加载模型映射失败', 'danger')
  } finally {
    loading.value = false
  }
}

function openCreateModal() {
  modalMode.value = 'create'
  modalAlias.value = ''
  modalOrigin.value = ''
  modalHandler.value = admin.activeHandler.value?.key || admin.handlers.value[0]?.key || 'codex'
  modalPlanTypes.value = defaultPlanTypesForHandler(modalHandler.value)
  modalExtra.value = '{}'
  modalError.value = ''
  syncPlanOrderDraft()
  modalOpen.value = true
}

function openEditModal(item: ModelItem) {
  modalMode.value = 'edit'
  modalAlias.value = item.alias
  modalOrigin.value = item.origin
  modalHandler.value = item.handler
  modalPlanTypes.value = item.plan_types || defaultPlanTypesForHandler(item.handler)
  modalExtra.value = safeStringify(item.extra)
  modalError.value = ''
  syncPlanOrderDraft()
  modalOpen.value = true
}

function closeModal() {
  modalOpen.value = false
  modalError.value = ''
  planOrderOpen.value = false
  dragIdx.value = null
}

function openPlanOrderModal() {
  syncPlanOrderDraft()
  planOrderOpen.value = true
}

function planOrderDraftSelected(planType: string) {
  return modalSelectedPlanTypes.value.includes(planType)
}

function togglePlanType(planType: string) {
  const selected = [...modalSelectedPlanTypes.value]
  const idx = selected.indexOf(planType)
  if (idx >= 0) {
    selected.splice(idx, 1)
  } else {
    selected.push(planType)
  }
  modalPlanTypes.value = joinPlanTypeInput(selected)
  syncPlanOrderDraft()
}

function onDragStart(idx: number) {
  dragIdx.value = idx
}

function onDragOver(event: DragEvent, idx: number) {
  event.preventDefault()
  if (dragIdx.value === null || dragIdx.value === idx) {
    return
  }
  const next = [...planOrderDraft.value]
  const moved = next.splice(dragIdx.value, 1)[0]
  if (!moved) {
    return
  }
  next.splice(idx, 0, moved)
  planOrderDraft.value = next
  dragIdx.value = idx
}

function onDragEnd() {
  dragIdx.value = null
  const selected = new Set(modalSelectedPlanTypes.value)
  modalPlanTypes.value = joinPlanTypeInput(planOrderDraft.value.filter((planType) => selected.has(planType)))
}

async function saveModel() {
  actionBusy.value = true
  modalError.value = ''

  try {
    let extra: Record<string, unknown> = {}
    try {
      extra = JSON.parse(modalExtra.value || '{}') as Record<string, unknown>
    } catch {
      throw new Error('附加参数必须是合法的 JSON')
    }

    const payload = {
      origin: modalOrigin.value.trim(),
      handler: modalHandler.value,
      plan_types: joinPlanTypeInput(splitPlanTypeInput(modalPlanTypes.value)),
      extra,
    }

    if (modalMode.value === 'edit') {
      await adminApi.updateModel(admin.token.value, modalAlias.value, payload)
    } else {
      await adminApi.createModel(admin.token.value, {
        alias: modalAlias.value.trim(),
        ...payload,
      })
    }

    closeModal()
    admin.notify(modalMode.value === 'edit' ? '模型映射已更新' : '模型映射已创建')
    await Promise.all([
      admin.loadOverview(admin.token.value, true),
      loadModels(),
    ])
  } catch (error) {
    modalError.value = error instanceof Error ? error.message : '保存模型映射失败'
  } finally {
    actionBusy.value = false
  }
}

function openDeleteConfirm(item: ModelItem) {
  confirm.show({
    title: '删除模型映射',
    message: `确认删除模型映射"${item.alias}"吗？`,
    confirmText: '确认删除',
    action: async () => {
      actionBusy.value = true
      try {
        await adminApi.deleteModel(admin.token.value, item.alias)
        admin.notify('模型映射已删除')
        await Promise.all([
          admin.loadOverview(admin.token.value, true),
          loadModels(),
        ])
      } catch (error) {
        admin.notify(error instanceof Error ? error.message : '删除模型映射失败', 'danger')
      } finally {
        actionBusy.value = false
      }
    },
  })
}

onMounted(() => {
  if (admin.authReady.value) {
    void loadModels()
  }
})

watch(
  () => admin.authReady.value,
  (ready) => {
    if (ready) {
      void loadModels()
    }
  },
)

watch(
  () => modalHandler.value,
  (handler, previous) => {
    if (handler === previous) {
      return
    }
    if (modalMode.value === 'create' || modalPlanTypes.value === defaultPlanTypesForHandler(previous || '')) {
      modalPlanTypes.value = defaultPlanTypesForHandler(handler)
    } else {
      modalPlanTypes.value = joinPlanTypeInput(
        modalSelectedPlanTypes.value.filter((planType) => modalAvailablePlanTypes.value.includes(planType)),
      )
    }
    syncPlanOrderDraft()
  },
)
</script>

<template>
  <div class="page-grid">
    <PageHeader
      eyebrow="模型"
      title="模型映射"
      icon="mdi-compare-horizontal"
    >
      <template #meta>
        <AdminBadge tone="secondary" icon="mdi-shape-outline">
          {{ items.length }} 映射
        </AdminBadge>
      </template>
      <template #actions>
        <AdminButton prepend-icon="mdi-plus" @click="openCreateModal">新建映射</AdminButton>
      </template>
    </PageHeader>

    <SectionCard
      title="映射列表"
      :eyebrow="`${filteredItems.length} 个结果`"
      icon="mdi-format-list-bulleted-square"
    >
      <div class="toolbar-panel mb-4">
        <VTextField
          v-model="search"
          label="搜索"
          placeholder="别名 / 上游模型"
          prepend-inner-icon="mdi-magnify"
          clearable
        />

        <div class="d-flex flex-wrap ga-2 align-center">
          <VChipGroup v-model="handlerFilter" mandatory color="primary">
            <VChip value="all" filter size="small">全部</VChip>
            <VChip
              v-for="handler in admin.handlers.value"
              :key="handler.key"
              :value="handler.key"
              filter
              size="small"
            >
              {{ handler.label }} ({{ modelsForHandler(handler.key) }})
            </VChip>
          </VChipGroup>
        </div>
      </div>

      <div v-if="filteredItems.length" class="model-grid">
        <VCard
          v-for="item in filteredItems"
          :key="item.alias"
          class="interactive-card"
          color="surface-container"
          variant="flat"
        >
          <VCardText class="pa-5 d-grid ga-4">
            <div class="d-flex justify-space-between align-start">
              <div style="min-width: 0">
                <div class="text-h6 font-weight-bold mb-1">{{ item.alias }}</div>
                <div class="d-flex align-center ga-2 text-caption text-medium-emphasis">
                  <span class="text-truncate" style="max-width: 280px">{{ item.origin }}</span>
                  <VIcon icon="mdi-chevron-right" size="16" />
                </div>
              </div>
              <AdminBadge tone="secondary" subtle :icon="item.handler === 'gemini' ? 'mdi-google-circles-communities' : 'mdi-console'">
                {{ admin.handlerLookup.value.get(item.handler)?.label || item.handler }}
              </AdminBadge>
            </div>

            <VSheet v-if="hasExtra(item.extra)" color="surface-container-high" rounded="lg" class="pa-3">
              <div class="text-caption text-medium-emphasis mb-2 d-flex align-center ga-1">
                <VIcon icon="mdi-code-json" size="14" />
                <span>JSON</span>
              </div>
              <code class="text-caption d-block" style="white-space: pre-wrap; overflow: auto; max-height: 80px">{{ safeStringify(item.extra) }}</code>
            </VSheet>

            <VSheet v-if="item.plan_types" color="surface-container-high" rounded="lg" class="pa-3">
              <div class="text-caption text-medium-emphasis mb-2 d-flex align-center ga-1">
                <VIcon icon="mdi-star-circle-outline" size="14" />
                <span>套餐</span>
              </div>
              <div class="d-flex flex-wrap ga-2">
                <AdminBadge
                  v-for="(pt, idx) in splitPlanTypeInput(item.plan_types)"
                  :key="pt"
                  tone="secondary"
                  subtle
                  icon="mdi-sort"
                >
                  {{ idx + 1 }}. {{ planTypeText(pt) }}
                </AdminBadge>
              </div>
            </VSheet>

            <div class="d-flex ga-2">
              <AdminButton
                variant="secondary"
                size="sm"
                prepend-icon="mdi-pencil-outline"
                @click="openEditModal(item)"
              >
                编辑
              </AdminButton>
              <AdminButton
                variant="danger"
                size="sm"
                prepend-icon="mdi-delete-outline"
                @click="openDeleteConfirm(item)"
              >
                删除
              </AdminButton>
            </div>
          </VCardText>
        </VCard>
      </div>

      <EmptyState
        v-else
        title="无匹配映射"
        description="调整筛选或新建映射。"
        icon="mdi-link-off"
      />
    </SectionCard>

    <ModalDialog
      :open="modalOpen"
      :title="modalMode === 'edit' ? '编辑模型映射' : '新建模型映射'"
      :icon="modalMode === 'edit' ? 'mdi-pencil-outline' : 'mdi-plus'"
      max-width="640"
      @close="closeModal"
    >
      <div class="d-grid ga-7 pt-2">
        <VTextField
          v-model="modalAlias"
          label="别名"
          placeholder="gpt-4-meow"
          prepend-inner-icon="mdi-tag-outline"
          :disabled="modalMode === 'edit'"
        />
        <VTextField
          v-model="modalOrigin"
          label="上游模型"
          placeholder="gpt-4-0125-preview"
          prepend-inner-icon="mdi-cloud-outline"
        />
        <VSelect
          v-model="modalHandler"
          label="目标处理器"
          prepend-inner-icon="mdi-cpu-64-bit"
          :items="admin.handlers.value.map((handler) => ({
            title: `${handler.label} (${statusText(handler.status)})`,
            value: handler.key,
          }))"
        />
        <VSheet
          color="surface-container-high"
          rounded="lg"
          class="pa-4 d-grid ga-3"
        >
          <div class="d-flex justify-space-between align-center ga-3 flex-wrap">
            <div>
              <div class="text-subtitle-2 font-weight-bold">套餐类型</div>
              <div class="text-caption text-medium-emphasis">
                多选并排序，调度会按从左到右的顺序优先使用。
              </div>
            </div>
            <AdminButton variant="secondary" size="sm" prepend-icon="mdi-swap-vertical" @click="openPlanOrderModal">
              排序
            </AdminButton>
          </div>

          <div class="d-flex flex-wrap ga-2">
            <VChip
              v-for="planType in modalAvailablePlanTypes"
              :key="planType"
              :color="planOrderDraftSelected(planType) ? 'primary' : undefined"
              :variant="planOrderDraftSelected(planType) ? 'flat' : 'outlined'"
              filter
              @click="togglePlanType(planType)"
            >
              {{ planTypeText(planType) }}
            </VChip>
          </div>

          <div class="d-flex flex-wrap ga-2">
            <AdminBadge
              v-for="(planType, idx) in modalSelectedPlanTypes"
              :key="`${planType}-${idx}`"
              tone="secondary"
              subtle
              icon="mdi-sort"
            >
              {{ idx + 1 }}. {{ planTypeText(planType) }}
            </AdminBadge>
            <span v-if="!modalSelectedPlanTypes.length" class="text-caption text-medium-emphasis">
              {{ hintNoPlanTypes }}
            </span>
          </div>
        </VSheet>
        <VTextarea
          v-model="modalExtra"
          rows="4"
          label="附加参数"
          placeholder="{}"
          prepend-inner-icon="mdi-code-json"
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
          prepend-icon="mdi-content-save-check-outline"
          :loading="actionBusy"
          @click="saveModel"
        >
          {{ modalMode === 'edit' ? '更新映射' : '创建映射' }}
        </AdminButton>
      </template>
    </ModalDialog>

    <ModalDialog
      :open="planOrderOpen"
      title="套餐类型排序"
      description="拖动调整优先级，勾选决定是否启用。"
      icon="mdi-sort"
      :max-width="460"
      @close="planOrderOpen = false"
    >
      <div class="plan-order-list">
        <div
          v-for="(planType, idx) in planOrderDraft"
          :key="planType"
          class="plan-order-item"
          :class="{
            'plan-order-item--selected': planOrderDraftSelected(planType),
            'plan-order-item--dragging': dragIdx === idx,
          }"
          draggable="true"
          @dragstart="onDragStart(idx)"
          @dragover="(event) => onDragOver(event, idx)"
          @dragend="onDragEnd"
        >
          <VIcon icon="mdi-drag" size="18" class="plan-order-drag text-medium-emphasis" />
          <VCheckbox
            :model-value="planOrderDraftSelected(planType)"
            density="compact"
            hide-details
            @update:model-value="togglePlanType(planType)"
            @click.stop
          />
          <span class="plan-order-label">{{ planTypeText(planType) }}</span>
          <span v-if="planOrderDraftSelected(planType)" class="plan-order-rank text-medium-emphasis">
            #{{ modalSelectedPlanTypes.indexOf(planType) + 1 }}
          </span>
        </div>
      </div>
      <template #footer>
        <AdminButton variant="ghost" @click="planOrderOpen = false">关闭</AdminButton>
      </template>
    </ModalDialog>

    <ModalDialog
      :open="confirm.open.value"
      :title="confirm.title.value"
      icon="mdi-delete-outline"
      @close="confirm.close()"
    >
      <p class="text-body-1">{{ confirm.message.value }}</p>
      <template #footer>
        <AdminButton variant="ghost" :disabled="actionBusy" @click="confirm.close()">取消</AdminButton>
        <AdminButton variant="danger" :loading="actionBusy" @click="confirm.submit()">确认删除</AdminButton>
      </template>
    </ModalDialog>
  </div>
</template>

<style scoped>
.plan-order-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.plan-order-item {
  display: flex;
  align-items: center;
  gap: 10px;
  min-height: 52px;
  padding: 0 12px;
  border-radius: 18px;
  border: 1px solid rgba(var(--v-theme-outline), 0.24);
  background: rgba(var(--v-theme-surface-container-high), 0.76);
}

.plan-order-item--selected {
  border-color: rgba(var(--v-theme-primary), 0.34);
}

.plan-order-item--dragging {
  opacity: 0.72;
}

.plan-order-label {
  flex: 1;
  min-width: 0;
  font-weight: 600;
}

.plan-order-rank {
  font-size: 0.85rem;
}
</style>
