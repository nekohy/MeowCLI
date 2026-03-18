<script setup lang="ts">
import { adminApi } from '~/composables/useAdminApi'
import {
  apiTypesText,
  DEFAULT_SETTINGS_FORM,
  joinPlanTypeInput,
  settingsToForm,
  settingsToPayload,
  splitPlanTypeInput,
  statusText,
  toneForStatus,
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
const selectedPlanTypes = computed(() => new Set(splitPlanTypeInput(form.value.codex_preferred_plan_types)))

function togglePreferredPlanType(planType: string) {
  const next = splitPlanTypeInput(form.value.codex_preferred_plan_types)
  const index = next.indexOf(planType)
  if (index >= 0) {
    next.splice(index, 1)
  } else {
    next.push(planType)
  }
  form.value.codex_preferred_plan_types = joinPlanTypeInput(next)
}

async function loadSettings() {
  if (!admin.token.value) {
    return
  }

  loading.value = true
  try {
    form.value = settingsToForm(await adminApi.getSettings(admin.token.value))
  } catch (error) {
    admin.notify(error instanceof Error ? error.message : '加载设置失败', 'danger')
  } finally {
    loading.value = false
  }
}

async function saveSettings() {
  actionBusy.value = true
  try {
    const result = await adminApi.updateSettings(admin.token.value, settingsToPayload(form.value))
    form.value = settingsToForm(result.settings || {})

    const deletedCount = result.deleted_free_accounts?.length || 0
    if (deletedCount > 0) {
      admin.notify(`设置已保存，并清理 ${deletedCount} 个 Free 凭据`, 'warning')
    } else {
      admin.notify('设置已保存')
    }

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
      eyebrow="运行参数"
      title="系统设置"
      description="区分系统级参数和处理器级参数；保存后会立即影响当前进程的转发与调度行为。"
    >
      <template #actions>
        <AdminButton variant="secondary" :disabled="loading" @click="loadSettings">
          {{ loading ? '加载中...' : '刷新设置' }}
        </AdminButton>
        <AdminButton :disabled="actionBusy" @click="saveSettings">
          {{ actionBusy ? '保存中...' : '保存设置' }}
        </AdminButton>
      </template>
    </PageHeader>

    <SectionCard title="系统级参数" eyebrow="全局">
      <div class="form-grid">
        <label class="form-field form-field-checkbox">
          <span>全局允许用户自定义 PlanType</span>
          <input v-model="form.allow_user_plan_type_header" type="checkbox">
          <small>总开关；关闭后所有处理器都会忽略 X-Meow-Plan-Type 请求头。</small>
        </label>
        <label class="form-field">
          <span>全局代理</span>
          <input v-model="form.global_proxy" type="url" placeholder="http://127.0.0.1:7890">
          <small>会影响所有上游 HTTP 请求。</small>
        </label>
        <label class="form-field">
          <span>最大重试次数</span>
          <input v-model="form.relay_max_retries" type="number" min="1">
          <small>转发失败后，最多切换凭据重试几次。</small>
        </label>
        <label class="form-field">
          <span>日志保留时间（秒）</span>
          <input v-model="form.logs_retention_seconds" type="number" min="1">
          <small>日志只保存在内存中，服务重启后会清空。</small>
        </label>
        <label class="form-field">
          <span>提前刷新时间（秒）</span>
          <input v-model="form.refresh_before_seconds" type="number" min="1">
          <small>在 Access Token 到期前多久主动刷新。</small>
        </label>
        <label class="form-field">
          <span>轮询间隔（毫秒）</span>
          <input v-model="form.poll_interval_milliseconds" type="number" min="1">
          <small>等待其他 goroutine 刷新 Token 时的轮询频率。</small>
        </label>
        <label class="form-field">
          <span>配额同步间隔（秒）</span>
          <input v-model="form.quota_sync_interval_seconds" type="number" min="1">
          <small>后台刷新配额数据的周期。</small>
        </label>
        <label class="form-field">
          <span>退避起始时间（秒）</span>
          <input v-model="form.throttle_base_seconds" type="number" min="1">
          <small>遇到 429 或上游错误时的首个退避时长。</small>
        </label>
        <label class="form-field">
          <span>退避上限（秒）</span>
          <input v-model="form.throttle_max_seconds" type="number" min="1">
          <small>指数退避的最大等待时间。</small>
        </label>
      </div>
    </SectionCard>

    <SectionCard title="处理器级参数" eyebrow="当前处理器">
      <div class="chip-row">
        <button
          v-for="handler in admin.handlers.value"
          :key="handler.key"
          type="button"
          :class="['chip', { 'is-active': admin.selectedHandler.value === handler.key }]"
          @click="admin.selectedHandler.value = handler.key"
        >
          {{ handler.label }}
        </button>
      </div>

      <div v-if="admin.activeHandler.value" class="handler-summary">
        <div>
          <h3>{{ admin.activeHandler.value.label }}</h3>
          <p>{{ admin.activeHandler.value.summary || '暂无说明' }}</p>
        </div>
        <div class="handler-summary-meta">
          <AdminBadge :tone="toneForStatus(admin.activeHandler.value.status)">
            {{ statusText(admin.activeHandler.value.status) }}
          </AdminBadge>
          <span>接口 {{ apiTypesText(admin.activeHandler.value.supported_api_types) }}</span>
          <span>{{ admin.activeHandler.value.credentials_total || 0 }} 个凭据</span>
          <span>{{ admin.activeHandler.value.models_total || 0 }} 个模型</span>
        </div>
      </div>

      <template v-if="admin.activeHandler.value?.key === 'codex'">
        <p class="table-subtitle">代理、套餐优先级与账号清理</p>
        <div class="form-grid">
          <label class="form-field">
            <span>Codex 代理</span>
            <input v-model="form.codex_proxy" type="url" placeholder="http://127.0.0.1:7890">
            <small>只影响 Codex CLI 相关请求；留空时继承全局代理。</small>
          </label>
          <label class="form-field">
            <span>内置 PlanType 优先级</span>
            <input v-model="form.codex_preferred_plan_types" type="text" placeholder="free,plus,team,pro">
            <div v-if="availablePlanTypes.length" class="chip-row">
              <button
                v-for="planType in availablePlanTypes"
                :key="planType"
                type="button"
                :class="['chip', { 'is-active': selectedPlanTypes.has(planType) }]"
                @click="togglePreferredPlanType(planType)"
              >
                {{ planType }}
              </button>
            </div>
            <small>使用英文逗号分隔；保存时后端会自动规范化为小写去重列表。</small>
          </label>
          <label class="form-field form-field-checkbox">
            <span>允许用户自定义 PlanType</span>
            <input v-model="form.codex_allow_user_plan_type_header" type="checkbox">
            <small>当前处理器的细粒度开关；还需要上方全局总开关同时开启才会生效。</small>
          </label>
          <label class="form-field form-field-checkbox">
            <span>自动删除 Free 凭据</span>
            <input v-model="form.codex_delete_free_accounts" type="checkbox">
            <small>保存设置时，会清理当前所有 Free 套餐账号。</small>
          </label>
        </div>
      </template>

      <EmptyState
        v-else
        :title="`${admin.activeHandler.value?.label || '当前处理器'} 暂无独立设置`"
        description="当前只有 Codex CLI 还保留少量处理器级设置；其他刷新与退避参数已经统一提升为全局配置。"
      />
    </SectionCard>
  </div>
</template>
