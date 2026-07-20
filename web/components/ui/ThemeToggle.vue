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

// 三态图标轮换:front/back 双层交替承载新图标,
// 配合共享类 .icon-swap / .icon-swap--swapped 做旋转缩放交叉切换
const frontIcon = ref(currentIcon.value)
const backIcon = ref(currentIcon.value)
const swapped = ref(false)

watch(currentIcon, (next) => {
  if (swapped.value) {
    frontIcon.value = next
  } else {
    backIcon.value = next
  }
  swapped.value = !swapped.value
})

function cyclePreference() {
  emit('update:preference', nextPreference.value)
}
</script>

<template>
  <VBtn
    variant="text"
    color="primary"
    class="text-none theme-toggle-btn"
    size="default"
    @click="cyclePreference"
  >
    <template #prepend>
      <span class="icon-swap" :class="{ 'icon-swap--swapped': swapped }" aria-hidden="true">
        <VIcon class="icon-swap__front" :icon="frontIcon" />
        <VIcon class="icon-swap__back" :icon="backIcon" />
      </span>
    </template>
    {{ currentLabel }}
  </VBtn>
</template>
