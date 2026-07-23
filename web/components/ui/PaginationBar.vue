<script setup lang="ts">
const props = withDefaults(defineProps<{
  total: number
  page: number
  maxPage: number
  showSummary?: boolean
  density?: 'default' | 'comfortable' | 'compact'
}>(), {
  showSummary: true,
  density: 'comfortable',
})

const emit = defineEmits<{
  change: [page: number]
}>()

// 连续 3 个数字页码窗口，随当前页平移（1/2/3 → 2/3/4 → 3/4/5），末端封顶不越界；
// 桌面端与移动端共用同一套 DOM 与算法
const visiblePages = computed(() => {
  const max = Math.max(1, props.maxPage)
  const count = Math.min(3, max)
  const start = Math.min(Math.max(props.page, 1), max - count + 1)
  return Array.from({ length: count }, (_, index) => start + index)
})

// 翻页方向驱动窗口过渡动画：向前滑出/向后滑入，布局本身不位移
const pageDirection = ref<'forward' | 'backward'>('forward')
watch(
  () => props.page,
  (next, prev) => {
    if (next === prev) return
    pageDirection.value = next > prev ? 'forward' : 'backward'
  },
)

const pageTransitionName = computed(() => `pager-${pageDirection.value}`)

// 全局 VBtn 默认 height:40 会生成内联 height，压过 compact 的 32px CSS；
// 这里按密度显式给定高度，与 main.css 的 40/32 尺寸链保持一致
const controlHeight = computed(() => (props.density === 'compact' ? 32 : 40))

function goToPage(target: number) {
  if (target < 1 || target > props.maxPage || target === props.page) return
  emit('change', target)
}
</script>

<template>
  <div class="pagination-bar" :class="`pagination-bar--density-${props.density}`">
    <div class="pagination-bar__leading">
      <slot name="leading" />
      <div v-if="showSummary" class="text-body-2 text-medium-emphasis">
        共 {{ total }} 条，当前第 {{ page }} / {{ maxPage }} 页
      </div>
    </div>
    <!-- 复用 v-pagination 结构/样式类：尺寸、圆角、激活态、居中规则全部走 main.css 既有链路 -->
    <nav
      class="pagination-bar__pager v-pagination"
      role="navigation"
      aria-label="分页导航"
    >
      <ul class="v-pagination__list">
        <li class="v-pagination__prev">
          <VBtn
            icon
            rounded="lg"
            :density="props.density"
            :height="controlHeight"
            :disabled="page <= 1"
            aria-label="上一页"
            @click="goToPage(page - 1)"
          >
            <VIcon icon="$prev" />
          </VBtn>
        </li>
        <TransitionGroup :name="pageTransitionName">
          <li
            v-for="item in visiblePages"
            :key="item"
            class="v-pagination__item"
            :class="{ 'v-pagination__item--is-active': item === page }"
          >
            <VBtn
              icon
              rounded="lg"
              :density="props.density"
            :height="controlHeight"
              :color="item === page ? 'primary' : undefined"
              :aria-current="item === page ? 'page' : undefined"
              :aria-label="`第 ${item} 页`"
              @click="goToPage(item)"
            >
              {{ item }}
            </VBtn>
          </li>
        </TransitionGroup>
        <li class="v-pagination__next">
          <VBtn
            icon
            rounded="lg"
            :density="props.density"
            :height="controlHeight"
            :disabled="page >= maxPage"
            aria-label="下一页"
            @click="goToPage(page + 1)"
          >
            <VIcon icon="$next" />
          </VBtn>
        </li>
      </ul>
    </nav>
  </div>
</template>
