<script setup lang="ts">
import { adminApi } from '~/composables/useAdminApi'
import {
  DEFAULT_SETTINGS_FORM,
  joinPlanTypeInput,
  settingsToForm,
  settingsToPayload,
  splitPlanTypeInput,
} from '~/lib/admin'
import type { SettingsForm } from '~/types/admin'

definePageMeta({
  navKey: 'settings',
})

const admin = useAdminApp()

const loading = ref(false)
const actionBusy = ref(false)
const form = ref<SettingsForm>({ ...DEFAULT_SETTINGS_FORM })

const availablePlanTypes = computed(() => admin.activeHandler.value?.plan_list || [])

// --- Plan type order modal ---
const planOrderOpen = ref(false)
const planOrderDraft = ref<string[]>([])

function openPlanOrderModal() {
  const selected = splitPlanTypeInput(form.value.codex_preferred_plan_types)
  const unselected = availablePlanTypes.value.filter((t) => !selected.includes(t))
  planOrderDraft.value = [...selected, ...unselected]
  planOrderOpen.value = true
}

function planOrderDraftSelected(planType: string) {
  const selected = splitPlanTypeInput(form.value.codex_preferred_plan_types)
  return selected.includes(planType)
}

function togglePlanOrderDraft(planType: string) {
  const selected = splitPlanTypeInput(form.value.codex_preferred_plan_types)
  const idx = selected.indexOf(planType)
  if (idx >= 0) {
    selected.splice(idx, 1)
  } else {
    selected.push(planType)
  }
  form.value.codex_preferred_plan_types = joinPlanTypeInput(selected)
  // re-sort draft: selected first (keeping their order), unselected after
  const newSelected = splitPlanTypeInput(form.value.codex_preferred_plan_types)
  const remaining = planOrderDraft.value.filter((t) => !newSelected.includes(t))
  planOrderDraft.value = [...newSelected, ...remaining]
}

// drag state
const dragIdx = ref<number | null>(null)

function onDragStart(idx: number) {
  dragIdx.value = idx
}

function onDragOver(e: DragEvent, idx: number) {
  e.preventDefault()
  if (dragIdx.value === null || dragIdx.value === idx) return
  const list = [...planOrderDraft.value]
  const moved = list.splice(dragIdx.value, 1)[0]
  if (moved === undefined) return
  list.splice(idx, 0, moved)
  planOrderDraft.value = list
  dragIdx.value = idx
}

function onDragEnd() {
  dragIdx.value = null
  // sync: only keep items that are selected, in draft order
  const selected = new Set(splitPlanTypeInput(form.value.codex_preferred_plan_types))
  const ordered = planOrderDraft.value.filter((t) => selected.has(t))
  form.value.codex_preferred_plan_types = joinPlanTypeInput(ordered)
}

const planOrderPreview = computed(() => splitPlanTypeInput(form.value.codex_preferred_plan_types))

const numericFields = [
  {
    key: 'relay_max_retries',
    label: '重试次数',
    hint: '失败后尝试其他凭据的次数',
    min: 1,
    suffix: '次',
  },
  {
    key: 'refresh_before_seconds',
    label: '预刷新窗口',
    hint: '令牌到期前的刷新提前量',
    min: 1,
    suffix: '秒',
  },
  {
    key: 'poll_interval_milliseconds',
    label: '轮询间隔',
    hint: '等待并发刷新的检查频率',
    min: 1,
    suffix: 'ms',
  },
  {
    key: 'quota_sync_interval_seconds',
    label: '配额同步',
    hint: '后台同步额度数据的周期',
    min: 1,
    suffix: '秒',
  },
  {
    key: 'logs_retention_seconds',
    label: '日志保留',
    hint: '内存日志存留时长',
    min: 1,
    suffix: '秒',
  },
  {
    key: 'throttle_base_seconds',
    label: '退避起始',
    hint: '首次退避等待时长',
    min: 1,
    suffix: '秒',
  },
  {
    key: 'throttle_max_seconds',
    label: '退避上限',
    hint: '指数退避的最长等待',
    min: 1,
    suffix: '秒',
  },
] as const satisfies Array<{
  key: keyof SettingsForm
  label: string
  hint: string
  min: number
  suffix: string
}>

type NumericFieldKey = (typeof numericFields)[number]['key']

