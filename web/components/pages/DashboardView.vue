<script setup lang="ts">
const admin = useAdminApp()
const router = useRouter()
const refreshing = ref(false)
const autoRefreshIntervalMs = 5000
let refreshTimer: number | undefined

const summary = computed(() => admin.overview.value.summary)
const recentLogs = computed(() => admin.overview.value.recent_logs)

async function refreshOverview() {
  if (!admin.authReady.value || refreshing.value) {
    return
  }
  refreshing.value = true
  try {
    await admin.loadOverview(admin.token.value, true)
  } finally {
    refreshing.value = false
  }
}

onMounted(() => {
  if (!import.meta.client) {
    return
  }
  void refreshOverview()
  refreshTimer = window.setInterval(() => {
    if (document.visibilityState === 'visible') {
      void refreshOverview()
    }
  }, autoRefreshIntervalMs)
})

onBeforeUnmount(() => {
  if (refreshTimer) {
    window.clearInterval(refreshTimer)
  }
})

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
      icon="mdi-view-dashboard-outline"
    >
      <template #actions>
        <AdminButton
          variant="secondary"
          @click="refreshOverview"
        >
          <template #prepend>
            <VIcon icon="mdi-refresh" :class="{ 'is-spinning': refreshing }" />
          </template>
          刷新
        </AdminButton>
      </template>
    </PageHeader>

    <div class="dashboard-action-grid">
      <VCard
        class="interactive-card dashboard-action-card surface-card"
        color="surface"
        variant="flat"
        role="button"
        tabindex="0"
        @click="openPage('/models')"
        @keyup.enter="openPage('/models')"
        @keyup.space.prevent="openPage('/models')"
      >
        <VCardText class="dashboard-action-shell">
          <div class="dashboard-action-copy">
            <div class="dashboard-action-label">映射规则</div>
            <div class="dashboard-action-value">{{ summary.models_total }}</div>
            <div class="dashboard-action-helper text-medium-emphasis">管理模型别名、上游模型和处理器绑定</div>
          </div>
          <VAvatar size="54" color="primary-container" rounded="xl" class="dashboard-action-avatar">
            <VIcon icon="mdi-vector-link" color="primary" size="26" />
          </VAvatar>
        </VCardText>
      </VCard>

      <VCard
        class="interactive-card dashboard-action-card surface-card"
        color="surface"
        variant="flat"
        role="button"
        tabindex="0"
        @click="openPage('/keys')"
        @keyup.enter="openPage('/keys')"
        @keyup.space.prevent="openPage('/keys')"
      >
        <VCardText class="dashboard-action-shell">
          <div class="dashboard-action-copy">
            <div class="dashboard-action-label">访问密钥</div>
            <div class="dashboard-action-value">{{ summary.auth_keys_total }}</div>
            <div class="dashboard-action-helper text-medium-emphasis">维护后台和 API 共用的访问凭证</div>
          </div>
          <VAvatar size="54" color="primary-container" rounded="xl" class="dashboard-action-avatar">
            <VIcon icon="mdi-shield-lock-outline" color="primary" size="26" />
          </VAvatar>
        </VCardText>
      </VCard>
    </div>

    <SectionCard
      title="后端服务"
      icon="mdi-server-network-outline"
    >
      <HandlerSwitchGrid
        :handlers="admin.handlers.value"
        @select="(key) => openHandler(key, admin.handlerLookup.value.get(key)?.supports_credentials ?? false)"
      />
    </SectionCard>

    <SectionCard
      title="最近请求"
      icon="mdi-pulse"
    >
      <LogEntryPanels
        v-if="recentLogs.length"
        :items="recentLogs"
        missing-credential-label="SYSTEM"
      />

      <EmptyState
        v-else
        title="暂无请求日志"
        description="收到请求后，日志会显示在这里"
        icon="mdi-file-document-outline"
      />
    </SectionCard>
  </div>
</template>
