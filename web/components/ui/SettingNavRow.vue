<script setup lang="ts">
defineProps<{
  title: string
  description: string
}>()

const emit = defineEmits<{
  activate: []
}>()
</script>

<template>
  <div
    class="settings-item settings-item--toggle settings-nav-row"
    role="button"
    tabindex="0"
    @click="emit('activate')"
    @keyup.enter="emit('activate')"
    @keyup.space.prevent="emit('activate')"
  >
    <div class="settings-item-copy">
      <div class="settings-item-title">{{ title }}</div>
      <div class="settings-item-description text-medium-emphasis">{{ description }}</div>
    </div>
    <VIcon icon="mdi-chevron-right" />
  </div>
</template>

<style scoped>
.settings-nav-row {
  border-radius: var(--admin-radius-panel);
  cursor: pointer;
}

.settings-nav-row:focus-visible {
  outline: 2px solid rgba(var(--v-theme-primary), 0.44);
  outline-offset: 2px;
}
</style>
