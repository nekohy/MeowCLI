<script setup lang="ts">
import type { UiTone } from '~/types/admin'

/** QuotaCard 的网格容器:默认插槽可放分支特有的前置卡(如 Credits/重置次数) */
defineProps<{
  cards: Array<{
    label: string
    value?: string
    tone?: UiTone
    percent?: number | null
    score?: string
    caption?: string[]
  }>
}>()
</script>

<template>
  <div class="quota-grid">
    <slot />
    <QuotaCard
      v-for="quota in cards"
      :key="quota.label"
      :label="quota.label"
      :value="quota.value"
      :tone="quota.tone"
      :percent="quota.percent"
      :score="quota.score"
      :caption="quota.caption"
    />
  </div>
</template>
