<script setup lang="ts">
defineProps<{
  open: boolean
  title: string
  draft: string[]
  dragIdx: number | null
  isSelected: (planType: string) => boolean
  rankOf: (planType: string) => number
  toggle: (planType: string) => void
  onDragStart: (idx: number) => void
  onDragOver: (e: DragEvent, idx: number) => void
  onDragEnd: () => void
}>()

defineEmits<{
  close: []
}>()
</script>

<template>
  <ModalDialog
    :open="open"
    :title="title"
    description="拖动排序，勾选启用"
    icon="mdi-swap-vertical"
    :max-width="400"
    @close="$emit('close')"
  >
    <div class="plan-order-list">
      <div
        v-for="(planType, idx) in draft"
        :key="planType"
        class="plan-order-item"
        :class="{
          'plan-order-item--selected': isSelected(planType),
          'plan-order-item--dragging': dragIdx === idx,
        }"
        draggable="true"
        @dragstart="onDragStart(idx)"
        @dragover="(e) => onDragOver(e, idx)"
        @dragend="onDragEnd"
      >
        <VIcon icon="mdi-drag" size="18" class="plan-order-drag text-medium-emphasis" />
        <VCheckbox
          :model-value="isSelected(planType)"
          density="compact"
          hide-details
          @update:model-value="toggle(planType)"
          @click.stop
        />
        <span class="plan-order-label">{{ planType }}</span>
        <span v-if="isSelected(planType)" class="plan-order-rank text-medium-emphasis">
          #{{ rankOf(planType) }}
        </span>
      </div>
    </div>
    <div v-if="!draft.length" class="text-center text-medium-emphasis py-4">
      暂无可用套餐类型
    </div>
    <template #footer>
      <VBtn variant="text" @click="$emit('close')">关闭</VBtn>
    </template>
  </ModalDialog>
</template>

<style scoped>
.plan-order-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.plan-order-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 10px;
  border-radius: 10px;
  background: rgba(var(--v-theme-on-surface), 0.04);
  transition: background 0.15s, opacity 0.15s;
  user-select: none;
}

.plan-order-item:hover {
  background: rgba(var(--v-theme-on-surface), 0.08);
}

.plan-order-item--selected {
  background: rgba(var(--v-theme-primary), 0.08);
}

.plan-order-item--selected:hover {
  background: rgba(var(--v-theme-primary), 0.14);
}

.plan-order-item--dragging {
  opacity: 0.5;
}

.plan-order-drag {
  cursor: grab;
}

.plan-order-drag:active {
  cursor: grabbing;
}

.plan-order-label {
  flex: 1;
  font-weight: 600;
  font-size: 0.875rem;
}

.plan-order-rank {
  font-size: 0.75rem;
  font-weight: 700;
}
</style>
