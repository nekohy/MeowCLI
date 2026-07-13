<script setup lang="ts">
import { adminApi } from '~/composables/useAdminApi'
import { joinPlanTypeInput, planTypeText, safeStringify, splitPlanTypeInput } from '~/lib/admin'
import {
  DEFAULT_MODEL_CATALOG_URLS,
  modelCatalogStorageKey,
  normalizeModelCatalog,
  type ModelCatalogItem,
} from '~/lib/modelCatalog'
import { resolveCreateModelHandler } from '~/lib/modelForm'
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
const selectedAliases = ref<string[]>([])

const modalOpen = ref(false)
const modalMode = ref<'create' | 'edit'>('create')
const modalAlias = ref('')
const modalOrigin = ref('')
const modalHandler = ref('gemini')
const modalPlanTypes = ref('')
const modalPlugins = ref<string[]>([])
const modalExtra = ref('{}')
const modalError = ref('')
const modelCatalogOpen = ref(false)
const modelCatalogLoading = ref(false)
const modelCatalogError = ref('')
const modelCatalogSearch = ref('')
const modelCatalogUrl = ref('')
const modelCatalogItems = ref<ModelCatalogItem[]>([])
const modelCatalogUrlByHandler = ref<Record<string, string>>({})
const batchModalOpen = ref(false)
const batchPlanTypes = ref('')
const batchPlugins = ref<string[]>([])
const batchExtra = ref('{}')
const batchError = ref('')
const handlerIconByKey: Record<string, string> = {
  codex: 'mdi-console',
  gemini: 'mdi-google-circles-communities',
  antigravity: 'mdi-compass-outline',
  'opencode-go': 'mdi-code-braces-box',
}

const modalHandlerConfig = computed(() => (
  admin.handlers.value.find((handler) => handler.key === modalHandler.value) || null
))
const modalAvailablePlanTypes = computed(() => modalHandlerConfig.value?.plan_list || [])
const modalAvailablePlugins = computed(() => modalHandlerConfig.value?.plugins || [])
const modalSelectedPlanTypes = computed(() => splitPlanTypeInput(modalPlanTypes.value, modalAvailablePlanTypes.value))
const batchSelectionEnabled = computed(() => handlerFilter.value !== 'all')
const batchHandlerConfig = computed(() => (
  admin.handlers.value.find((handler) => handler.key === handlerFilter.value) || null
))
const batchAvailablePlanTypes = computed(() => batchHandlerConfig.value?.plan_list || [])
const batchAvailablePlugins = computed(() => batchHandlerConfig.value?.plugins || [])

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

const batchPlanOrder = usePlanOrderModal(
  () => batchPlanTypes.value,
  (value) => { batchPlanTypes.value = value },
  () => batchAvailablePlanTypes.value,
)

