<script setup lang="ts">
import { colorForTone } from '~/lib/admin'
import type { UiTone } from '~/types/admin'

const props = withDefaults(defineProps<{
  tone?: UiTone
  subtle?: boolean
  icon?: string
}>(), {
  tone: 'neutral',
  subtle: false,
  icon: undefined,
})

const chipColor = computed(() => colorForTone(props.tone))
const chipVariant = computed(() => (props.subtle ? 'text' : 'tonal'))
</script>

<template>
  <VChip
    :color="chipColor"
    :variant="chipVariant"
    :prepend-icon="icon"
    size="small"
    class="font-weight-bold"
  >
    <slot />
  </VChip>
</template>