const numericFieldLookup = new Map<NumericFieldKey, (typeof numericFields)[number]>(
  numericFields.map((field) => [field.key, field]),
)

const numericGroups = [
  {
    title: '调度策略',
    fields: ['relay_max_retries', 'refresh_before_seconds', 'poll_interval_milliseconds'] as NumericFieldKey[],
  },
  {
    title: '数据保留',
    fields: ['quota_sync_interval_seconds', 'logs_retention_seconds'] as NumericFieldKey[],
  },
  {
    title: '指数退避',
    fields: ['throttle_base_seconds', 'throttle_max_seconds'] as NumericFieldKey[],
  },
].map((group) => ({
  ...group,
  fields: group.fields
    .map((key) => numericFieldLookup.get(key))
    .filter(Boolean),
}))

function normalizeSettingsForm(source: SettingsForm): SettingsForm {
  const next: SettingsForm = {
    ...source,
    global_proxy: source.global_proxy.trim(),
    codex_proxy: source.codex_proxy.trim(),
    codex_preferred_plan_types: joinPlanTypeInput(splitPlanTypeInput(source.codex_preferred_plan_types)),
  }

  for (const field of numericFields) {
    const parsed = Number.parseInt(String(source[field.key]).trim(), 10)
    const fallback = Number.parseInt(DEFAULT_SETTINGS_FORM[field.key], 10)
    next[field.key] = String(Number.isFinite(parsed) && parsed > 0 ? parsed : fallback)
  }

  return next
}


async function loadSettings() {
  if (!admin.token.value) {
    return
  }

  loading.value = true
  try {
    form.value = normalizeSettingsForm(settingsToForm(await adminApi.getSettings(admin.token.value)))
  } catch (error) {
    admin.notify(error instanceof Error ? error.message : '加载设置失败', 'danger')
  } finally {
    loading.value = false
  }
}

async function saveSettings() {
  actionBusy.value = true
  try {
    const normalized = normalizeSettingsForm(form.value)
    form.value = normalized
    const result = await adminApi.updateSettings(admin.token.value, settingsToPayload(normalized))
    form.value = normalizeSettingsForm(settingsToForm(result.settings || {}))
    admin.notify('设置已保存', 'success')
    await admin.loadOverview(admin.token.value, true)
  } catch (error) {
    admin.notify(error instanceof Error ? error.message : '保存设置失败', 'danger')
  } finally {
    actionBusy.value = false
  }
}

onMounted(() => {
  if (admin.authReady.value) {
    void loadSettings()
  }
})

watch(
  () => admin.authReady.value,
  (ready) => {
    if (ready) {
      void loadSettings()
    }
  },
)
</script>

