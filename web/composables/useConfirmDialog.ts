export function useConfirmDialog() {
  const open = ref(false)
  const title = ref('')
  const message = ref('')
  const text = ref('确认')
  const variant = ref<'secondary' | 'danger'>('danger')
  let pendingAction: null | (() => Promise<void>) = null

  function show(options: {
    title: string
    message: string
    confirmText?: string
    confirmVariant?: 'secondary' | 'danger'
    action: () => Promise<void>
  }) {
    title.value = options.title
    message.value = options.message
    text.value = options.confirmText || '确认'
    variant.value = options.confirmVariant || 'danger'
    pendingAction = options.action
    open.value = true
  }

  function close() {
    open.value = false
    pendingAction = null
  }

  // busy/loading 状态由调用方在 action 内自行维护(各页面的 actionBusy),
  // submit 先同步 close 使 pendingAction 失效,天然防止重复提交
  async function submit() {
    if (!pendingAction) return
    const action = pendingAction
    close()
    await action()
  }

  return { open, title, message, text, variant, show, close, submit }
}
