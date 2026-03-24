<script setup lang="ts">
defineProps<{
  title: string
  description?: string
  icon?: string
}>()
</script>

<template>
  <VSheet
    rounded="xl"
    color="surface-container"
    class="pa-6 text-medium-emphasis"
  >
    <div class="empty-state-shell">
      <VAvatar size="60" color="primary-container" rounded="xl">
        <VIcon :icon="icon || 'mdi-compass-outline'" color="primary" size="24" />
      </VAvatar>
      <div class="empty-state-copy">
        <div class="text-h6 font-weight-bold" style="color: rgba(var(--v-theme-on-surface), 0.94)">{{ title }}</div>
        <p v-if="description" class="text-body-2">{{ description }}</p>
      </div>
      <div v-if="$slots.action">
        <slot name="action" />
      </div>
    </div>
  </VSheet>
</template>
