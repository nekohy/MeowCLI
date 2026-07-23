<script setup lang="ts">
import type { ComponentPublicInstance } from 'vue'

export interface CategoryTabItem {
  key: string
  label: string
  icon?: string
}

const props = withDefaults(defineProps<{
  items: CategoryTabItem[]
  modelValue: string
  // 页面上有多个实例或需要稳定的 aria 关联时传入,用于 tab/panel 的 id 前缀
  idPrefix?: string
}>(), {
  idPrefix: 'category-tabs',
})

const emit = defineEmits<{
  'update:modelValue': [key: string]
}>()

const scroller = ref<HTMLElement | null>(null)
const tabElements = new Map<string, HTMLElement>()
const canScrollStart = ref(false)
const canScrollEnd = ref(false)

function setTabRef(key: string, el: Element | ComponentPublicInstance | null) {
  if (el instanceof HTMLElement) {
    tabElements.set(key, el)
  } else {
    tabElements.delete(key)
  }
}

function updateScrollState() {
  const el = scroller.value
  if (!el) {
    return
  }
  canScrollStart.value = el.scrollLeft > 1
  canScrollEnd.value = el.scrollLeft + el.clientWidth < el.scrollWidth - 1
}

function scrollTabIntoView(key: string) {
  tabElements.get(key)?.scrollIntoView({ behavior: 'smooth', inline: 'nearest', block: 'nearest' })
}

function select(key: string, focus = false) {
  if (key !== props.modelValue) {
    emit('update:modelValue', key)
  }
  nextTick(() => {
    if (focus) {
      tabElements.get(key)?.focus()
    }
    scrollTabIntoView(key)
  })
}

// 键盘漫游:方向键/Home/End 自动激活并聚焦目标 tab(ARIA tabs 自动激活模式)
function onKeydown(event: KeyboardEvent, index: number) {
  const last = props.items.length - 1
  let nextIndex: number | null = null
  if (event.key === 'ArrowRight') {
    nextIndex = index >= last ? 0 : index + 1
  } else if (event.key === 'ArrowLeft') {
    nextIndex = index <= 0 ? last : index - 1
  } else if (event.key === 'Home') {
    nextIndex = 0
  } else if (event.key === 'End') {
    nextIndex = last
  }
  if (nextIndex === null) {
    return
  }
  event.preventDefault()
  const next = props.items[nextIndex]
  if (next) {
    select(next.key, true)
  }
}

watch(() => props.modelValue, (key) => {
  nextTick(() => scrollTabIntoView(key))
})

watch(() => props.items.length, () => {
  nextTick(updateScrollState)
})

let resizeObserver: ResizeObserver | null = null

onMounted(() => {
  updateScrollState()
  scrollTabIntoView(props.modelValue)
  if (scroller.value && typeof ResizeObserver !== 'undefined') {
    resizeObserver = new ResizeObserver(updateScrollState)
    resizeObserver.observe(scroller.value)
  }
})

onBeforeUnmount(() => {
  resizeObserver?.disconnect()
})
</script>

<template>
  <div
    class="category-tabs surface-card"
    :class="{
      'category-tabs--fade-start': canScrollStart,
      'category-tabs--fade-end': canScrollEnd,
    }"
  >
    <div
      ref="scroller"
      class="category-tabs__scroller"
      role="tablist"
      aria-label="设置分类"
      @scroll.passive="updateScrollState"
    >
      <button
        v-for="(item, index) in items"
        :key="item.key"
        :ref="(el) => setTabRef(item.key, el)"
        :id="`${idPrefix}-tab-${item.key}`"
        type="button"
        role="tab"
        class="category-tab"
        :class="{ 'category-tab--active': item.key === modelValue }"
        :aria-selected="item.key === modelValue"
        :aria-controls="`${idPrefix}-panel-${item.key}`"
        :tabindex="item.key === modelValue ? 0 : -1"
        @click="select(item.key)"
        @keydown="onKeydown($event, index)"
      >
        <VIcon v-if="item.icon" :icon="item.icon" size="18" class="category-tab__icon" />
        <span>{{ item.label }}</span>
      </button>
    </div>
  </div>
</template>

<style scoped>
.category-tabs {
  padding: 6px;
  border-radius: var(--admin-radius-panel);
}

.category-tabs__scroller {
  display: flex;
  gap: 4px;
  overflow-x: auto;
  overscroll-behavior-x: contain;
  scrollbar-color: rgba(var(--v-theme-primary), 0.46) transparent;
  scrollbar-width: thin;
}

/* 细滚动条:与 open-code-go-rewards-list 同一语言,保留桌面端可发现性 */
.category-tabs__scroller::-webkit-scrollbar {
  height: 6px;
}

.category-tabs__scroller::-webkit-scrollbar-track {
  background: transparent;
}

.category-tabs__scroller::-webkit-scrollbar-thumb {
  border: 1px solid transparent;
  border-radius: 999px;
  background: rgba(var(--v-theme-primary), 0.4);
  background-clip: padding-box;
}

.category-tabs__scroller::-webkit-scrollbar-thumb:hover {
  background: rgba(var(--v-theme-primary), 0.62);
  background-clip: padding-box;
}

/* 边缘渐隐提示可滚动方向(仅在实际溢出时启用,避免裁切文字) */
.category-tabs--fade-start.category-tabs--fade-end .category-tabs__scroller {
  mask-image: linear-gradient(to right, transparent, #000 28px, #000 calc(100% - 28px), transparent);
}

.category-tabs--fade-start:not(.category-tabs--fade-end) .category-tabs__scroller {
  mask-image: linear-gradient(to right, transparent, #000 28px);
}

.category-tabs--fade-end:not(.category-tabs--fade-start) .category-tabs__scroller {
  mask-image: linear-gradient(to left, transparent, #000 28px);
}

.category-tab {
  display: inline-flex;
  flex: 0 0 auto;
  align-items: center;
  gap: 8px;
  min-height: 38px;
  padding-inline: 16px;
  border: 0;
  border-radius: var(--admin-radius-nav);
  background: transparent;
  color: rgba(var(--v-theme-on-surface), 0.72);
  cursor: pointer;
  font-family: inherit;
  font-size: 0.92rem;
  font-weight: 600;
  letter-spacing: 0.015em;
  line-height: 1.2;
  white-space: nowrap;
  transition:
    background-color var(--md-dur-fast) var(--md-motion-standard),
    color var(--md-dur-fast) var(--md-motion-standard),
    transform var(--md-dur-fast) var(--md-motion-standard);
}

.category-tab:active {
  transform: scale(0.97);
  transition-duration: var(--md-dur-press);
}

/* hover/激活与侧边导航同一 primary-container 浸染刻度 */
.category-tab:hover {
  background: var(--admin-nav-hover);
  color: rgb(var(--v-theme-on-surface));
}

/* 键盘聚焦沿用 hover 的 primary-container 浸染,不再画矩形焦点框 */
.category-tab:focus-visible {
  outline: none;
  background: var(--admin-nav-hover);
  color: rgb(var(--v-theme-on-surface));
}

.category-tab:focus-visible .category-tab__icon {
  color: rgb(var(--v-theme-primary));
}

.category-tab--active {
  background: var(--admin-nav-active);
  color: rgb(var(--v-theme-on-surface));
  font-weight: 700;
}

.category-tab__icon {
  color: rgba(var(--v-theme-on-surface), 0.6);
  transition: color var(--md-dur-fast) var(--md-motion-standard);
}

.category-tab:hover .category-tab__icon,
.category-tab--active .category-tab__icon {
  color: rgb(var(--v-theme-primary));
}
</style>
