<script setup lang="ts">
withDefaults(defineProps<{
  total: number
  page: number
  maxPage: number
  totalVisible?: number
}>(), {
  totalVisible: 7,
})

const emit = defineEmits<{
  change: [page: number]
}>()
</script>

<template>
  <div class="pagination-bar">
    <div class="text-body-2 text-medium-emphasis">
      共 {{ total }} 条，当前第 {{ page }} / {{ maxPage }} 页
    </div>
    <VPagination
      :model-value="page"
      :length="maxPage"
      density="comfortable"
      :total-visible="totalVisible"
      @update:model-value="(value) => emit('change', Number(value))"
    />
  </div>
</template>
