<script setup lang="ts">
import {
  MODEL_SCHEDULING_STRATEGIES,
  PRESERVE_MODEL_SCHEDULING_OPTION,
  type ModelSchedulingOption,
  type ModelSchedulingSelection,
} from '~/lib/modelScheduling'

const props = withDefaults(defineProps<{
  id: string
  modelValue: ModelSchedulingSelection
  allowPreserve?: boolean
}>(), {
  allowPreserve: false,
})

const emit = defineEmits<{
  'update:modelValue': [value: ModelSchedulingSelection]
}>()

const focused = ref(false)

const options = computed<readonly ModelSchedulingOption[]>(() => (
  props.allowPreserve
    ? [PRESERVE_MODEL_SCHEDULING_OPTION, ...MODEL_SCHEDULING_STRATEGIES]
    : MODEL_SCHEDULING_STRATEGIES
))

function select(value: ModelSchedulingSelection) {
  emit('update:modelValue', value)
}

function handleFocusOut(event: FocusEvent) {
  const currentTarget = event.currentTarget as HTMLElement
  if (!currentTarget.contains(event.relatedTarget as Node | null)) {
    focused.value = false
  }
}
</script>

<template>
  <VField
    class="model-scheduling-selector"
    :id="`${id}-field`"
    label="调度策略"
    variant="outlined"
    rounded="lg"
    color="primary"
    active
    :focused="focused"
    @focusin="focused = true"
    @focusout="handleFocusOut"
  >
    <div
      :id="`${id}-field`"
      class="v-field__input model-scheduling-selector__content"
      role="radiogroup"
      aria-label="调度策略"
      :aria-describedby="allowPreserve ? `${id}-description` : undefined"
    >
      <span v-if="allowPreserve" :id="`${id}-description`" class="model-scheduling-selector__description">
        批量选择会覆盖当前策略；“保持不变”不会修改
      </span>

      <div class="model-scheduling-selector__list">
        <label
          v-for="option in options"
          :key="option.value"
          class="model-scheduling-option"
          :class="{ 'model-scheduling-option--selected': modelValue === option.value }"
          @click.stop
        >
          <input
            class="model-scheduling-option__input"
            type="radio"
            :name="id"
            :value="option.value"
            :checked="modelValue === option.value"
            @click.stop
            @change="select(option.value as ModelSchedulingSelection)"
          >
          <span class="model-scheduling-option__icon">
            <VIcon :icon="option.icon" size="21" />
          </span>
          <span class="model-scheduling-option__copy">
            <span class="model-scheduling-option__title">{{ option.label }}</span>
            <span class="model-scheduling-option__description">{{ option.description }}</span>
          </span>
          <span class="model-scheduling-option__indicator" aria-hidden="true">
            <span class="model-scheduling-option__indicator-dot" />
          </span>
        </label>
      </div>
    </div>
  </VField>
</template>

<style scoped>
.model-scheduling-selector {
  --v-input-control-height: 48px;
  grid-area: auto;
  align-self: start;
  min-width: 0;
  margin: 0;
}

.model-scheduling-selector :deep(.v-field__field) {
  min-width: 0;
}

.model-scheduling-selector__content {
  display: grid;
  gap: 8px;
  width: 100%;
  min-width: 0;
  min-height: 0;
  padding: 14px 12px 12px;
}

.model-scheduling-selector__description {
  padding-inline: 4px;
  color: rgba(var(--v-theme-on-surface), 0.58);
  font-size: 0.74rem;
  line-height: 1.35;
}

.model-scheduling-selector__list {
  display: grid;
  grid-template-columns: 1fr;
  gap: 5px;
}

.model-scheduling-option {
  position: relative;
  display: grid;
  grid-template-columns: 24px minmax(0, 1fr) 24px;
  align-items: center;
  gap: 10px;
  min-height: 58px;
  padding: 9px 8px 9px 12px;
  border: 0;
  border-radius: var(--admin-radius-control-sm);
  background: rgba(var(--v-theme-on-surface), 0.035);
  cursor: pointer;
  user-select: none;
}

.model-scheduling-option:hover {
  background: rgba(var(--v-theme-on-surface), 0.07);
}

.model-scheduling-option:focus-within {
  outline: 2px solid rgba(var(--v-theme-primary), 0.44);
  outline-offset: -2px;
}

.model-scheduling-option--selected {
  background: rgba(var(--v-theme-primary-container), 0.3);
}

.model-scheduling-option--selected:hover {
  background: rgba(var(--v-theme-primary-container), 0.4);
}

.model-scheduling-option__input {
  position: absolute;
  width: 1px;
  height: 1px;
  opacity: 0;
  pointer-events: none;
}

.model-scheduling-option__icon {
  display: grid;
  place-items: center;
  width: 24px;
  height: 24px;
  color: rgba(var(--v-theme-on-surface-variant), 0.84);
}

.model-scheduling-option--selected .model-scheduling-option__icon {
  color: rgb(var(--v-theme-primary));
}

.model-scheduling-option__copy {
  display: grid;
  gap: 3px;
  min-width: 0;
}

.model-scheduling-option__title {
  color: rgba(var(--v-theme-on-surface), 0.92);
  font-size: 0.86rem;
  font-weight: 700;
  line-height: 1.3;
}

.model-scheduling-option__description {
  color: rgba(var(--v-theme-on-surface), 0.62);
  font-size: 0.75rem;
  line-height: 1.4;
}

.model-scheduling-option__indicator {
  display: grid;
  place-items: center;
  width: 20px;
  height: 20px;
  border: 2px solid rgba(var(--v-theme-on-surface), 0.48);
  border-radius: 50%;
  justify-self: end;
}

.model-scheduling-option__indicator-dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  background: rgb(var(--v-theme-primary));
  opacity: 0;
  transform: scale(0);
  transition:
    opacity 70ms linear,
    transform 110ms cubic-bezier(0.2, 0, 0, 1);
}

.model-scheduling-option--selected .model-scheduling-option__title {
  color: rgb(var(--v-theme-primary)) !important;
}

.model-scheduling-option--selected .model-scheduling-option__description {
  color: rgba(var(--v-theme-on-surface), 0.7);
}

.model-scheduling-option--selected .model-scheduling-option__indicator {
  border-color: rgb(var(--v-theme-primary));
}

.model-scheduling-option--selected .model-scheduling-option__indicator-dot {
  opacity: 1;
  transform: scale(1);
}

@media (prefers-reduced-motion: reduce) {
  .model-scheduling-option__indicator-dot {
    transition: none;
  }
}

@media (max-width: 480px) {
  .model-scheduling-selector__content {
    padding-inline: 10px;
  }

  .model-scheduling-option {
    padding-inline: 10px 8px;
  }
}
</style>
