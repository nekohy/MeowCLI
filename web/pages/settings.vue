<script setup lang="ts">
import { adminApi } from '~/composables/useAdminApi'
// components 配置为 pathPrefix:false,显式 import 避免命名歧义
import MaintenanceActions from '~/components/settings/MaintenanceActions.vue'
import MaintenanceRow from '~/components/settings/MaintenanceRow.vue'
import {
  ANTIGRAVITY_API_ENDPOINT_OPTIONS,
  DEFAULT_SETTINGS_FORM,
  antigravityAPIEndpointText,
  geminiBaseURLText,
  handlerIcon,
  joinAntigravityAPIEndpointInput,
  joinGeminiBaseURLInput,
  settingsToForm,
  settingsToPayload,
  splitAntigravityAPIEndpointInput,
  splitGeminiBaseURLInput,
  splitPlanTypeInput,
} from '~/lib/admin'
import type { SettingsForm } from '~/types/admin'

definePageMeta({
  navKey: 'settings',
})

const admin = useAdminApp()

const loading = ref(false)
const actionBusy = ref(false)
const geminiEndpointOpen = ref(false)
const antigravityEndpointOpen = ref(false)
const form = ref<SettingsForm>({ ...DEFAULT_SETTINGS_FORM })

const fallbackCodexPlanTypes = ['free', 'plus', 'edu', 'prolite', 'pro', 'team', 'enterprise', 'unknown']
const fallbackGeminiPlanTypes = ['ultra', 'pro', 'free', 'unknown']

const codexPlanTypes = computed(() => admin.handlerLookup.value.get('codex')?.plan_list || fallbackCodexPlanTypes)
const geminiPlanTypes = computed(() => admin.handlerLookup.value.get('gemini')?.plan_list || fallbackGeminiPlanTypes)
const antigravityPlanTypes = computed(() => admin.handlerLookup.value.get('antigravity')?.plan_list || fallbackGeminiPlanTypes)

const codexPlanOrder = usePlanOrderModal(
  () => form.value.codex_preferred_plan_types,
  (v) => { form.value.codex_preferred_plan_types = v },
  () => codexPlanTypes.value,
)

const geminiPlanOrder = usePlanOrderModal(
  () => form.value.gemini_preferred_plan_types,
  (v) => { form.value.gemini_preferred_plan_types = v },
  () => geminiPlanTypes.value,
)

const antigravityPlanOrder = usePlanOrderModal(
  () => form.value.antigravity_preferred_plan_types,
  (v) => { form.value.antigravity_preferred_plan_types = v },
  () => antigravityPlanTypes.value,
)

const hasAntigravityCreditsOverageSetting = computed(() => (
  typeof form.value.antigravity_use_credits === 'boolean'
))

type SettingsCategoryKey = 'global' | 'codex' | 'gemini' | 'antigravity' | 'opencode-go'

const settingsCategories: Array<{ key: SettingsCategoryKey; label: string; icon: string }> = [
  { key: 'global', label: '全局', icon: 'mdi-web' },
  { key: 'codex', label: 'Codex', icon: handlerIcon('codex') },
  { key: 'gemini', label: 'Gemini CLI', icon: handlerIcon('gemini') },
  { key: 'antigravity', label: 'Antigravity', icon: handlerIcon('antigravity') },
  { key: 'opencode-go', label: 'OpenCode Go', icon: handlerIcon('opencode-go') },
]

// 后端未下发 antigravity_use_credits 时说明该 handler 不可用,沿用旧行为整体隐藏该分类
const visibleCategories = computed(() => (
  settingsCategories.filter((category) => (
    category.key !== 'antigravity' || hasAntigravityCreditsOverageSetting.value
  ))
))

const selectedCategory = ref<SettingsCategoryKey>('global')

// 只读校验 + 写入透传:当前分类被隐藏时回退到全局,切换分类不触碰共享表单
const activeCategory = computed<SettingsCategoryKey>({
  get: () => (
    visibleCategories.value.some((category) => category.key === selectedCategory.value)
      ? selectedCategory.value
      : 'global'
  ),
  set: (value) => { selectedCategory.value = value },
})

