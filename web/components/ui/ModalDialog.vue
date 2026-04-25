<script setup lang="ts">
const props = withDefaults(defineProps<{
  open: boolean
  title: string
  description?: string
  icon?: string
  maxWidth?: number | string
}>(), {
  description: undefined,
  icon: 'mdi-information-outline',
  maxWidth: 560,
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
    <VCard color="surface-container-high" rounded="xl" class="modal-card">
      <VCardItem class="pa-5">
        <template #prepend>
          <VAvatar size="44" color="primary-container" rounded="xl">
            <VIcon :icon="icon" color="primary" size="20" />
          </VAvatar>
        </template>
        <VCardTitle class="text-h6 font-weight-bold">{{ title }}</VCardTitle>
        <VCardSubtitle v-if="description" class="text-wrap mt-1">{{ description }}</VCardSubtitle>
        <template #append>
          <VBtn
            icon="mdi-close"
            variant="text"
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
