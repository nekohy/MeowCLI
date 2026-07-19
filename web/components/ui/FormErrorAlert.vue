<script setup lang="ts">
/** 表单内联错误提示:统一各弹窗/卡片的 VAlert 错误块写法 */
withDefaults(defineProps<{
  message?: string
  /** 多行错误(如导入失败清单)保留换行 */
  preWrap?: boolean
}>(), {
  message: '',
  preWrap: false,
})
</script>

<template>
  <VAlert
    v-if="message"
    type="error"
    variant="tonal"
    density="comfortable"
    :text="message"
    :style="preWrap ? 'white-space: pre-wrap' : undefined"
  />
</template>
