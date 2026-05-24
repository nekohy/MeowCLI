<script setup lang="ts">
import { adminApi } from '~/composables/useAdminApi'
import { joinPlanTypeInput, planTypeText, safeStringify, splitPlanTypeInput } from '~/lib/admin'
import type { ModelItem, PluginInfo } from '~/types/admin'

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
const modalPlugins = ref<string[]>([])
const modalExtra = ref('{}')
const modalError = ref('')
const handlerIconByKey: Record<string, string> = {
  codex: 'mdi-console',
  gemini: 'mdi-google-circles-communities',
  antigravity: 'mdi-compass-outline',
}

const modalHandlerConfig = computed(() => (
  admin.handlers.value.find((handler) => handler.key === modalHandler.value) || null
))
const modalAvailablePlanTypes = computed(() => modalHandlerConfig.value?.plan_list || [])
const modalAvailablePlugins = computed(() => modalHandlerConfig.value?.plugins || [])
const modalSelectedPlanTypes = computed(() => splitPlanTypeInput(modalPlanTypes.value, modalAvailablePlanTypes.value))

function defaultPlanTypesForHandler(_handlerKey: string) {
  return ''
}

function planTypesForHandler(handlerKey: string) {
  return admin.handlers.value.find((handler) => handler.key === handlerKey)?.plan_list || []
}

function modelPlanTypes(item: ModelItem) {
  return splitPlanTypeInput(item.plan_types, planTypesForHandler(item.handler))
}

function modelPlanSummary(item: ModelItem) {
  return modelPlanTypes(item).map((planType) => planTypeText(planType)).join(' -> ')
}

function splitPluginInput(value: string) {
  return String(value || '')
    .split(',')
    .map((item) => item.trim())
    .filter(Boolean)
    .filter((item, index, list) => list.indexOf(item) === index)
}

function pluginsForHandler(handlerKey: string) {
  return admin.handlers.value.find((handler) => handler.key === handlerKey)?.plugins || []
}

function modelPlugins(item: ModelItem) {
  const available = new Set(pluginsForHandler(item.handler).map((plugin) => plugin.name))
  return splitPluginInput(item.plugin).filter((name) => available.has(name))
}

function pluginLabel(name: string, plugins: PluginInfo[]) {
  return plugins.find((plugin) => plugin.name === name)?.label || name
}

const modelPlanOrder = usePlanOrderModal(
  () => modalPlanTypes.value,
  (value) => { modalPlanTypes.value = value },
  () => modalAvailablePlanTypes.value,
)

const modelPluginOrder = useOrderedSelectionModal(
  () => modalPlugins.value,
  (value) => { modalPlugins.value = value },
  () => modalAvailablePlugins.value.map((plugin) => plugin.name),
)

const modalPlanSummary = computed(() => (
  modelPlanOrder.preview.value.length
    ? `已选择 ${modelPlanOrder.preview.value.length} 个套餐`
    : '未指定套餐顺序'
))

const modalPluginSummary = computed(() => (
  modelPluginOrder.preview.value.length
    ? `已启用 ${modelPluginOrder.preview.value.length} 个插件`
    : '未启用插件'
))

function formatExtra(extra: unknown): string {
  try {
    return JSON.stringify(extra, null, 2)
  } catch {
    return safeStringify(extra)
  }
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
    return [item.alias, item.origin, item.handler, item.plan_types, item.plugin, safeStringify(item.extra)]
      .some((value) => String(value || '').toLowerCase().includes(query))
  })
})

function modelsForHandler(handlerKey: string) {
  return items.value.filter((item) => item.handler === handlerKey).length
}

function handlerIcon(handlerKey: string) {
  return handlerIconByKey[handlerKey] || 'mdi-cpu-64-bit'
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
  modalPlugins.value = []
  modalExtra.value = '{}'
  modalError.value = ''
  modalOpen.value = true
}

function openEditModal(item: ModelItem) {
  modalMode.value = 'edit'
  modalAlias.value = item.alias
  modalOrigin.value = item.origin
  modalHandler.value = item.handler
  modalPlanTypes.value = joinPlanTypeInput(modelPlanTypes(item), planTypesForHandler(item.handler)) || defaultPlanTypesForHandler(item.handler)
  modalPlugins.value = modelPlugins(item)
  modalExtra.value = safeStringify(item.extra)
  modalError.value = ''
  modalOpen.value = true
}

function closeModal() {
  modalOpen.value = false
  modalError.value = ''
  modelPlanOrder.closeModal()
  modelPluginOrder.closeModal()
}

function modalPluginLabel(name: string) {
  return pluginLabel(name, modalAvailablePlugins.value)
}

