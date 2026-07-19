/**
 * 登录态就绪后加载首屏数据。
 * authReady 在服务端恒为 false、仅在客户端异步置 true,
 * 因此 immediate watch 一处即可覆盖「已就绪」与「等待就绪」两种挂载时序。
 */
export function useAuthReadyLoader(load: () => void | Promise<void>) {
  const admin = useAdminApp()
  watch(
    () => admin.authReady.value,
    (ready) => {
      if (ready) {
        void load()
      }
    },
    { immediate: true },
  )
}
