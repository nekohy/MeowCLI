<script setup lang="ts">
defineProps<{
  title?: string
  eyebrow?: string
  icon?: string
  description?: string
}>()
</script>

<template>
  <VCard color="surface-container" variant="flat">
    <VCardItem v-if="title || eyebrow || description || $slots.actions" class="section-card-head">
      <div class="section-card-leading">
        <VAvatar v-if="icon" size="40" color="secondary-container" rounded="lg">
          <VIcon :icon="icon" color="secondary" size="18" />
        </VAvatar>
        <div class="section-card-copy">
          <VCardSubtitle v-if="eyebrow" class="section-card-eyebrow">{{ eyebrow }}</VCardSubtitle>
          <VCardTitle v-if="title" class="section-card-title">{{ title }}</VCardTitle>
          <div v-if="description" class="section-card-description text-medium-emphasis">{{ description }}</div>
        </div>
      </div>
      <template #append>
        <div v-if="$slots.actions">
          <slot name="actions" />
        </div>
      </template>
    </VCardItem>
    <VCardText class="section-card-body" :class="{ 'pt-0': title || eyebrow || description || $slots.actions }">
      <slot />
    </VCardText>
  </VCard>
</template>