function modalPluginDescription(name: string) {
  return modalAvailablePlugins.value.find((plugin) => plugin.name === name)?.description || ''
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
      plan_types: joinPlanTypeInput(splitPlanTypeInput(modalPlanTypes.value, modalAvailablePlanTypes.value), modalAvailablePlanTypes.value),
      plugin: modalPlugins.value
        .filter((name) => modalAvailablePlugins.value.some((plugin) => plugin.name === name))
        .join(','),
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
        modalAvailablePlanTypes.value,
      )
    }
    const availablePlugins = new Set(modalAvailablePlugins.value.map((plugin) => plugin.name))
    modalPlugins.value = modalPlugins.value.filter((name) => availablePlugins.has(name))
  },
)
</script>

<template>
  <div class="page-grid">
    <PageHeader
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
          class="interactive-card model-card"
          color="surface-container"
          variant="flat"
        >
          <VCardText class="pa-5 d-flex flex-column ga-3 model-card-body">
            <div class="d-flex justify-space-between align-center">
              <div style="min-width: 0">
                <div class="text-h6 font-weight-bold">{{ item.alias }}</div>
                <div class="text-caption text-medium-emphasis text-truncate" style="max-width: 280px">{{ item.origin }}</div>
              </div>
              <AdminBadge tone="secondary" subtle :icon="handlerIcon(item.handler)">
                {{ admin.handlerLookup.value.get(item.handler)?.label || item.handler }}
              </AdminBadge>
            </div>

            <div v-if="modelPlanTypes(item).length" class="d-flex flex-wrap ga-2 align-center">
              <AdminBadge
                tone="secondary"
                subtle
                icon="mdi-swap-vertical"
                class="model-plan-badge"
              >
                <span class="model-plan-summary-inline">
                  <span
                    v-for="(planType, idx) in modelPlanTypes(item)"
                    :key="planType"
                    class="model-plan-summary-token"
                  >
                    {{ planTypeText(planType) }}<span v-if="idx < modelPlanTypes(item).length - 1" class="model-plan-arrow"> -&gt; </span>
                  </span>
                </span>
              </AdminBadge>
            </div>

            <div v-if="modelPlugins(item).length" class="d-flex flex-wrap ga-2 align-center">
              <AdminBadge
                v-for="pluginName in modelPlugins(item)"
                :key="pluginName"
                tone="accent"
                subtle
                icon="mdi-puzzle-outline"
                class="model-plugin-badge"
              >
                {{ pluginLabel(pluginName, pluginsForHandler(item.handler)) }}
              </AdminBadge>
            </div>

            <details v-if="hasExtra(item.extra)" class="extra-json-panel">
              <summary>附加参数 JSON</summary>
              <pre>{{ formatExtra(item.extra) }}</pre>
            </details>

            <div class="d-flex ga-2 justify-end model-card-actions">
              <AdminButton
                variant="secondary"
                size="sm"
                prepend-icon="mdi-pencil-outline"
                class="model-action-button"
                @click="openEditModal(item)"
              >
                编辑
              </AdminButton>
              <AdminButton
                variant="danger"
                size="sm"
                prepend-icon="mdi-delete-outline"
                class="model-action-button"
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
        description="调整筛选或新建映射"
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
      <div class="model-form-stack">
        <VTextField
          v-model="modalAlias"
          label="别名"
          placeholder="gpt-4-meow"
          prepend-inner-icon="mdi-tag-outline"
          persistent-placeholder
          :disabled="modalMode === 'edit'"
        />
        <VTextField
          v-model="modalOrigin"
          label="上游模型"
          placeholder="gpt-4-0125-preview"
          prepend-inner-icon="mdi-cloud-outline"
          persistent-placeholder
        />
        <VSelect
          v-model="modalHandler"
          label="目标处理器"
          prepend-inner-icon="mdi-cpu-64-bit"
          :items="admin.handlers.value.map((handler) => ({
            title: handler.label,
            value: handler.key,
          }))"
        />
        <VTextField
          class="model-order-field"
          :model-value="modalPlanSummary"
          label="调用套餐顺序"
          prepend-inner-icon="mdi-swap-vertical"
          append-inner-icon="mdi-menu-right"
          readonly
          @click="modelPlanOrder.openModal()"
          @click:append-inner="modelPlanOrder.openModal()"
        />
        <VTextField
          class="model-order-field"
          :model-value="modalAvailablePlugins.length ? modalPluginSummary : '无插件可用，开发者正在激情Meow Meow中'"
          label="插件"
          prepend-inner-icon="mdi-puzzle-outline"
          :append-inner-icon="modalAvailablePlugins.length ? 'mdi-menu-right' : undefined"
          readonly
          :disabled="!modalAvailablePlugins.length"
          @click="modalAvailablePlugins.length && modelPluginOrder.openModal()"
          @click:append-inner="modelPluginOrder.openModal()"
        />
        <VTextarea
          v-model="modalExtra"
          rows="4"
          label="附加参数"
          placeholder="{}"
          prepend-inner-icon="mdi-code-json"
          persistent-placeholder
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

    <PlanOrderModal
      :open="modelPlanOrder.open.value"
      title="调用套餐排序"
      :max-width="520"
      :draft="modelPlanOrder.draft.value"
      :drag-idx="modelPlanOrder.dragIdx.value"
      :is-selected="modelPlanOrder.isSelected"
      :rank-of="modelPlanOrder.rankOf"
      :toggle="modelPlanOrder.toggle"
      :on-drag-start="modelPlanOrder.onDragStart"
      :on-drag-over="modelPlanOrder.onDragOver"
      :on-drag-end="modelPlanOrder.onDragEnd"
      @close="modelPlanOrder.closeModal()"
    />

    <PlanOrderModal
      :open="modelPluginOrder.open.value"
      title="插件排序"
      description="拖动排序，勾选启用；请求会按此顺序执行插件"
      icon="mdi-puzzle-outline"
      empty-text="当前处理器暂无可用插件"
      :max-width="520"
      :draft="modelPluginOrder.draft.value"
      :drag-idx="modelPluginOrder.dragIdx.value"
      :is-selected="modelPluginOrder.isSelected"
      :rank-of="modelPluginOrder.rankOf"
      :toggle="modelPluginOrder.toggle"
      :on-drag-start="modelPluginOrder.onDragStart"
      :on-drag-over="modelPluginOrder.onDragOver"
      :on-drag-end="modelPluginOrder.onDragEnd"
      :item-label="modalPluginLabel"
      :item-description="modalPluginDescription"
      @close="modelPluginOrder.closeModal()"
    />

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
.model-form-stack {
  display: grid;
  gap: 16px;
  padding-top: 4px;
}

