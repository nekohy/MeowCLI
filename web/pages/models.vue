<script setup lang="ts">
import { adminApi } from '~/composables/useAdminApi'
import { joinPlanTypeInput, planTypeText, safeStringify, splitPlanTypeInput, statusText } from '~/lib/admin'
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
const pluginModalOpen = ref(false)
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

const modalSelectedPlugins = computed(() => {
  const available = new Set(modalAvailablePlugins.value.map((plugin) => plugin.name))
  return modalPlugins.value.filter((name) => available.has(name))
})

const modelPlanOrder = usePlanOrderModal(
  () => modalPlanTypes.value,
  (value) => { modalPlanTypes.value = value },
  () => modalAvailablePlanTypes.value,
)

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
  closePluginModal()
}

function openPluginModal() {
  pluginModalOpen.value = true
}

function closePluginModal() {
  pluginModalOpen.value = false
}

function isModalPluginSelected(name: string) {
  return modalSelectedPlugins.value.includes(name)
}

function toggleModalPlugin(name: string) {
  const idx = modalPlugins.value.indexOf(name)
  if (idx >= 0) {
    modalPlugins.value.splice(idx, 1)
    return
  }
  modalPlugins.value.push(name)
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
                {{ modelPlanSummary(item) }}
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
          class="model-plan-panel"
        >
          <div class="d-flex justify-space-between align-center ga-3 flex-wrap">
            <div class="text-subtitle-2 font-weight-bold">调用套餐顺序</div>
            <AdminButton variant="secondary" size="sm" prepend-icon="mdi-swap-vertical" @click="modelPlanOrder.openModal()">
              排序
            </AdminButton>
          </div>
        </VSheet>
        <VSheet
          color="surface-container-high"
          rounded="lg"
          class="model-plugin-panel"
        >
          <div class="d-flex justify-space-between align-center ga-3 flex-wrap">
            <div>
              <div class="text-subtitle-2 font-weight-bold">插件</div>
              <div v-if="!modalAvailablePlugins.length" class="model-plan-summary text-medium-emphasis">
                无插件可用，开发者正在激情Meow Meow中
              </div>
            </div>
            <AdminButton
              variant="secondary"
              size="sm"
              prepend-icon="mdi-puzzle-outline"
              :disabled="!modalAvailablePlugins.length"
              @click="openPluginModal()"
            >
              选择
            </AdminButton>
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

    <PlanOrderModal
      :open="modelPlanOrder.open.value"
      title="调用套餐排序"
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

    <ModalDialog
      :open="pluginModalOpen"
      title="插件选择"
      description="勾选当前模型启用的请求插件"
      icon="mdi-puzzle-outline"
      :max-width="440"
      @close="closePluginModal"
    >
      <div v-if="modalAvailablePlugins.length" class="model-plugin-list" role="group" aria-label="插件">
        <label
          v-for="plugin in modalAvailablePlugins"
          :key="plugin.name"
          class="model-plugin-option"
          :class="{ 'is-selected': isModalPluginSelected(plugin.name) }"
        >
          <input
            :checked="isModalPluginSelected(plugin.name)"
            class="model-plugin-native"
            type="checkbox"
            :value="plugin.name"
            :aria-label="plugin.label"
            @change="toggleModalPlugin(plugin.name)"
          >
          <span class="model-plugin-check" aria-hidden="true">
            <VIcon :icon="isModalPluginSelected(plugin.name) ? 'mdi-checkbox-marked' : 'mdi-checkbox-blank-outline'" />
          </span>
          <span class="model-plugin-label">
            <span>{{ plugin.label }}</span>
            <small>{{ plugin.description }}</small>
          </span>
        </label>
      </div>
      <div v-else class="text-center text-medium-emphasis py-4">
        当前处理器暂无可用插件
      </div>
      <template #footer>
        <VBtn variant="text" @click="closePluginModal">关闭</VBtn>
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
.model-form-stack {
  display: grid;
  gap: 16px;
  padding-top: 4px;
}

.model-plan-panel {
  display: grid;
  gap: 7px;
  padding: 13px 14px;
  border: 1px solid rgba(var(--v-theme-outline-variant), 0.58);
  background: rgba(var(--v-theme-surface), 0.72) !important;
  box-shadow: inset 0 1px 0 rgba(var(--v-theme-on-surface), 0.025);
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

.model-plugin-panel {
  display: grid;
  gap: 8px;
  padding: 13px 14px;
  border: 1px solid rgba(var(--v-theme-outline-variant), 0.58);
  background: rgba(var(--v-theme-surface), 0.72) !important;
}

.model-plugin-badge {
  border: 1px solid rgba(var(--v-theme-tertiary), 0.34);
  background: rgba(var(--v-theme-tertiary), 0.07);
}

.model-plan-badge {
  border: 1px solid rgba(var(--v-theme-secondary), 0.32);
  background: rgba(var(--v-theme-secondary), 0.06);
}

.model-plugin-list {
  display: grid;
  gap: 10px;
}

.model-plugin-option {
  display: grid;
  grid-template-columns: 24px minmax(0, 1fr);
  gap: 12px;
  align-items: start;
  min-height: 62px;
  padding: 12px 14px;
  border: 1px solid rgba(var(--v-theme-outline-variant), 0.62);
  border-radius: 10px;
  background: rgba(var(--v-theme-surface-container-highest), 0.42);
  cursor: pointer;
  transition:
    border-color 140ms ease,
    background-color 140ms ease,
    box-shadow 140ms ease;
}

.model-plugin-option:hover {
  border-color: rgba(var(--v-theme-primary), 0.42);
  background: rgba(var(--v-theme-surface-container-highest), 0.62);
}

.model-plugin-option.is-selected {
  border-color: rgba(var(--v-theme-primary), 0.72);
  background: rgba(var(--v-theme-primary), 0.11);
  box-shadow: inset 0 1px 0 rgba(var(--v-theme-on-surface), 0.035);
}

.model-plugin-native {
  position: absolute;
  width: 1px;
  height: 1px;
  opacity: 0;
  pointer-events: none;
}

.model-plugin-check {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding-top: 1px;
  color: rgb(var(--v-theme-primary));
  line-height: 1;
}

.model-plugin-label {
  display: grid;
  gap: 4px;
  min-width: 0;
  padding-top: 1px;
}

.model-plugin-label > span {
  color: rgba(var(--v-theme-on-surface), 0.92);
  font-size: 0.88rem;
  font-weight: 650;
  line-height: 1.25;
  overflow-wrap: anywhere;
}

.model-plugin-label > small {
  color: rgba(var(--v-theme-on-surface), 0.66);
  font-size: 0.76rem;
  font-weight: 450;
  line-height: 1.4;
  overflow-wrap: anywhere;
}

.model-plan-summary {
  font-size: 0.78rem;
  line-height: 1.45;
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
