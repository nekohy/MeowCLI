export function useOrderedSelectionModal(
  getSelected: () => string[],
  setSelected: (value: string[]) => void,
  allItems: () => string[],
) {
  const open = ref(false)
  const draft = ref<string[]>([])
  const dragIdx = ref<number | null>(null)

  function items() {
    return allItems()
      .map((item) => item.trim())
      .filter(Boolean)
      .filter((item, index, list) => list.indexOf(item) === index)
  }

  function selectedItems() {
    const allowed = new Set(items())
    return getSelected()
      .map((item) => item.trim())
      .filter(Boolean)
      .filter((item, index, list) => list.indexOf(item) === index && allowed.has(item))
  }

  function openModal() {
    const selected = selectedItems()
    const unselected = items().filter((item) => !selected.includes(item))
    draft.value = [...selected, ...unselected]
    open.value = true
  }

  function isSelected(item: string) {
    return selectedItems().includes(item)
  }

  function toggle(item: string) {
    const selected = selectedItems()
    const idx = selected.indexOf(item)
    if (idx >= 0) {
      selected.splice(idx, 1)
    } else {
      selected.push(item)
    }
    setSelected(selected)
    const newSelected = selectedItems()
    const remaining = draft.value.filter((draftItem) => !newSelected.includes(draftItem))
    draft.value = [...newSelected, ...remaining]
  }

  function onDragStart(idx: number) {
    dragIdx.value = idx
  }

  function onDragOver(e: DragEvent, idx: number) {
    e.preventDefault()
    if (dragIdx.value === null || dragIdx.value === idx) return
    const list = [...draft.value]
    const moved = list.splice(dragIdx.value, 1)[0]
    if (moved === undefined) return
    list.splice(idx, 0, moved)
    draft.value = list
    dragIdx.value = idx
  }

  function onDragEnd() {
    dragIdx.value = null
    const selected = new Set(selectedItems())
    const ordered = draft.value.filter((item) => selected.has(item))
    setSelected(ordered)
  }

  function closeModal() {
    open.value = false
  }

  const preview = computed(() => selectedItems())

  function rankOf(item: string) {
    return selectedItems().indexOf(item) + 1
  }

  return {
    open,
    draft,
    dragIdx,
    openModal,
    isSelected,
    toggle,
    onDragStart,
    onDragOver,
    onDragEnd,
    closeModal,
    preview,
    rankOf,
  }
}