.model-card {
  border: 1px solid rgba(var(--v-theme-outline-variant), 0.62);
  background: rgba(var(--v-theme-surface-container), 0.82) !important;
  box-shadow: inset 0 1px 0 rgba(var(--v-theme-on-surface), 0.035);
}

.model-card-body {
  min-height: 100%;
}

.model-card-actions {
  margin-top: auto;
}

.model-action-button {
  min-width: 78px;
  padding-inline: 13px 15px;
  font-size: 0.82rem;
  font-weight: 700;
}

.model-action-button :deep(.v-btn__content) {
  line-height: 1;
}

.model-action-button :deep(.v-btn__prepend) {
  margin-inline-end: 7px;
}

.model-order-field {
  cursor: pointer;
}

.model-order-field :deep(.v-field) {
  cursor: pointer;
}

.model-order-field :deep(input) {
  cursor: pointer;
  font-weight: 400;
}

.model-plugin-badge {
  max-width: 100%;
  height: auto !important;
  min-height: 32px;
  align-items: center;
  border: 1px solid rgba(var(--v-theme-tertiary), 0.34);
  background: rgba(var(--v-theme-tertiary), 0.07);
}

.model-plan-badge {
  max-width: 100%;
  height: auto !important;
  min-height: 32px;
  align-items: center;
  border: 1px solid rgba(var(--v-theme-secondary), 0.32);
  background: rgba(var(--v-theme-secondary), 0.06);
}

.model-plugin-badge :deep(.v-chip__content),
.model-plan-badge :deep(.v-chip__content) {
  display: block;
  min-width: 0;
  overflow: visible;
  white-space: normal;
  overflow-wrap: normal;
  word-break: normal;
  line-height: 1.35;
  padding-block: 3px;
}

.model-plugin-badge :deep(.v-chip__prepend),
.model-plan-badge :deep(.v-chip__prepend) {
  align-self: center;
  margin-top: 0;
}

.model-plan-summary-inline {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
}

.model-plan-summary-token {
  white-space: nowrap;
}

.model-plan-arrow {
  white-space: pre;
}

.extra-json-panel {
  border: 1px solid rgba(var(--v-theme-outline-variant), 0.58);
  border-radius: 12px;
  background: rgba(var(--v-theme-surface-container-high), 0.74);
}

.extra-json-panel > summary {
  cursor: pointer;
  padding: 7px 10px;
  font-size: 0.75rem;
  font-weight: 700;
  color: rgba(var(--v-theme-on-surface), 0.72);
  user-select: none;
}

.extra-json-panel > pre {
  max-height: 240px;
  margin: 0;
  padding: 0 10px 10px;
  overflow: auto;
  white-space: pre-wrap;
  word-break: break-word;
  font-size: 0.72rem;
  line-height: 1.55;
}
</style>
