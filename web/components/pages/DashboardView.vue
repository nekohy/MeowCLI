<script setup lang="ts">
import { formatTime, statusText, toneForStatus } from '~/lib/admin'

const admin = useAdminApp()
const router = useRouter()

const summary = computed(() => admin.overview.value.summary)
const recentLogs = computed(() => admin.overview.value.recent_logs)
const onlineHandlers = computed(() => (
  admin.handlers.value.filter((item) => item.status === 'enabled' || item.status === 'available').length
))

function previewLog(text: string) {
  const line = text
    .split(/\r?\n/)
    .map((item) => item.trim())
    .find(Boolean)

  return line ? line.slice(0, 120) : '无详细文本'
}

async function openHandler(key: string, supportsCredentials: boolean) {
  admin.selectedHandler.value = key
  await router.push(supportsCredentials ? '/credentials' : '/models')
}

async function openPage(path: string) {
  await router.push(path)
}
</script>

<template>
  <div class="page-grid">
    <PageHeader
      eyebrow="控制台"
      title="运行概览"
      icon="mdi-view-dashboard"
    >
      <template #meta>
        <AdminBadge tone="success" icon="mdi-check-circle">
          在线服务 {{ onlineHandlers }}
        </AdminBadge>
        <AdminBadge tone="secondary" icon="mdi-history">
          历史记录 {{ summary.logs_total || 0 }}
        </AdminBadge>
      </template>
    </PageHeader>

    <div class="summary-grid">
      <MetricCard
        label="在线处理器"
        :value="onlineHandlers"
        helper="活跃组件"
        icon="mdi-server"
      />
      <MetricCard
        label="就绪凭据"
        :value="summary.credentials_enabled"
        helper="令牌池状态"
        icon="mdi-shield-check"
      />
      <MetricCard
        label="总计凭据"
        :value="summary.credentials_total"
        helper="全部导入"
        icon="mdi-key-chain"
      />
      <MetricCard
        label="映射规则"
        :value="summary.models_total"
        helper="模型映射"
        icon="mdi-vector-link"
      />
      <MetricCard
        label="访问密钥"
        :value="summary.auth_keys_total"
        helper="权限控制"
        icon="mdi-shield-lock"
      />
    </div>

    <SectionCard
      title="服务处理器"
      eyebrow="组件"
      icon="mdi-cpu-64-bit"
    >
      <HandlerSwitchGrid
        :handlers="admin.handlers.value"
        :selected="admin.selectedHandler.value"
        @select="openHandler($event, admin.handlerLookup.value.get($event)?.supports_credentials ?? false)"
      />
    </SectionCard>

    <SectionCard
      title="实时诊断"
      eyebrow="日志"
      icon="mdi-pulse"
    >
      <VExpansionPanels
        v-if="recentLogs.length"
        variant="accordion"
        class="log-panels"
      >
        <VExpansionPanel
          v-for="item in recentLogs"
          :key="`${item.handler}-${item.credential_id}-${item.created_at}-${item.status_code}`"
          elevation="0"
          border
          class="mb-2"
        >
          <VExpansionPanelTitle class="py-3">
            <div class="activity-title w-100">
              <div class="d-flex align-center ga-3 mb-1">
                <AdminBadge :tone="item.status_code < 400 ? 'success' : 'danger'">
                  {{ item.status_code }}
                </AdminBadge>
                <span class="text-subtitle-2 font-weight-bold">{{ admin.handlerLookup.value.get(item.handler)?.label || item.handler }}</span>
                <span class="text-caption text-medium-emphasis d-none d-sm-inline">{{ item.credential_id || 'SYSTEM' }}</span>
                <VSpacer />
                <span class="text-caption text-medium-emphasis">{{ formatTime(item.created_at) }}</span>
              </div>
              <div class="text-caption text-medium-emphasis text-truncate" style="opacity: 0.7">{{ previewLog(item.text) }}</div>
            </div>
          </VExpansionPanelTitle>
          <VExpansionPanelText>
            <VSheet color="surface-container-high" rounded="lg" class="pa-3 mt-2">
              <pre class="log-text">{{ item.text }}</pre>
            </VSheet>
          </VExpansionPanelText>
        </VExpansionPanel>
      </VExpansionPanels>

      <EmptyState
        v-else
        title="暂无请求日志"
        description="请求到达后会自动出现在这里。"
        icon="mdi-file-document-outline"
      />
    </SectionCard>
  </div>
</template>
