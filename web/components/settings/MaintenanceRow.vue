<script setup lang="ts">
withDefaults(defineProps<{
  label: string
  description: string
  actionText: string
  icon: string
  variant?: 'secondary' | 'danger'
  loading?: boolean
  disabled?: boolean
}>(), {
  variant: 'secondary',
  loading: false,
  disabled: false,
})

const emit = defineEmits<{
  activate: []
}>()
</script>

<template>
  <div class="settings-item settings-item--toggle">
    <div class="settings-item-copy">
      <div class="settings-item-title">{{ label }}</div>
      <div class="settings-item-description text-medium-emphasis">{{ description }}</div>
    </div>
    <div class="settings-maintenance-actions">
      <AdminButton
        :variant="variant"
        size="sm"
        :prepend-icon="icon"
        :loading="loading"
        :disabled="disabled"
        @click="emit('activate')"
      >
        {{ actionText }}
      </AdminButton>
    </div>
  </div>
</template>

<style scoped>
.settings-maintenance-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  justify-content: flex-end;
}

@media (max-width: 959px) {
  .settings-maintenance-actions {
    justify-content: stretch;
  }

  .settings-maintenance-actions > * {
    flex: 1 1 180px;
  }
}
</style>
