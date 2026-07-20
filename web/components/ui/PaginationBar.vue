<script setup lang="ts">
const props = withDefaults(defineProps<{
  total: number
  page: number
  maxPage: number
  totalVisible?: number
  showSummary?: boolean
  density?: 'default' | 'comfortable' | 'compact'
}>(), {
  totalVisible: 7,
  showSummary: true,
  density: 'comfortable',
})

const emit = defineEmits<{
  change: [page: number]
}>()
</script>

<template>
  <div class="pagination-bar" :class="`pagination-bar--density-${props.density}`">
    <div class="pagination-bar__leading">
      <slot name="leading" />
      <div v-if="showSummary" class="text-body-2 text-medium-emphasis">
        共 {{ total }} 条，当前第 {{ page }} / {{ maxPage }} 页
      </div>
    </div>
    <VPagination
      :model-value="page"
      :length="maxPage"
      :density="props.density"
      :total-visible="totalVisible"
      @update:model-value="(value) => emit('change', Number(value))"
    />
  </div>
</template>
