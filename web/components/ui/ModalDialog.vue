<script setup lang="ts">
const props = withDefaults(defineProps<{
  open: boolean
  title: string
  description?: string
  icon?: string
  maxWidth?: number | string
  surface?: 'default' | 'secondary'
}>(), {
  description: undefined,
  icon: 'mdi-information-outline',
  maxWidth: 560,
  surface: 'default',
})

defineEmits<{
  close: []
}>()
</script>

<template>
  <VDialog
    :model-value="open"
    :max-width="maxWidth"
    scrollable
    @update:model-value="(value) => !value && $emit('close')"
  >
    <VCard
      :color="surface === 'secondary' ? 'surface-container-highest' : 'surface-container-high'"
      class="modal-card"
    >
      <VCardItem class="pa-5 pb-3">
        <VCardTitle class="text-h6 font-weight-bold modal-title">
          <VIcon :icon="icon" color="primary" size="20" />
          <span>{{ title }}</span>
        </VCardTitle>
        <VCardSubtitle v-if="description" class="text-wrap mt-1">{{ description }}</VCardSubtitle>
        <template #append>
          <VBtn
            icon="mdi-close"
            variant="text"
            aria-label="关闭对话框"
            @click="$emit('close')"
          />
        </template>
      </VCardItem>
      <VCardText class="px-5 pt-0">
        <slot />
      </VCardText>
      <VCardActions v-if="$slots.footer" class="justify-end flex-wrap ga-2 px-5 pb-5">
        <slot name="footer" />
      </VCardActions>
    </VCard>
  </VDialog>
</template>