function makeEndpointSelection(
  get: () => string | undefined,
  set: (value: string) => void,
  split: (value: string) => string[],
  join: (values: string[]) => string,
  format: (value: string) => string,
  noun: string,
) {
  const selection = computed(() => split(get() ?? ''))
  const preview = computed(() => selection.value.map(format).join(' / '))
  const isSelected = (value: string) => selection.value.includes(value)
  const toggle = (value: string) => {
    const selected = split(get() ?? '')
    const idx = selected.indexOf(value)
    if (idx >= 0) {
      // 至少保留一个端点:split 对空数组会静默回退到第一个选项,
      // 在写路径上拦截,避免勾选被静默改成用户没选过的值
      if (selected.length === 1) {
        admin.notify(`至少保留一个${noun}`, 'warning')
        return
      }
      selected.splice(idx, 1)
    } else {
      selected.push(value)
    }
    set(join(selected))
  }
  return { selection, preview, isSelected, toggle }
}

const geminiEndpoint = makeEndpointSelection(
  () => form.value.gemini_base_urls,
  (v) => { form.value.gemini_base_urls = v },
  splitGeminiBaseURLInput,
  joinGeminiBaseURLInput,
  geminiBaseURLText,
  '接口',
)
const antigravityEndpoint = makeEndpointSelection(
  () => form.value.antigravity_api_endpoint,
  (v) => { form.value.antigravity_api_endpoint = v },
  splitAntigravityAPIEndpointInput,
  joinAntigravityAPIEndpointInput,
  antigravityAPIEndpointText,
  '端点',
)

const numericFields = [
  {
    key: 'relay_max_retries',
    label: '重试次数',
    hint: '失败后尝试其他凭据的次数',
    min: 1,
    suffix: '次',
  },
  {
    key: 'weighted_best_count',
    label: '候选池大小',
    hint: '从高分凭据中进入加权随机的候选数量',
    min: 1,
    suffix: '个',
  },
  {
    key: 'import_concurrency',
    label: '导入并发',
    hint: '批量导入凭据时同时处理的任务数',
    min: 1,
    suffix: '个',
  },
  {
    key: 'quota_sync_interval_seconds',
    label: '配额同步',
    hint: '后台同步额度数据的周期',
    min: 1,
    suffix: '秒',
  },
  {
    key: 'score_refresh_interval_seconds',
    label: 'Score刷新',
    hint: '内存调度分数的重算周期',
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
    key: 'max_log_rows',
    label: '日志上限',
    hint: '内存中最多保留的日志条数',
    min: 1,
    suffix: '条',
  },
  {
    key: 'content_affinity_max_entries',
    label: '内容亲和总容量',
    hint: '所有模型合计最多保留的内容亲和记录数',
    min: 1,
    suffix: '条',
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
    fields: ['relay_max_retries', 'weighted_best_count', 'content_affinity_max_entries', 'import_concurrency'] as NumericFieldKey[],
  },
  {
    title: '数据保留',
    fields: ['quota_sync_interval_seconds', 'score_refresh_interval_seconds', 'logs_retention_seconds', 'max_log_rows'] as NumericFieldKey[],
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
    gemini_proxy: source.gemini_proxy.trim(),
    antigravity_proxy: source.antigravity_proxy.trim(),
    opencode_go_proxy: source.opencode_go_proxy.trim(),
    codex_user_agent: source.codex_user_agent.trim(),
    antigravity_user_agent: source.antigravity_user_agent.trim(),
    antigravity_api_endpoint: splitAntigravityAPIEndpointInput(source.antigravity_api_endpoint).join(','),
    codex_preferred_plan_types: splitPlanTypeInput(source.codex_preferred_plan_types, codexPlanTypes.value).join(','),
    gemini_base_urls: splitGeminiBaseURLInput(source.gemini_base_urls).join(','),
    gemini_preferred_plan_types: splitPlanTypeInput(source.gemini_preferred_plan_types, geminiPlanTypes.value).join(','),
    antigravity_preferred_plan_types: splitPlanTypeInput(source.antigravity_preferred_plan_types, antigravityPlanTypes.value).join(','),
  }

  for (const field of numericFields) {
    const parsed = Number.parseInt(String(source[field.key]).trim(), 10)
    const defaultValue = Number.parseInt(DEFAULT_SETTINGS_FORM[field.key], 10)
    next[field.key] = String(Number.isFinite(parsed) && parsed > 0 ? parsed : defaultValue)
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
    admin.notifyError(error, '加载设置失败')
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
    form.value = normalizeSettingsForm(settingsToForm(result.settings))
    admin.notify('设置已保存', 'success')
    await admin.loadOverview(admin.token.value, true)
  } catch (error) {
    admin.notifyError(error, '保存设置失败')
  } finally {
    actionBusy.value = false
  }
}

