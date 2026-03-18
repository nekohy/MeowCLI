<script setup lang="ts">
import { apiTypesText, formatTime, statusText, toneForStatus } from '~/lib/admin'

const admin = useAdminApp()
const router = useRouter()

const summary = computed(() => admin.overview.value.summary)
const recentLogs = computed(() => admin.overview.value.recent_logs)

async function openHandler(key: string, supportsCredentials: boolean) {
  admin.selectedHandler.value = key
  await router.push(supportsCredentials ? '/credentials' : '/models')
}
</script>

<template>
  <div class="page-grid">
    <PageHeader
      eyebrow="运行概览"
      title="控制台状态"
      description="集中查看凭据、模型映射、访问密钥与最近请求。"
    >
      <template #actions>
        <AdminButton variant="secondary" :disabled="admin.booting.value" @click="admin.loadOverview(admin.token.value)">
          {{ admin.booting.value ? '刷新中...' : '刷新总览' }}
        </AdminButton>
      </template>
    </PageHeader>

    <section class="metric-grid">
      <MetricCard label="启用凭据" :value="summary.credentials_enabled" helper="当前可参与调度的账户" />
      <MetricCard label="全部凭据" :value="summary.credentials_total" helper="包含已停用和待同步账户" />
      <MetricCard label="模型映射" :value="summary.models_total" helper="对外别名总数" />
      <MetricCard label="访问密钥" :value="summary.auth_keys_total" helper="后台与 API 使用同一套密钥体系" />
    </section>

    <SectionCard title="处理器能力" eyebrow="路由与调度">
      <div class="card-grid">
        <button
          v-for="handler in admin.handlers.value"
          :key="handler.key"
          type="button"
          class="capability-card"
          @click="openHandler(handler.key, handler.supports_credentials)"
        >
          <div class="capability-top">
            <div>
              <div class="capability-title">{{ handler.label }}</div>
              <div class="capability-summary">{{ handler.summary || '暂无说明' }}</div>
            </div>
            <AdminBadge :tone="toneForStatus(handler.status)">
              {{ statusText(handler.status) }}
            </AdminBadge>
          </div>
          <div class="capability-meta">
            <span>接口 {{ apiTypesText(handler.supported_api_types) }}</span>
            <span>{{ handler.models_total || 0 }} 个模型</span>
            <span>{{ handler.credentials_total || 0 }} 个凭据</span>
          </div>
        </button>
      </div>
    </SectionCard>

    <SectionCard title="最近请求" eyebrow="诊断">
      <div v-if="recentLogs.length" class="log-list">
        <article
          v-for="item in recentLogs"
          :key="`${item.handler}-${item.credential_id}-${item.created_at}-${item.status_code}`"
          class="log-item"
        >
          <div class="log-meta">
            <AdminBadge :tone="item.status_code < 400 ? 'success' : 'danger'">
              {{ item.status_code }}
            </AdminBadge>
            <span>{{ admin.handlerLookup.value.get(item.handler)?.label || item.handler }}</span>
            <span>{{ item.credential_id || '未记录凭据' }}</span>
            <span>{{ formatTime(item.created_at) }}</span>
          </div>
          <div class="log-text">{{ item.text }}</div>
        </article>
      </div>
      <EmptyState
        v-else
        title="暂无请求日志"
        description="这里会显示最近的请求记录"
      />
    </SectionCard>
  </div>
</template>
