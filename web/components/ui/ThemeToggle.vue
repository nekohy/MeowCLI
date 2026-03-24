<script setup lang="ts">
import type { ThemeMode } from '~/types/admin'

const props = withDefaults(defineProps<{
  theme?: ThemeMode
}>(), {
  theme: 'light',
})

defineEmits<{
  toggle: []
}>()

const nextLabel = computed(() => (
  props.theme === 'dark' ? '切换到浅色模式' : '切换到深色模式'
))

const currentIcon = computed(() => (
  props.theme === 'dark' ? 'mdi-white-balance-sunny' : 'mdi-weather-night'
))

const currentLabel = computed(() => (
  props.theme === 'dark' ? '深色' : '浅色'
))
</script>

<template>
  <VTooltip :text="nextLabel" location="bottom">
    <template #activator="{ props: tooltipProps }">
      <VBtn
        v-bind="tooltipProps"
        :prepend-icon="currentIcon"
        variant="text"
        color="secondary"
        class="text-none"
        size="default"
        @click="$emit('toggle')"
      >
        {{ currentLabel }}
      </VBtn>
    </template>
  </VTooltip>
</template>