<template>
  <div class="page-grid">
    <PageHeader
      eyebrow="系统"
      title="系统设置"
      icon="mdi-cog-outline"
    >
      <template #actions>
        <AdminButton
          prepend-icon="mdi-content-save-outline"
          :loading="actionBusy"
          @click="saveSettings"
        >
          保存
        </AdminButton>
      </template>
    </PageHeader>

    <VProgressLinear v-if="loading" indeterminate color="primary" rounded class="mb-2" />

    <!-- Global -->
    <SectionCard title="全局" icon="mdi-earth">
      <div class="setting-field-stack">
        <div class="settings-item settings-item--toggle">
          <div class="settings-item-copy">
            <div class="settings-item-title">透传 PlanType 请求头</div>
            <div class="settings-item-description text-medium-emphasis">允许客户端通过请求头指定套餐类型</div>
          </div>
          <VSwitch v-model="form.allow_user_plan_type_header" />
        </div>

        <div class="settings-item">
          <div class="settings-item-copy">
            <div class="settings-item-title">全局代理</div>
            <div class="settings-item-description text-medium-emphasis">所有上游请求使用的 HTTP 代理</div>
          </div>
          <VTextField
            v-model="form.global_proxy"
            placeholder="http://127.0.0.1:7890"
            variant="outlined"
            density="comfortable"
            class="settings-item-control"
          />
        </div>

        <template v-for="group in numericGroups" :key="group.title">
          <div class="settings-group-divider">{{ group.title }}</div>
          <div v-for="field in group.fields" :key="field!.key" class="settings-item">
            <div class="settings-item-copy">
              <div class="settings-item-title">{{ field!.label }}</div>
              <div class="settings-item-description text-medium-emphasis">{{ field!.hint }}</div>
            </div>
            <VTextField
              v-model="form[field!.key]"
              type="number"
              :min="field!.min"
              :suffix="field!.suffix"
              variant="outlined"
              density="comfortable"
              class="settings-item-control settings-item-control--number"
            />
          </div>
        </template>
      </div>
    </SectionCard>

    <!-- Codex -->
    <SectionCard title="Codex" icon="mdi-console">
      <div class="setting-field-stack">
        <div class="settings-item">
          <div class="settings-item-copy">
            <div class="settings-item-title">Codex代理</div>
            <div class="settings-item-description text-medium-emphasis">仅 Codex 上游使用的 HTTP 代理，覆盖全局代理</div>
          </div>
          <VTextField
            v-model="form.codex_proxy"
            placeholder="http://127.0.0.1:7890"
            variant="outlined"
            density="comfortable"
            class="settings-item-control"
          />
        </div>

        <div class="settings-item settings-item--toggle" style="cursor: pointer" @click="openPlanOrderModal">
          <div class="settings-item-copy">
            <div class="settings-item-title">调用套餐顺序</div>
            <div class="settings-item-description text-medium-emphasis">
              优先使用的套餐类型及顺序：{{ planOrderPreview.length ? planOrderPreview.join(' → ') : '未配置' }}
            </div>
          </div>
          <VIcon icon="mdi-chevron-right" />
        </div>

        <div class="settings-item settings-item--toggle">
          <div class="settings-item-copy">
            <div class="settings-item-title">允许 PlanType 请求头</div>
            <div class="settings-item-description text-medium-emphasis">允许客户端为 Codex 请求指定套餐类型</div>
          </div>
          <VSwitch v-model="form.codex_allow_user_plan_type_header" />
        </div>
      </div>
    </SectionCard>

    <!-- Plan Order Modal -->
    <ModalDialog
      :open="planOrderOpen"
      title="调用套餐顺序"
      description="拖动排序，勾选启用"
      icon="mdi-swap-vertical"
      :max-width="400"
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
          @dragover="(e) => onDragOver(e, idx)"
          @dragend="onDragEnd"
        >
          <VIcon icon="mdi-drag" size="18" class="plan-order-drag text-medium-emphasis" />
          <VCheckbox
            :model-value="planOrderDraftSelected(planType)"
            density="compact"
            hide-details
            @update:model-value="togglePlanOrderDraft(planType)"
            @click.stop
          />
          <span class="plan-order-label">{{ planType }}</span>
          <span v-if="planOrderDraftSelected(planType)" class="plan-order-rank text-medium-emphasis">
            #{{ splitPlanTypeInput(form.codex_preferred_plan_types).indexOf(planType) + 1 }}
          </span>
        </div>
      </div>
      <div v-if="!planOrderDraft.length" class="text-center text-medium-emphasis py-4">
        暂无可用套餐类型
      </div>
      <template #footer>
        <VBtn variant="text" @click="planOrderOpen = false">关闭</VBtn>
      </template>
    </ModalDialog>
  </div>
</template>

<style scoped>
.settings-group-divider {
  font-size: 1rem;
  line-height: 1.3;
  font-weight: 800;
  letter-spacing: 0.03em;
  text-transform: uppercase;
  color: rgba(var(--v-theme-on-surface), 0.65);
  padding-top: 12px;
  padding-bottom: 4px;
}

.plan-order-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.plan-order-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 10px;
  border-radius: 10px;
  background: rgba(var(--v-theme-on-surface), 0.04);
  transition: background 0.15s, opacity 0.15s;
  user-select: none;
}

.plan-order-item:hover {
  background: rgba(var(--v-theme-on-surface), 0.08);
}

.plan-order-item--selected {
  background: rgba(var(--v-theme-primary), 0.08);
}

.plan-order-item--selected:hover {
  background: rgba(var(--v-theme-primary), 0.14);
}

.plan-order-item--dragging {
  opacity: 0.5;
}

.plan-order-drag {
  cursor: grab;
}

.plan-order-drag:active {
  cursor: grabbing;
}

.plan-order-label {
  flex: 1;
  font-weight: 600;
  font-size: 0.875rem;
}

.plan-order-rank {
  font-size: 0.75rem;
  font-weight: 700;
}
</style>
