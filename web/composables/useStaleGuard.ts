/**
 * 竞态守卫：收敛"自增序号 + await 后比对丢弃晚到响应"的手写 token 模式。
 *
 * 用法：
 *   const guard = useStaleGuard()
 *   const token = guard.next()
 *   const data = await fetcher()
 *   if (guard.isStale(token)) return   // 已有更新的请求/已关闭,丢弃
 *   ...写状态
 *   关闭/切换目标时调 guard.invalidate() 使 in-flight 立即失效
 */
export function useStaleGuard() {
  let current = 0

  function next() {
    return ++current
  }

  function isStale(token: number) {
    return token !== current
  }

  function invalidate() {
    current += 1
  }

  return { next, isStale, invalidate }
}