const batchPluginOrder = useOrderedSelectionModal(
  () => batchPlugins.value,
  (value) => { batchPlugins.value = value },
  () => batchAvailablePlugins.value.map((plugin) => plugin.name),
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

const batchPlanSummary = computed(() => (
  batchPlanOrder.preview.value.length
    ? `已选择 ${batchPlanOrder.preview.value.length} 个套餐`
    : '未指定套餐顺序'
))

const batchPluginSummary = computed(() => (
  batchPluginOrder.preview.value.length
    ? `已启用 ${batchPluginOrder.preview.value.length} 个插件`
    : '未启用插件'
))

const modelCatalogHandlerLabel = computed(() => (
  admin.handlerLookup.value.get(modalHandler.value)?.label || modalHandler.value
))

const modelCatalogAvailable = computed(() => Boolean(DEFAULT_MODEL_CATALOG_URLS[modalHandler.value]))
const modelCatalogUsingDefaultUrl = computed(() => (
  modelCatalogUrl.value.trim() === defaultModelCatalogUrl(modalHandler.value)
))

const filteredModelCatalogItems = computed(() => {
  const query = modelCatalogSearch.value.trim().toLowerCase()
  if (!query) {
    return modelCatalogItems.value
  }

  return modelCatalogItems.value.filter((item) => (
    item.id.toLowerCase().includes(query)
    || item.name.toLowerCase().includes(query)
    || item.description.toLowerCase().includes(query)
  ))
})
const plainModelCatalogItems = computed(() => filteredModelCatalogItems.value.filter((item) => !item.description))
const describedModelCatalogItems = computed(() => filteredModelCatalogItems.value.filter((item) => item.description))

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
const selectedAliasSet = computed(() => new Set(selectedAliases.value))
const selectedBatchItems = computed(() => (
  items.value.filter((item) => item.handler === handlerFilter.value && selectedAliasSet.value.has(item.alias))
))
const allVisibleSelected = computed(() => (
  batchSelectionEnabled.value
  && filteredItems.value.length > 0
  && filteredItems.value.every((item) => selectedAliasSet.value.has(item.alias))
))
const batchModalTitle = computed(() => (
  batchHandlerConfig.value
    ? `批量编辑 ${batchHandlerConfig.value.label} 模型`
    : '批量编辑模型'
))

function modelsForHandler(handlerKey: string) {
  return items.value.filter((item) => item.handler === handlerKey).length
}

function handlerIcon(handlerKey: string) {
  return handlerIconByKey[handlerKey] || 'mdi-cpu-64-bit'
}

function defaultModelCatalogUrl(handlerKey: string) {
  return DEFAULT_MODEL_CATALOG_URLS[handlerKey] || ''
}

function loadModelCatalogUrl(handlerKey: string) {
  const defaultUrl = defaultModelCatalogUrl(handlerKey)
  if (!import.meta.client) {
    return defaultUrl
  }

  try {
    return localStorage.getItem(modelCatalogStorageKey(handlerKey))?.trim() || defaultUrl
  } catch {
    return defaultUrl
  }
}

function saveModelCatalogUrl(handlerKey: string, url: string) {
  const trimmed = url.trim()
  const defaultUrl = defaultModelCatalogUrl(handlerKey)
  modelCatalogUrlByHandler.value = {
    ...modelCatalogUrlByHandler.value,
    [handlerKey]: trimmed || defaultUrl,
  }
  if (!import.meta.client) {
    return
  }

  try {
    if (!trimmed || trimmed === defaultUrl) {
      localStorage.removeItem(modelCatalogStorageKey(handlerKey))
    } else {
      localStorage.setItem(modelCatalogStorageKey(handlerKey), trimmed)
    }
  } catch {
    // localStorage may be unavailable in private contexts; the in-memory value still works.
  }
}

function resetModelCatalogUrl() {
  const handlerKey = modalHandler.value
  const defaultUrl = defaultModelCatalogUrl(handlerKey)
  modelCatalogUrl.value = defaultUrl
  saveModelCatalogUrl(handlerKey, defaultUrl)
}

async function fetchModelCatalog() {
  const handlerKey = modalHandler.value
  const url = modelCatalogUrl.value.trim()
  modelCatalogLoading.value = true
  modelCatalogError.value = ''

  try {
    if (!url) {
      throw new Error('模型列表链接不能为空')
    }
    const response = await fetch(url, {
      headers: { Accept: 'application/json' },
      cache: 'no-store',
    })
    if (!response.ok) {
      throw new Error(`模型列表获取失败 (${response.status})`)
    }
    modelCatalogItems.value = normalizeModelCatalog(await response.json())
    saveModelCatalogUrl(handlerKey, url)
    if (!modelCatalogItems.value.length) {
      modelCatalogError.value = '远程列表为空'
    }
  } catch (error) {
    modelCatalogItems.value = []
    modelCatalogError.value = error instanceof Error ? error.message : '模型列表获取失败'
  } finally {
    modelCatalogLoading.value = false
  }
}

function openModelCatalog() {
  if (!modelCatalogAvailable.value) {
    return
  }
  const handlerKey = modalHandler.value
  const url = loadModelCatalogUrl(handlerKey)
  modelCatalogUrlByHandler.value = {
    ...modelCatalogUrlByHandler.value,
    [handlerKey]: url,
  }
  modelCatalogUrl.value = url
  modelCatalogSearch.value = ''
  modelCatalogError.value = ''
  modelCatalogItems.value = []
  modelCatalogOpen.value = true
  void fetchModelCatalog()
}

function closeModelCatalog() {
  modelCatalogOpen.value = false
  modelCatalogError.value = ''
}

function selectModelCatalogItem(item: ModelCatalogItem) {
  modalOrigin.value = item.id
  closeModelCatalog()
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
  modalHandler.value = resolveCreateModelHandler(
    handlerFilter.value,
    admin.activeHandler.value?.key,
    admin.handlers.value.map((handler) => handler.key),
  )
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

function clearSelection() {
  selectedAliases.value = []
}

function toggleSelectVisible() {
  if (!batchSelectionEnabled.value) {
    clearSelection()
    return
  }
  if (allVisibleSelected.value) {
    const visible = new Set(filteredItems.value.map((item) => item.alias))
    selectedAliases.value = selectedAliases.value.filter((alias) => !visible.has(alias))
    return
  }
  const aliases = new Set(selectedAliases.value)
  filteredItems.value.forEach((item) => aliases.add(item.alias))
  selectedAliases.value = [...aliases]
}

function toggleSelectModel(alias: string) {
  if (!batchSelectionEnabled.value) {
    return
  }
  selectedAliases.value = selectedAliasSet.value.has(alias)
    ? selectedAliases.value.filter((value) => value !== alias)
    : [...selectedAliases.value, alias]
}

function openBatchModal() {
  if (!batchSelectionEnabled.value || !selectedBatchItems.value.length) {
    return
  }
  batchPlanTypes.value = defaultPlanTypesForHandler(handlerFilter.value)
  batchPlugins.value = []
  batchExtra.value = '{}'
  batchError.value = ''
  batchModalOpen.value = true
}

function closeBatchModal() {
  batchModalOpen.value = false
  batchError.value = ''
  batchPlanOrder.closeModal()
  batchPluginOrder.closeModal()
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

function batchPluginLabel(name: string) {
  return pluginLabel(name, batchAvailablePlugins.value)
}

function batchPluginDescription(name: string) {
  return batchAvailablePlugins.value.find((plugin) => plugin.name === name)?.description || ''
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

async function saveBatchModels() {
  actionBusy.value = true
  batchError.value = ''

  try {
    if (!batchSelectionEnabled.value || !selectedBatchItems.value.length) {
      throw new Error('请选择同一处理器下的模型')
    }
    let extra: Record<string, unknown> = {}
    try {
      extra = JSON.parse(batchExtra.value || '{}') as Record<string, unknown>
    } catch {
      throw new Error('附加参数必须是合法的 JSON')
    }

    const response = await adminApi.batchUpdateModels(admin.token.value, {
      aliases: selectedBatchItems.value.map((item) => item.alias),
      handler: handlerFilter.value,
      plan_types: joinPlanTypeInput(splitPlanTypeInput(batchPlanTypes.value, batchAvailablePlanTypes.value), batchAvailablePlanTypes.value),
      plugin: batchPlugins.value
        .filter((name) => batchAvailablePlugins.value.some((plugin) => plugin.name === name))
        .join(','),
      extra,
    })

    closeBatchModal()
    clearSelection()
    const failed = response.errors.length
    admin.notify(
      failed
        ? `已更新 ${response.updated.length} 个模型，${failed} 个失败`
        : `已批量更新 ${response.updated.length} 个模型`,
      failed ? 'warning' : 'success',
    )
    await Promise.all([
      admin.loadOverview(admin.token.value, true),
      loadModels(),
    ])
  } catch (error) {
    batchError.value = error instanceof Error ? error.message : '批量更新模型失败'
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
  () => handlerFilter.value,
  () => {
    clearSelection()
    closeBatchModal()
  },
)

watch(
  () => items.value,
  () => {
    if (!batchSelectionEnabled.value) {
      clearSelection()
      return
    }
    const available = new Set(items.value
      .filter((item) => item.handler === handlerFilter.value)
      .map((item) => item.alias))
    selectedAliases.value = selectedAliases.value.filter((alias) => available.has(alias))
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

        <div v-if="batchSelectionEnabled" class="batch-toolbar">
          <div class="batch-select-summary">
            <VCheckboxBtn
              :model-value="allVisibleSelected"
              :indeterminate="selectedBatchItems.length > 0 && !allVisibleSelected"
              :disabled="!filteredItems.length"
              @update:model-value="toggleSelectVisible"
            />
            <span>已选 {{ selectedBatchItems.length }} 个</span>
          </div>
          <AdminButton
            variant="secondary"
            size="sm"
            prepend-icon="mdi-pencil-outline"
            class="model-action-button batch-action-button"
            :disabled="!selectedBatchItems.length"
            @click="openBatchModal"
          >
            批量编辑
          </AdminButton>
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
              <div class="model-card-title-row">
                <VCheckboxBtn
                  v-if="batchSelectionEnabled"
                  :model-value="selectedAliasSet.has(item.alias)"
                  class="model-select-check"
                  @update:model-value="() => toggleSelectModel(item.alias)"
                />
                <div style="min-width: 0">
                  <div class="text-h6 font-weight-bold">{{ item.alias }}</div>
                  <div class="text-caption text-medium-emphasis text-truncate" style="max-width: 280px">{{ item.origin }}</div>
                </div>
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
        <div class="model-origin-row">
          <VTextField
            v-model="modalOrigin"
            label="上游模型"
            placeholder="gpt-4-0125-preview"
            prepend-inner-icon="mdi-cloud-outline"
            persistent-placeholder
            class="model-origin-field"
          />
          <AdminButton
            variant="secondary"
            prepend-icon="mdi-format-list-bulleted"
            class="model-catalog-trigger"
            :disabled="!modelCatalogAvailable"
            @click="openModelCatalog"
          >
            模型列表
          </AdminButton>
        </div>
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

    <ModalDialog
      :open="modelCatalogOpen"
      :title="`${modelCatalogHandlerLabel} 模型列表`"
      description=""
      icon="mdi-format-list-bulleted-square"
      max-width="860"
      @close="closeModelCatalog"
    >
      <div class="model-catalog-stack">
        <div class="model-catalog-controls">
          <VTextField
            v-model="modelCatalogUrl"
            label="模型列表链接"
            prepend-inner-icon="mdi-vector-link"
            persistent-placeholder
            hide-details
          />
          <div class="model-catalog-actions">
            <AdminButton
              variant="ghost"
              prepend-icon="mdi-keyboard-return"
              :disabled="modelCatalogUsingDefaultUrl"
              @click="resetModelCatalogUrl"
            >
              默认
            </AdminButton>
            <AdminButton
              variant="secondary"
              prepend-icon="mdi-cached"
              :loading="modelCatalogLoading"
              @click="fetchModelCatalog"
            >
              刷新
            </AdminButton>
          </div>
        </div>

        <VAlert
          v-if="modelCatalogError"
          type="error"
          variant="tonal"
          density="comfortable"
          :text="modelCatalogError"
        />

        <div v-if="filteredModelCatalogItems.length" class="model-catalog-list">
          <button
            v-for="item in plainModelCatalogItems"
            :key="item.id"
            type="button"
            class="model-catalog-item"
            @click="selectModelCatalogItem(item)"
          >
            <span class="model-catalog-item-main">
              <span class="model-catalog-name">{{ item.name }}</span>
              <span class="model-catalog-id">{{ item.id }}</span>
            </span>
          </button>

          <div v-if="describedModelCatalogItems.length" class="model-catalog-described-group">
            <button
              v-for="item in describedModelCatalogItems"
              :key="item.id"
              type="button"
              class="model-catalog-item model-catalog-item--described"
              @click="selectModelCatalogItem(item)"
            >
              <span class="model-catalog-item-main">
                <span class="model-catalog-name">{{ item.name }}</span>
                <span class="model-catalog-id">{{ item.id }}</span>
              </span>
              <span class="model-catalog-description">
                {{ item.description }}
              </span>
            </button>
          </div>
        </div>

        <EmptyState
          v-else-if="!modelCatalogLoading"
          title="没有可选模型"
          description="刷新列表或调整搜索条件"
          icon="mdi-link-off"
        />

        <div v-else class="model-catalog-loading">
          <VProgressCircular indeterminate color="primary" size="28" />
          <span>正在获取模型列表</span>
        </div>
      </div>
      <template #footer>
        <AdminButton variant="ghost" @click="closeModelCatalog">关闭</AdminButton>
      </template>
    </ModalDialog>

    <ModalDialog
      :open="batchModalOpen"
      :title="batchModalTitle"
      :description="`将统一覆盖 ${selectedBatchItems.length} 个已选模型的套餐、插件和附加参数`"
      icon="mdi-pencil-outline"
      max-width="640"
      @close="closeBatchModal"
    >
      <div class="model-form-stack">
        <VTextField
          class="model-order-field"
          :model-value="batchPlanSummary"
          label="调用套餐顺序"
          prepend-inner-icon="mdi-swap-vertical"
          append-inner-icon="mdi-menu-right"
          readonly
          @click="batchPlanOrder.openModal()"
          @click:append-inner="batchPlanOrder.openModal()"
        />
        <VTextField
          class="model-order-field"
          :model-value="batchAvailablePlugins.length ? batchPluginSummary : '当前处理器暂无可用插件'"
          label="插件导入"
          prepend-inner-icon="mdi-puzzle-outline"
          :append-inner-icon="batchAvailablePlugins.length ? 'mdi-menu-right' : undefined"
          readonly
          :disabled="!batchAvailablePlugins.length"
          @click="batchAvailablePlugins.length && batchPluginOrder.openModal()"
          @click:append-inner="batchPluginOrder.openModal()"
        />
        <VTextarea
          v-model="batchExtra"
          rows="4"
          label="附加参数"
          placeholder="{}"
          prepend-inner-icon="mdi-code-json"
          persistent-placeholder
        />
        <VAlert
          v-if="batchError"
          type="error"
          variant="tonal"
          density="comfortable"
          :text="batchError"
        />
      </div>
      <template #footer>
        <AdminButton variant="ghost" @click="closeBatchModal">取消</AdminButton>
        <AdminButton
          prepend-icon="mdi-content-save-check-outline"
          :loading="actionBusy"
          @click="saveBatchModels"
        >
          批量更新
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

    <PlanOrderModal
      :open="batchPlanOrder.open.value"
      title="批量调用套餐排序"
      :max-width="520"
      :draft="batchPlanOrder.draft.value"
      :drag-idx="batchPlanOrder.dragIdx.value"
      :is-selected="batchPlanOrder.isSelected"
      :rank-of="batchPlanOrder.rankOf"
      :toggle="batchPlanOrder.toggle"
      :on-drag-start="batchPlanOrder.onDragStart"
      :on-drag-over="batchPlanOrder.onDragOver"
      :on-drag-end="batchPlanOrder.onDragEnd"
      @close="batchPlanOrder.closeModal()"
    />

    <PlanOrderModal
      :open="batchPluginOrder.open.value"
      title="批量插件排序"
      description="拖动排序，勾选启用；所有已选模型会使用同一插件顺序"
      icon="mdi-puzzle-outline"
      empty-text="当前处理器暂无可用插件"
      :max-width="520"
      :draft="batchPluginOrder.draft.value"
      :drag-idx="batchPluginOrder.dragIdx.value"
      :is-selected="batchPluginOrder.isSelected"
      :rank-of="batchPluginOrder.rankOf"
      :toggle="batchPluginOrder.toggle"
      :on-drag-start="batchPluginOrder.onDragStart"
      :on-drag-over="batchPluginOrder.onDragOver"
      :on-drag-end="batchPluginOrder.onDragEnd"
      :item-label="batchPluginLabel"
      :item-description="batchPluginDescription"
      @close="batchPluginOrder.closeModal()"
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

.model-origin-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 10px;
  align-items: start;
}

.model-origin-field {
  min-width: 0;
}

.model-catalog-trigger {
  min-width: 112px;
  margin-top: 8px;
}

.model-catalog-stack {
  display: grid;
  gap: 14px;
}

.model-catalog-controls {
  display: grid;
  gap: 8px;
}

.model-catalog-actions {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 8px;
}

.model-catalog-list {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(240px, 1fr));
  gap: 8px;
  max-height: min(58vh, 560px);
  overflow: auto;
  padding-right: 2px;
}

.model-catalog-described-group {
  grid-column: 1 / -1;
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(240px, 1fr));
  gap: 8px;
  margin-top: 2px;
}

.model-catalog-item {
  display: grid;
  gap: 6px;
  width: 100%;
  padding: 12px 14px;
  border: 1px solid rgba(var(--v-theme-outline-variant), 0.62);
  border-radius: 8px;
  background: rgba(var(--v-theme-surface-container), 0.72);
  color: rgba(var(--v-theme-on-surface), 0.9);
  text-align: left;
  cursor: pointer;
  transition: border-color 140ms ease, background 140ms ease, transform 140ms ease;
}

.model-catalog-item:hover {
  border-color: rgba(var(--v-theme-primary), 0.58);
  background: rgba(var(--v-theme-primary), 0.07);
  transform: translateY(-1px);
}

.model-catalog-item-main {
  display: grid;
  gap: 4px;
  min-width: 0;
}

.model-catalog-name {
  overflow-wrap: anywhere;
  font-size: 0.92rem;
  font-weight: 800;
  line-height: 1.35;
}

.model-catalog-id {
  overflow-wrap: anywhere;
  color: rgba(var(--v-theme-on-surface), 0.68);
  font-size: 0.8rem;
  line-height: 1.35;
}

.model-catalog-description {
  overflow-wrap: anywhere;
  color: rgba(var(--v-theme-on-surface), 0.74);
  font-size: 0.8rem;
  line-height: 1.45;
}

.model-catalog-loading {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 10px;
  min-height: 140px;
  color: rgba(var(--v-theme-on-surface), 0.7);
  font-size: 0.88rem;
  font-weight: 700;
}

.model-card {
  border: 1px solid rgba(var(--v-theme-outline-variant), 0.62);
  background: rgba(var(--v-theme-surface-container), 0.82) !important;
  box-shadow: inset 0 1px 0 rgba(var(--v-theme-on-surface), 0.035);
}

.model-card-body {
  min-height: 100%;
}

.batch-toolbar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  padding-top: 2px;
}

.batch-select-summary {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  min-height: 34px;
  padding-inline: 2px 10px;
  border: 1px solid rgba(var(--v-theme-outline-variant), 0.58);
  border-radius: 8px;
  background: rgba(var(--v-theme-on-surface), 0.035);
  color: rgba(var(--v-theme-on-surface), 0.78);
  font-size: 0.82rem;
  font-weight: 700;
  white-space: nowrap;
}

.batch-select-summary :deep(.v-selection-control) {
  min-height: 30px;
}

.batch-select-summary :deep(.v-selection-control__wrapper) {
  width: 30px;
  height: 30px;
}

.batch-action-button {
  min-width: 90px;
}

.model-card-title-row {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}

.model-select-check {
  flex: 0 0 auto;
  margin-inline-start: -6px;
}

.model-select-check :deep(.v-selection-control) {
  min-height: 32px;
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

@media (max-width: 720px) {
  .model-origin-row {
    grid-template-columns: 1fr;
  }

  .model-catalog-list {
    grid-template-columns: 1fr;
  }

  .model-catalog-described-group {
    grid-template-columns: 1fr;
  }

  .model-catalog-actions {
    justify-content: stretch;
  }

  .model-catalog-actions > * {
    flex: 1 1 120px;
  }

  .model-catalog-trigger {
    width: 100%;
    margin-top: 0;
  }
}
</style>