/* --- 维护操作:独立于设置表单,立即生效,不参与保存 --- */

type ProviderKey = Exclude<SettingsCategoryKey, 'global'>
type Action = 'quota' | 'logs'
type BusyKey = `${ProviderKey | 'all'}:${Action}`

const confirm = useConfirmDialog()
// 按 provider + 动作记录 busy:按钮各自转圈、互不阻塞,并防止重复请求
const busy = ref<Partial<Record<BusyKey, boolean>>>({})

function isBusy(provider: ProviderKey | 'all', action: Action) {
  return Boolean(busy.value[`${provider}:${action}`])
}

function providerBusy(provider: ProviderKey) {
  return isBusy(provider, 'quota') || isBusy(provider, 'logs')
}

const anyBusy = computed(() => Object.values(busy.value).some(Boolean))

function providerLabel(provider: ProviderKey) {
  return settingsCategories.find((category) => category.key === provider)?.label ?? provider
}

async function refreshQuota(provider: ProviderKey) {
  if (providerBusy(provider)) {
    return
  }
  const label = providerLabel(provider)
  busy.value[`${provider}:quota`] = true
  try {
    const result = await adminApi.refreshQuota(admin.token.value, provider)
    admin.notify(`已提交 ${label} 额度刷新，共 ${result.queued} 个任务`)
  } catch (error) {
    admin.notifyError(error, `${label} 额度刷新失败`)
  } finally {
    busy.value[`${provider}:quota`] = false
  }
}

function confirmClearLogs(provider: ProviderKey | 'all') {
  if (provider === 'all' ? anyBusy.value : providerBusy(provider)) {
    return
  }
  const label = provider === 'all' ? '所有渠道' : providerLabel(provider)
  confirm.show({
    title: `清空 ${label} 请求日志`,
    message: `将清空 ${label} 的请求日志，错误率统计随之重置并刷新调度状态，此操作不可恢复。确认继续吗？`,
    confirmText: '确认清空',
    action: async () => {
      const key: BusyKey = `${provider}:logs`
      busy.value[key] = true
      try {
        const result = await adminApi.clearLogs(admin.token.value, provider)
        const detail = result.errors?.length ? `：${result.errors.join('；')}` : ''
        if (result.refreshed && !detail) {
          admin.notify(`已清空 ${label} 请求日志（${result.deleted} 条），调度状态已刷新`)
        } else {
          admin.notify(`已清空 ${label} 请求日志（${result.deleted} 条），但调度状态刷新失败${detail}`, 'warning')
        }
        await admin.loadOverview(admin.token.value, true)
      } catch (error) {
        admin.notifyError(error, '清空日志失败')
      } finally {
        busy.value[key] = false
      }
    },
  })
}

useAuthReadyLoader(loadSettings)
</script>

