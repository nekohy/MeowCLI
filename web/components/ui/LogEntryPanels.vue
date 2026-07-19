<script setup lang="ts">
import { hasLogError, logItemKey, logMetaItems } from '~/lib/logs'
import type { LogItem } from '~/types/admin'

/**
 * 日志展开面板:总览页“最近请求”与日志页列表共用,
 * 差异仅在于凭据缺失时的占位文案。
 */
withDefaults(defineProps<{
  items: LogItem[]
  missingCredentialLabel?: string
}>(), {
  missingCredentialLabel: undefined,
})

const admin = useAdminApp()
</script>

<template>
  <VExpansionPanels
    multiple
    variant="accordion"
    class="log-panels log-list"
  >
    <VExpansionPanel
      v-for="item in items"
      :key="logItemKey(item)"
      elevation="0"
      border
    >
      <VExpansionPanelTitle class="py-3">
        <div class="activity-title">
          <div class="activity-topline">
            <span
              class="log-status-pill"
              :class="item.status_code < 400 ? 'log-status-pill--success' : 'log-status-pill--error'"
            >
              <span class="log-status-dot" />
              {{ item.status_code }}
            </span>
            <span class="text-subtitle-2 font-weight-medium">
              {{ admin.handlerLookup.value.get(item.handler)?.label || item.handler }}
            </span>
          </div>
        </div>
      </VExpansionPanelTitle>
      <VExpansionPanelText>
        <div class="log-detail-stack">
          <div class="log-meta-panel">
            <div
              v-for="meta in logMetaItems(item, missingCredentialLabel)"
              :key="meta.label"
              class="log-meta-item"
              :class="{ 'log-meta-item--wide': meta.wide }"
            >
              <span>{{ meta.label }}</span>
              <strong>{{ meta.value }}</strong>
            </div>
          </div>
          <div v-if="hasLogError(item.error)" class="log-detail-surface">
            <div class="log-detail-heading">
              <span>错误响应</span>
              <span>JSON</span>
            </div>
            <pre class="log-text">{{ item.error }}</pre>
          </div>
        </div>
      </VExpansionPanelText>
    </VExpansionPanel>
  </VExpansionPanels>
</template>
