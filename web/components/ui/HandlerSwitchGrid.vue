<script setup lang="ts">
import type { HandlerOverview } from '~/types/admin'

withDefaults(defineProps<{
  handlers: HandlerOverview[]
  // 切换场景（凭据页）传入当前选中 key 以高亮；展示场景（仪表盘）不传则不显示选中态
  selected?: string
}>(), {
  selected: '',
})

defineEmits<{
  select: [key: string]
}>()
</script>

<template>
  <div class="handler-switch-grid">
    <VCard
      v-for="handler in handlers"
      :key="handler.key"
      class="interactive-card handler-card surface-card"
      :class="{ 'is-active': selected !== '' && selected === handler.key }"
      color="surface"
      variant="flat"
      role="button"
      tabindex="0"
      @click="$emit('select', handler.key)"
      @keyup.enter="$emit('select', handler.key)"
      @keyup.space.prevent="$emit('select', handler.key)"
    >
      <VCardText class="handler-card-shell">
        <div class="handler-card-top">
          <div class="handler-card-copy">
            <div class="handler-card-title">{{ handler.label }}</div>
          </div>
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
