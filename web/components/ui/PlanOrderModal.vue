<script setup lang="ts">
import { planTypeText } from '~/lib/admin'

defineProps<{
  open: boolean
  title: string
  description?: string
  icon?: string
  maxWidth?: string | number
  emptyText?: string
  draft: string[]
  dragIdx: number | null
  isSelected: (planType: string) => boolean
  rankOf: (planType: string) => number
  toggle: (planType: string) => void
  onDragStart: (idx: number) => void
  onDragOver: (e: DragEvent, idx: number) => void
  onDragEnd: () => void
  onMove: (idx: number, direction: -1 | 1) => void
  itemLabel?: (item: string) => string
  itemDescription?: (item: string) => string
}>()

defineEmits<{
  close: []
}>()
</script>

<template>
  <ModalDialog
    :open="open"
    :title="title"
    :description="description || '拖动排序，勾选启用'"
    :icon="icon || 'mdi-swap-vertical'"
    :max-width="maxWidth || 520"
    @close="$emit('close')"
  >
    <div class="plan-order-list">
      <div
        v-for="(item, idx) in draft"
        :key="item"
        class="plan-order-item"
        :class="{
          'plan-order-item--selected': isSelected(item),
          'plan-order-item--dragging': dragIdx === idx,
          'plan-order-item--with-description': Boolean(itemDescription?.(item)),
        }"
        draggable="true"
        @dragstart="onDragStart(idx)"
        @dragover="(e) => onDragOver(e, idx)"
        @dragend="onDragEnd"
      >
        <VIcon icon="mdi-drag" size="18" class="plan-order-drag text-medium-emphasis" aria-hidden="true" />
        <VCheckbox
          :model-value="isSelected(item)"
          class="plan-order-check"
          density="compact"
          hide-details
          :aria-label="itemLabel ? itemLabel(item) : planTypeText(item)"
          @update:model-value="toggle(item)"
          @click.stop
        />
        <span class="plan-order-label">
          <span>{{ itemLabel ? itemLabel(item) : planTypeText(item) }}</span>
          <small v-if="itemDescription?.(item)">{{ itemDescription(item) }}</small>
        </span>
        <span class="plan-order-side">
          <span v-if="isSelected(item)" class="plan-order-rank text-medium-emphasis">
            #{{ rankOf(item) }}
          </span>
          <span class="plan-order-move">
            <VBtn
              icon="mdi-arrow-up"
              size="x-small"
              variant="text"
              class="hit-target-48"
              :disabled="idx === 0"
              :aria-label="`上移 ${itemLabel ? itemLabel(item) : planTypeText(item)}`"
              @click.stop="onMove(idx, -1)"
            />
            <VBtn
              icon="mdi-arrow-down"
              size="x-small"
              variant="text"
              class="hit-target-48"
              :disabled="idx === draft.length - 1"
              :aria-label="`下移 ${itemLabel ? itemLabel(item) : planTypeText(item)}`"
              @click.stop="onMove(idx, 1)"
            />
          </span>
        </span>
      </div>
    </div>
    <div v-if="!draft.length" class="text-center text-medium-emphasis py-4">
      {{ emptyText || '暂无可用套餐类型' }}
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
  gap: 6px;
}

.plan-order-item {
  display: grid;
  grid-template-columns: 20px 28px minmax(0, 1fr) auto;
  align-items: center;
  column-gap: 10px;
  min-height: 48px;
  padding: 8px 12px;
  border-radius: var(--admin-radius-panel);
  background: rgba(var(--v-theme-on-surface), 0.04);
  transition: background 0.15s, opacity 0.15s;
  user-select: none;
}

.plan-order-item--with-description {
  align-items: center;
  min-height: 64px;
  padding-block: 10px;
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
  align-self: center;
  cursor: grab;
}

.plan-order-item--with-description .plan-order-drag {
  align-self: center;
  margin-top: 0;
}

.plan-order-drag:active {
  cursor: grabbing;
}

.plan-order-check {
  align-self: center;
  margin-inline-start: -2px;
}

.plan-order-item--with-description .plan-order-check {
  align-self: center;
  margin-top: 0;
}

.plan-order-check :deep(.v-selection-control) {
  min-height: 32px;
}

.plan-order-check :deep(.v-selection-control__wrapper) {
  width: 32px;
  height: 32px;
}

.plan-order-label {
  display: grid;
  gap: 2px;
  align-self: center;
  min-width: 0;
}

.plan-order-label > span {
  color: rgba(var(--v-theme-on-surface), 0.91);
  font-weight: 600;
  font-size: 0.875rem;
  line-height: 1.25;
  overflow-wrap: anywhere;
}

.plan-order-label > small {
  color: rgba(var(--v-theme-on-surface), 0.58);
  font-size: 0.75rem;
  font-weight: 400;
  line-height: 1.35;
  overflow-wrap: anywhere;
}

.plan-order-rank {
  align-self: center;
  font-size: 0.75rem;
  font-weight: 700;
}

.plan-order-side {
  display: flex;
  align-items: center;
  gap: 10px;
}

/* 上移/下移按钮:拖拽的键盘替代,间距 12px 避免外扩命中区相互重叠 */
.plan-order-move {
  display: flex;
  align-items: center;
  gap: 12px;
}

.plan-order-item--with-description .plan-order-rank {
  align-self: center;
  padding-top: 0;
}
</style>
