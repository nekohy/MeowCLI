<script setup lang="ts">
/**
 * useOrderedSelectionModal 的共享宿主:把 composable 返回值透传给 PlanOrderModal,
 * 调用方一行接入,不再逐字段写 11 个绑定。
 * 注意:modal 是普通对象,模板中需显式 .value 解包其中的 ref。
 */
withDefaults(defineProps<{
  modal: ReturnType<typeof useOrderedSelectionModal>
  title: string
  description?: string
  icon?: string
  maxWidth?: string | number
  emptyText?: string
  itemLabel?: (item: string) => string
  itemDescription?: (item: string) => string
}>(), {
  description: undefined,
  icon: undefined,
  maxWidth: undefined,
  emptyText: undefined,
  itemLabel: undefined,
  itemDescription: undefined,
})
</script>

<template>
  <PlanOrderModal
    :open="modal.open.value"
    :title="title"
    :description="description"
    :icon="icon"
    :max-width="maxWidth"
    :empty-text="emptyText"
    :draft="modal.draft.value"
    :drag-idx="modal.dragIdx.value"
    :is-selected="modal.isSelected"
    :rank-of="modal.rankOf"
    :toggle="modal.toggle"
    :on-drag-start="modal.onDragStart"
    :on-drag-over="modal.onDragOver"
    :on-drag-end="modal.onDragEnd"
    :item-label="itemLabel"
    :item-description="itemDescription"
    @close="modal.closeModal()"
  />
</template>
