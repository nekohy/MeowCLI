<script setup lang="ts">
import { statusText, toneForStatus } from '~/lib/admin'
import type { HandlerOverview } from '~/types/admin'

defineProps<{
  handlers: HandlerOverview[]
  selected: string
}>()

defineEmits<{
  select: [key: string]
}>()
</script>

<template>
  <div class="handler-switch-grid">
    <VCard
      v-for="handler in handlers"
      :key="handler.key"
      class="interactive-card handler-card"
      :class="{ 'is-active': selected === handler.key }"
      color="surface-container"
      variant="flat"
      role="button"
      tabindex="0"
      @click="$emit('select', handler.key)"
      @keyup.enter="$emit('select', handler.key)"
    >
      <VCardText class="handler-card-shell">
        <div class="handler-card-top">
          <div class="handler-card-copy">
            <div class="handler-card-title">{{ handler.label }}</div>
          </div>
          <AdminBadge :tone="toneForStatus(handler.status)">
            {{ statusText(handler.status) }}
          </AdminBadge>
        </div>

        <div class="handler-card-stats">
          <div class="handler-card-stat">
            <div class="text-body-2 text-medium-emphasis">凭据</div>
            <div class="handler-card-stat-value">{{ handler.credentials_total || 0 }}</div>
          </div>
          <div class="handler-card-stat">
            <div class="text-body-2 text-medium-emphasis">可用</div>
            <div class="handler-card-stat-value">{{ handler.credentials_enabled || 0 }}</div>
          </div>
        </div>
      </VCardText>
    </VCard>
  </div>
</template>
