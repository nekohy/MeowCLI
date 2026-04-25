<script setup lang="ts">
import { formatTime, statusText, toneForStatus } from '~/lib/admin'
import type { LogItem } from '~/types/admin'

const admin = useAdminApp()
const router = useRouter()

const summary = computed(() => admin.overview.value.summary)
const recentLogs = computed(() => admin.overview.value.recent_logs)

function hasLogDetail(text: string) {
  return text
    .split(/\r?\n/)
    .map((item) => item.trim())
    .some(Boolean)
}

function logItemKey(item: LogItem) {
  return `${item.handler}-${item.credential_id}-${item.created_at}-${item.status_code}`
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
      title="运行概览"
      icon="mdi-view-dashboard"
    />

    <div class="dashboard-action-grid">
      <VCard
        class="interactive-card dashboard-action-card"
        color="surface-container"
        variant="flat"
        border
        role="button"
        tabindex="0"
        @click="openPage('/models')"
        @keyup.enter="openPage('/models')"
      >
        <VCardText class="dashboard-action-shell">
          <div class="dashboard-action-copy">
            <div class="dashboard-action-label">映射规则</div>
            <div class="dashboard-action-value">{{ summary.models_total }}</div>
            <div class="dashboard-action-helper text-medium-emphasis">管理模型别名、上游模型和处理器绑定</div>
          </div>
          <VAvatar size="54" color="primary-container" rounded="xl">
            <VIcon icon="mdi-vector-link" color="primary" size="26" />
          </VAvatar>
        </VCardText>
      </VCard>

      <VCard
        class="interactive-card dashboard-action-card"
        color="surface-container"
        variant="flat"
        border
        role="button"
        tabindex="0"
        @click="openPage('/keys')"
        @keyup.enter="openPage('/keys')"
      >
        <VCardText class="dashboard-action-shell">
          <div class="dashboard-action-copy">
            <div class="dashboard-action-label">访问密钥</div>
            <div class="dashboard-action-value">{{ summary.auth_keys_total }}</div>
            <div class="dashboard-action-helper text-medium-emphasis">维护后台和 API 共用的访问凭证</div>
          </div>
          <VAvatar size="54" color="secondary-container" rounded="xl">
            <VIcon icon="mdi-shield-lock" color="secondary" size="26" />
          </VAvatar>
        </VCardText>
      </VCard>
    </div>

    <SectionCard
      title="后端服务"
      icon="mdi-cpu-64-bit"
    >
      <div class="dashboard-handler-grid">
        <VCard
          v-for="handler in admin.handlers.value"
          :key="handler.key"
          class="interactive-card dashboard-handler-card"
          color="surface-container"
          variant="flat"
          border
          role="button"
          tabindex="0"
          @click="openHandler(handler.key, handler.supports_credentials)"
          @keyup.enter="openHandler(handler.key, handler.supports_credentials)"
        >
          <VCardText class="dashboard-handler-shell">
            <div class="dashboard-handler-main">
              <div class="dashboard-handler-title">{{ handler.label }}</div>
              <AdminBadge :tone="toneForStatus(handler.status)">
                {{ statusText(handler.status) }}
              </AdminBadge>
            </div>
            <div class="dashboard-handler-stats">
              <div>
                <div class="dashboard-handler-stat-label">凭据</div>
                <div class="dashboard-handler-stat-value">{{ handler.credentials_total || 0 }}</div>
              </div>
              <div>
                <div class="dashboard-handler-stat-label">可用</div>
                <div class="dashboard-handler-stat-value">{{ handler.credentials_enabled || 0 }}</div>
              </div>
            </div>
          </VCardText>
        </VCard>
      </div>
    </SectionCard>

    <SectionCard
      title="最近请求"
      icon="mdi-pulse"
    >
      <div
        v-if="recentLogs.length"
        class="log-list"
      >
        <template
          v-for="item in recentLogs"
          :key="logItemKey(item)"
        >
          <VExpansionPanels
            v-if="hasLogDetail(item.text)"
            variant="accordion"
            class="log-panels"
          >
            <VExpansionPanel
              elevation="0"
              border
            >
              <VExpansionPanelTitle class="py-3">
                <div class="activity-title w-100">
                  <div class="d-flex align-center ga-3">
                    <AdminBadge :tone="item.status_code < 400 ? 'success' : 'danger'">
                      {{ item.status_code }}
                    </AdminBadge>
                    <span class="text-subtitle-2 font-weight-bold">{{ admin.handlerLookup.value.get(item.handler)?.label || item.handler }}</span>
                    <span class="text-caption text-medium-emphasis d-none d-sm-inline">{{ item.credential_id || 'SYSTEM' }}</span>
                    <VSpacer />
                    <span class="text-caption text-medium-emphasis">{{ formatTime(item.created_at) }}</span>
                  </div>
                </div>
              </VExpansionPanelTitle>
              <VExpansionPanelText>
                <VSheet color="surface-container-high" rounded="lg" class="log-detail-surface">
                  <pre class="log-text">{{ item.text }}</pre>
                </VSheet>
              </VExpansionPanelText>
            </VExpansionPanel>
          </VExpansionPanels>

          <VSheet
            v-else
            class="log-static-row"
            color="surface-container"
            rounded="xl"
            border
          >
            <div class="activity-title w-100">
              <div class="d-flex align-center ga-3">
                <AdminBadge :tone="item.status_code < 400 ? 'success' : 'danger'">
                  {{ item.status_code }}
                </AdminBadge>
                <span class="text-subtitle-2 font-weight-bold">{{ admin.handlerLookup.value.get(item.handler)?.label || item.handler }}</span>
                <span class="text-caption text-medium-emphasis d-none d-sm-inline">{{ item.credential_id || 'SYSTEM' }}</span>
                <VSpacer />
                <span class="text-caption text-medium-emphasis">{{ formatTime(item.created_at) }}</span>
              </div>
            </div>
          </VSheet>
        </template>
      </div>

      <EmptyState
        v-else
        title="暂无请求日志"
        description="收到请求后，日志会显示在这里"
        icon="mdi-file-document-outline"
      />
    </SectionCard>
  </div>
</template>
