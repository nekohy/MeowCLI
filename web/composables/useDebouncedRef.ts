/**
 * 防抖搜索输入:input 即时更新,query 延迟 delay ms 后跟随(input 去空白)。
 * 页面直接重置时同时写 input 与 query 即可,定时器在组件卸载时自动清理。
 */
export function useDebouncedRef(initial = '', delay = 250) {
  const input = ref(initial)
  const query = ref(initial.trim())

  let timer: ReturnType<typeof setTimeout> | undefined

  watch(input, (value) => {
    if (timer) {
      clearTimeout(timer)
    }
    timer = setTimeout(() => {
      timer = undefined
      query.value = value.trim()
    }, delay)
  })

  onBeforeUnmount(() => {
    if (timer) {
      clearTimeout(timer)
    }
  })

  return { input, query }
}
