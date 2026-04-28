<script setup lang="ts">
import { GEMINI_BASE_URL_OPTIONS } from '~/lib/admin'

defineProps<{
  open: boolean
  selected: string[]
  isSelected: (value: string) => boolean
  toggle: (value: string) => void
}>()

defineEmits<{
  close: []
}>()
</script>

<template>
  <ModalDialog
    :open="open"
    title="Gemini CLI 接口"
    description="勾选启用，每次请求随机选择"
    icon="mdi-swap-vertical"
    :max-width="400"
    @close="$emit('close')"
  >
    <div class="endpoint-list">
      <div
        v-for="option in GEMINI_BASE_URL_OPTIONS"
        :key="option.value"
        class="endpoint-item"
        :class="{ 'endpoint-item--selected': isSelected(option.value) }"
        @click="toggle(option.value)"
      >
        <VCheckbox
          :model-value="isSelected(option.value)"
          density="compact"
          hide-details
          @update:model-value="toggle(option.value)"
          @click.stop
        />
        <span class="endpoint-label">{{ option.title }}</span>
      </div>
    </div>
    <template #footer>
      <div class="endpoint-summary text-medium-emphasis">
        已启用 {{ selected.length }} 个
      </div>
      <VBtn variant="text" @click="$emit('close')">关闭</VBtn>
    </template>
  </ModalDialog>
</template>

<style scoped>
.endpoint-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.endpoint-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 10px;
  border-radius: 10px;
  background: rgba(var(--v-theme-on-surface), 0.04);
  transition: background 0.15s;
  user-select: none;
  cursor: pointer;
}

.endpoint-item:hover {
  background: rgba(var(--v-theme-on-surface), 0.08);
}

.endpoint-item--selected {
  background: rgba(var(--v-theme-primary), 0.08);
}

.endpoint-item--selected:hover {
  background: rgba(var(--v-theme-primary), 0.14);
}

.endpoint-label {
  flex: 1;
  font-weight: 600;
  font-size: 0.875rem;
}

.endpoint-summary {
  margin-right: auto;
  font-size: 0.75rem;
  font-weight: 700;
}
</style>
