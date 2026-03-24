export function useConfirmDialog() {
  const open = ref(false)
  const title = ref('')
  const message = ref('')
  const text = ref('确认')
  const variant = ref<'secondary' | 'danger'>('danger')
  const busy = ref(false)
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
    if (busy.value) return
    open.value = false
    pendingAction = null
  }

  async function submit() {
    if (!pendingAction || busy.value) return
    const action = pendingAction
    close()
    await action()
  }

  return { open, title, message, text, variant, busy, show, close, submit }
}
