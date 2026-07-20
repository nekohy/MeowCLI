<script setup lang="ts">
import { colorForTone } from '~/lib/admin'
import type { UiTone } from '~/types/admin'

const props = withDefaults(defineProps<{
  label: string
  value?: string
  tone?: UiTone
  percent?: number | null
  score?: string
  caption?: string[]
  clickable?: boolean
}>(), {
  value: '',
  tone: 'secondary',
  percent: null,
  score: '',
  caption: () => [],
  clickable: false,
})

const emit = defineEmits<{
  activate: []
}>()

// UiTone 是全站词汇,渲染前经 colorForTone 转成 Vuetify 色名
// (text-danger/text-accent 在 Vuetify 中并不存在)
const toneColor = computed(() => colorForTone(props.tone))

function handleActivate() {
  if (props.clickable) {
    emit('activate')
  }
}
</script>

<template>
  <div
    class="quota-card"
    :class="{ 'quota-card--clickable': clickable }"
    :role="clickable ? 'button' : undefined"
    :tabindex="clickable ? 0 : undefined"
    @click="handleActivate"
    @keydown.enter="handleActivate"
    @keydown.space.prevent="handleActivate"
  >
    <div class="quota-row">
      <div class="quota-label text-medium-emphasis">{{ label }}</div>
      <span class="quota-value">{{ value }}</span>
    </div>
    <VProgressLinear
      v-if="percent !== null"
      :model-value="percent"
      :color="toneColor"
      rounded
      height="10"
    />
    <div v-if="caption.length || score" class="quota-footer text-medium-emphasis">
      <div v-if="caption.length" class="quota-caption">
        <span v-for="text in caption" :key="text">
          <span class="quota-caption-text">{{ text }}</span>
        </span>
      </div>
      <div v-if="score" class="quota-score">{{ score }}</div>
    </div>
  </div>
</template>
