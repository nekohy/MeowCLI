<script setup lang="ts">
defineProps<{
  eyebrow?: string
  title: string
  icon?: string
}>()
</script>

<template>
  <VCard color="surface-container" variant="flat" class="page-header-card">
    <VCardText class="page-header-shell">
      <div class="page-header-copy">
        <VAvatar size="52" color="primary-container" rounded="xl" class="page-header-avatar">
          <VIcon :icon="icon || 'mdi-view-dashboard-outline'" color="primary" size="24" />
        </VAvatar>
        <div class="page-header-text">
          <div v-if="eyebrow" class="page-header-eyebrow">{{ eyebrow }}</div>
          <h1 class="page-header-title">{{ title }}</h1>
          <div v-if="$slots.meta" class="page-header-meta">
            <slot name="meta" />
          </div>
        </div>
      </div>
      <div v-if="$slots.actions" class="page-header-actions">
        <slot name="actions" />
      </div>
    </VCardText>
  </VCard>
</template>