<template>
  <div class="page-grid">
    <PageHeader
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

    <CategoryTabs
      v-model="activeCategory"
      :items="visibleCategories"
      id-prefix="settings"
    />

    <div class="settings-panels">
      <Transition name="handler-content-fade" mode="out-in">
        <div
          :key="activeCategory"
          :id="`settings-panel-${activeCategory}`"
          role="tabpanel"
          :aria-labelledby="`settings-tab-${activeCategory}`"
        >
          <!-- Global -->
          <SectionCard v-if="activeCategory === 'global'" title="全局" icon="mdi-web">
            <div class="setting-field-stack">
              <div class="settings-item">
                <div class="settings-item-copy">
                  <div class="settings-item-title">全局代理</div>
                  <div class="settings-item-description text-medium-emphasis">所有上游请求使用的代理</div>
                </div>
                <VTextField
                  v-model="form.global_proxy"
                  placeholder="http://127.0.0.1:7890"
                  hide-details
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
                    hide-details
                    class="settings-item-control settings-item-control--number"
                  />
                </div>
              </template>

              <div class="settings-group-divider">维护操作</div>
              <MaintenanceRow
                label="请求日志"
                description="立刻清空所有渠道请求日志"
                action-text="清空所有日志"
                icon="mdi-delete-outline"
                variant="danger"
                :loading="isBusy('all', 'logs')"
                :disabled="anyBusy && !isBusy('all', 'logs')"
                @activate="confirmClearLogs('all')"
              />
            </div>
          </SectionCard>

          <!-- Codex -->
          <SectionCard v-else-if="activeCategory === 'codex'" title="Codex" :icon="handlerIcon('codex')">
            <div class="setting-field-stack">
              <div class="settings-item">
                <div class="settings-item-copy">
                  <div class="settings-item-title">Codex 代理</div>
                  <div class="settings-item-description text-medium-emphasis">未设置时回退到全局代理</div>
                </div>
                <VTextField
                  v-model="form.codex_proxy"
                  placeholder="http://127.0.0.1:7890"
                  hide-details
                  class="settings-item-control"
                />
              </div>

              <div class="settings-item">
                <div class="settings-item-copy">
                  <div class="settings-item-title">UA</div>
                  <div class="settings-item-description text-medium-emphasis">自定义 UA，不懂别动</div>
                </div>
                <VTextField
                  v-model="form.codex_user_agent"
                  hide-details
                  class="settings-item-control"
                />
              </div>

              <SettingNavRow
                title="调用套餐顺序"
                :description="`优先使用的套餐类型及顺序：${codexPlanOrder.preview.value.length ? codexPlanOrder.preview.value.join(' → ') : '未配置'}`"
                @activate="codexPlanOrder.openModal()"
              />

              <div class="settings-item settings-item--toggle">
                <div class="settings-item-copy">
                  <div class="settings-item-title">启用粘性对话</div>
                  <div class="settings-item-description text-medium-emphasis">Codex 目前共享缓存，非必要不用启用，优先于内容粘性</div>
                </div>
                <VSwitch v-model="form.codex_enable_sticky_session" />
              </div>

              <div class="settings-group-divider">维护操作</div>
              <MaintenanceActions
                :quota-busy="isBusy('codex', 'quota')"
                :logs-busy="isBusy('codex', 'logs')"
                @refresh="refreshQuota('codex')"
                @clear="confirmClearLogs('codex')"
              />

            </div>
          </SectionCard>

          <SectionCard v-else-if="activeCategory === 'gemini'" title="Gemini CLI" :icon="handlerIcon('gemini')">
            <div class="setting-field-stack">
              <div class="settings-item">
                <div class="settings-item-copy">
                  <div class="settings-item-title">Gemini CLI 代理</div>
                  <div class="settings-item-description text-medium-emphasis">未设置时回退到全局代理</div>
                </div>
                <VTextField
                  v-model="form.gemini_proxy"
                  placeholder="http://127.0.0.1:7890"
                  hide-details
                  class="settings-item-control"
                />
              </div>

              <SettingNavRow
                title="Gemini CLI 接口"
                :description="`已启用：${geminiEndpoint.preview.value}`"
                @activate="geminiEndpointOpen = true"
              />

              <SettingNavRow
                title="调用套餐顺序"
                :description="`优先使用的套餐类型及顺序：${geminiPlanOrder.preview.value.length ? geminiPlanOrder.preview.value.join(' → ') : '未配置'}`"
                @activate="geminiPlanOrder.openModal()"
              />

              <div class="settings-group-divider">维护操作</div>
              <MaintenanceActions
                :quota-busy="isBusy('gemini', 'quota')"
                :logs-busy="isBusy('gemini', 'logs')"
                @refresh="refreshQuota('gemini')"
                @clear="confirmClearLogs('gemini')"
              />

            </div>
          </SectionCard>

          <SectionCard
            v-else-if="activeCategory === 'antigravity'"
            title="Antigravity"
            :icon="handlerIcon('antigravity')"
          >
            <div class="setting-field-stack">
              <div class="settings-item">
                <div class="settings-item-copy">
                  <div class="settings-item-title">Antigravity 代理</div>
                  <div class="settings-item-description text-medium-emphasis">未设置时回退到全局代理</div>
                </div>
                <VTextField
                  v-model="form.antigravity_proxy"
                  placeholder="http://127.0.0.1:7890"
                  hide-details
                  class="settings-item-control"
                />
              </div>

              <div class="settings-item">
                <div class="settings-item-copy">
                  <div class="settings-item-title">UA</div>
                  <div class="settings-item-description text-medium-emphasis">自定义 UA，不懂别动</div>
                </div>
                <VTextField
                  v-model="form.antigravity_user_agent"
                  hide-details
                  class="settings-item-control"
                />
              </div>

              <SettingNavRow
                title="API 端点"
                :description="`已启用：${antigravityEndpoint.preview.value}`"
                @activate="antigravityEndpointOpen = true"
              />

              <div class="settings-item settings-item--toggle">
                <div class="settings-item-copy">
                  <div class="settings-item-title">配额耗尽后使用 Credits</div>
                  <div class="settings-item-description text-medium-emphasis">仅作为 Antigravity 配额兜底，不会作为套餐类型参与调度筛选</div>
                </div>
                <VSwitch v-model="form.antigravity_use_credits" />
              </div>

              <SettingNavRow
                title="调用套餐顺序"
                :description="`优先使用的套餐类型及顺序：${antigravityPlanOrder.preview.value.length ? antigravityPlanOrder.preview.value.join(' → ') : '未配置'}`"
                @activate="antigravityPlanOrder.openModal()"
              />

              <div class="settings-group-divider">维护操作</div>
              <MaintenanceActions
                :quota-busy="isBusy('antigravity', 'quota')"
                :logs-busy="isBusy('antigravity', 'logs')"
                @refresh="refreshQuota('antigravity')"
                @clear="confirmClearLogs('antigravity')"
              />
            </div>
          </SectionCard>

          <SectionCard v-else-if="activeCategory === 'opencode-go'" title="OpenCode Go" :icon="handlerIcon('opencode-go')">
            <div class="setting-field-stack">
              <div class="settings-item">
                <div class="settings-item-copy">
                  <div class="settings-item-title">OpenCode Go 代理</div>
                  <div class="settings-item-description text-medium-emphasis">未设置时回退到全局代理</div>
                </div>
                <VTextField
                  v-model="form.opencode_go_proxy"
                  placeholder="http://127.0.0.1:7890"
                  hide-details
                  class="settings-item-control"
                />
              </div>

              <div class="settings-group-divider">维护操作</div>
              <MaintenanceActions
                :quota-busy="isBusy('opencode-go', 'quota')"
                :logs-busy="isBusy('opencode-go', 'logs')"
                @refresh="refreshQuota('opencode-go')"
                @clear="confirmClearLogs('opencode-go')"
              />

            </div>
          </SectionCard>
        </div>
      </Transition>
    </div>

    <!-- Plan Order Modals -->
    <OrderedSelectionHost :modal="codexPlanOrder" title="调用套餐顺序" />
    <OrderedSelectionHost :modal="geminiPlanOrder" title="调用套餐顺序" />
    <OrderedSelectionHost :modal="antigravityPlanOrder" title="套餐顺序" />

    <EndpointSelectionModal
      :open="geminiEndpointOpen"
      title="Gemini CLI 接口"
      :selected="geminiEndpoint.selection.value"
      :is-selected="geminiEndpoint.isSelected"
      :toggle="geminiEndpoint.toggle"
      @close="geminiEndpointOpen = false"
    />

    <EndpointSelectionModal
      :open="antigravityEndpointOpen"
      title="Antigravity API 端点"
      :options="ANTIGRAVITY_API_ENDPOINT_OPTIONS"
      :selected="antigravityEndpoint.selection.value"
      :is-selected="antigravityEndpoint.isSelected"
      :toggle="antigravityEndpoint.toggle"
      @close="antigravityEndpointOpen = false"
    />

    <ConfirmDialogHost
      :confirm="confirm"
      :action-busy="anyBusy"
      icon="mdi-delete-outline"
    />
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
</style>
