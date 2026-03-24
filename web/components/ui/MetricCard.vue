<script setup lang="ts">
defineProps<{
  label: string
  value: string | number
  helper?: string
  icon?: string
  color?: string
}>()
</script>

<template>
  <VCard color="surface-container" variant="tonal" border class="h-100">
    <VCardText class="metric-shell">
      <div class="metric-head">
        <div class="metric-copy">
          <div class="metric-label">{{ label }}</div>
          <div class="metric-value">{{ value }}</div>
        </div>
        <VAvatar size="40" :color="`${color || 'primary'}-container`" rounded="lg">
          <VIcon :icon="icon || 'mdi-chart-box-outline'" :color="color || 'primary'" size="20" />
        </VAvatar>
      </div>
      <div v-if="helper" class="metric-helper text-medium-emphasis">{{ helper }}</div>
    </VCardText>
  </VCard>
</template>
