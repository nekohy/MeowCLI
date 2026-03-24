<script setup lang="ts">
const props = withDefaults(defineProps<{
  variant?: 'primary' | 'secondary' | 'ghost' | 'danger'
  size?: 'md' | 'sm'
  block?: boolean
  type?: 'button' | 'submit' | 'reset'
  disabled?: boolean
  loading?: boolean
  prependIcon?: string
  appendIcon?: string
}>(), {
  variant: 'primary',
  size: 'md',
  block: false,
  type: 'button',
  disabled: false,
  loading: false,
  prependIcon: undefined,
  appendIcon: undefined,
})

const variantMap = {
  primary: { color: 'primary', style: 'flat' },
  secondary: { color: 'secondary', style: 'tonal' },
  ghost: { color: 'primary', style: 'text' },
  danger: { color: 'error', style: 'tonal' },
} as const

const buttonColor = computed(() => variantMap[props.variant].color)
const buttonVariant = computed(() => variantMap[props.variant].style)
const buttonSize = computed(() => (props.size === 'sm' ? 'small' : 'default'))
</script>

<template>
  <VBtn
    :type="type"
    :disabled="disabled"
    :loading="loading"
    :block="block"
    :color="buttonColor"
    :variant="buttonVariant"
    :size="buttonSize"
    :prepend-icon="prependIcon"
    :append-icon="appendIcon"
    class="text-none font-weight-medium"
  >
    <slot />
  </VBtn>
</template>
