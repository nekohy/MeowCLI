<script setup lang="ts">
import { adminApi } from '~/composables/useAdminApi'
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

const geminiEndpointSelection = computed(() => splitGeminiBaseURLInput(form.value.gemini_base_urls))
const geminiEndpointPreview = computed(() => geminiEndpointSelection.value.map(geminiBaseURLText).join(' / '))
const antigravityEndpointSelection = computed(() => splitAntigravityAPIEndpointInput(form.value.antigravity_api_endpoint))
const antigravityEndpointPreview = computed(() => antigravityEndpointSelection.value.map(antigravityAPIEndpointText).join(' / '))

function isGeminiEndpointSelected(value: string) {
  return geminiEndpointSelection.value.includes(value)
}

function toggleGeminiEndpoint(value: string) {
  const selected = splitGeminiBaseURLInput(form.value.gemini_base_urls)
  const idx = selected.indexOf(value)
  if (idx >= 0) {
    // 至少保留一个端点:split 对空数组会静默回退到第一个选项,
    // 在写路径上拦截,避免勾选被静默改成用户没选过的值
    if (selected.length === 1) {
      admin.notify('至少保留一个接口', 'warning')
      return
    }
    selected.splice(idx, 1)
  } else {
    selected.push(value)
  }
  form.value.gemini_base_urls = joinGeminiBaseURLInput(selected)
}

function isAntigravityEndpointSelected(value: string) {
  return antigravityEndpointSelection.value.includes(value)
}

function toggleAntigravityEndpoint(value: string) {
  const selected = splitAntigravityAPIEndpointInput(form.value.antigravity_api_endpoint)
  const idx = selected.indexOf(value)
  if (idx >= 0) {
    // 同上:至少保留一个端点
    if (selected.length === 1) {
      admin.notify('至少保留一个端点', 'warning')
      return
    }
    selected.splice(idx, 1)
  } else {
    selected.push(value)
  }
  form.value.antigravity_api_endpoint = joinAntigravityAPIEndpointInput(selected)
}

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
    key: 'refresh_before_seconds',
    label: '预刷新窗口',
    hint: '令牌到期前的刷新提前量',
    min: 1,
    suffix: '秒',
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
    fields: ['relay_max_retries', 'weighted_best_count', 'content_affinity_max_entries', 'import_concurrency', 'refresh_before_seconds'] as NumericFieldKey[],
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
    form.value = normalizeSettingsForm(settingsToForm(result.settings))
    admin.notify('设置已保存', 'success')
    await admin.loadOverview(admin.token.value, true)
  } catch (error) {
    admin.notify(error instanceof Error ? error.message : '保存设置失败', 'danger')
  } finally {
    actionBusy.value = false
  }
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

    <!-- Global -->
    <SectionCard title="全局" icon="mdi-earth">
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
      </div>
    </SectionCard>

    <!-- Codex -->
    <SectionCard title="Codex" :icon="handlerIcon('codex')">
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

      </div>
    </SectionCard>

    <SectionCard title="Gemini CLI" :icon="handlerIcon('gemini')">
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
          :description="`已启用：${geminiEndpointPreview}`"
          @activate="geminiEndpointOpen = true"
        />

        <SettingNavRow
          title="调用套餐顺序"
          :description="`优先使用的套餐类型及顺序：${geminiPlanOrder.preview.value.length ? geminiPlanOrder.preview.value.join(' → ') : '未配置'}`"
          @activate="geminiPlanOrder.openModal()"
        />

      </div>
    </SectionCard>

    <SectionCard
      v-if="hasAntigravityCreditsOverageSetting"
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
          :description="`已启用：${antigravityEndpointPreview}`"
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
      </div>
    </SectionCard>

    <SectionCard title="OpenCode Go" :icon="handlerIcon('opencode-go')">
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

      </div>
    </SectionCard>

    <!-- Plan Order Modals -->
    <OrderedSelectionHost :modal="codexPlanOrder" title="调用套餐顺序" />
    <OrderedSelectionHost :modal="geminiPlanOrder" title="调用套餐顺序" />
    <OrderedSelectionHost :modal="antigravityPlanOrder" title="套餐顺序" />

    <EndpointSelectionModal
      :open="geminiEndpointOpen"
      title="Gemini CLI 接口"
      :selected="geminiEndpointSelection"
      :is-selected="isGeminiEndpointSelected"
      :toggle="toggleGeminiEndpoint"
      @close="geminiEndpointOpen = false"
    />

    <EndpointSelectionModal
      :open="antigravityEndpointOpen"
      title="Antigravity API 端点"
      :options="ANTIGRAVITY_API_ENDPOINT_OPTIONS"
      :selected="antigravityEndpointSelection"
      :is-selected="isAntigravityEndpointSelected"
      :toggle="toggleAntigravityEndpoint"
      @close="antigravityEndpointOpen = false"
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
