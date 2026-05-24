<script setup lang="ts">
import type { ThemeMode, ThemePreference } from '~/types/admin'

const props = defineProps<{
  theme: ThemeMode
  preference: ThemePreference
}>()

const emit = defineEmits<{
  'update:preference': [preference: ThemePreference]
}>()

const nextPreference = computed<ThemePreference>(() => {
  if (props.preference === 'system') {
    return 'light'
  }

  return props.preference === 'light' ? 'dark' : 'system'
})

const currentIcon = computed(() => (
  props.preference === 'system'
    ? 'mdi-theme-light-dark'
    : props.preference === 'dark' ? 'mdi-weather-night' : 'mdi-white-balance-sunny'
))

const currentLabel = computed(() => (
  props.preference === 'system'
    ? '自动'
    : props.preference === 'dark' ? '深色' : '浅色'
))

function cyclePreference() {
  emit('update:preference', nextPreference.value)
}
</script>

<template>
  <VBtn
    :prepend-icon="currentIcon"
    variant="text"
    color="primary"
    class="text-none"
    size="default"
    @click="cyclePreference"
  >
    {{ currentLabel }}
  </VBtn>
</template>
