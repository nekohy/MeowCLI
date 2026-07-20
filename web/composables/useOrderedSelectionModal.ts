export function useOrderedSelectionModal(
  getSelected: () => string[],
  setSelected: (value: string[]) => void,
  allItems: () => string[],
) {
  const open = ref(false)
  const draft = ref<string[]>([])
  const dragIdx = ref<number | null>(null)

  // computed 缓存归一化结果:isSelected/rankOf 在模板中按项调用,
  // 普通函数会让每次渲染重复 trim/filter/去重(O(n²))
  const items = computed(() => (
    allItems()
      .map((item) => item.trim())
      .filter(Boolean)
      .filter((item, index, list) => list.indexOf(item) === index)
  ))

  const selectedItems = computed(() => {
    const allowed = new Set(items.value)
    return getSelected()
      .map((item) => item.trim())
      .filter(Boolean)
      .filter((item, index, list) => list.indexOf(item) === index && allowed.has(item))
  })

  function openModal() {
    const selected = selectedItems.value
    const unselected = items.value.filter((item) => !selected.includes(item))
    draft.value = [...selected, ...unselected]
    open.value = true
  }

  function isSelected(item: string) {
    return selectedItems.value.includes(item)
  }

  function toggle(item: string) {
    const selected = [...selectedItems.value]
    const idx = selected.indexOf(item)
    if (idx >= 0) {
      selected.splice(idx, 1)
    } else {
      selected.push(item)
    }
    setSelected(selected)
    const newSelected = selectedItems.value
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
    const selected = new Set(selectedItems.value)
    const ordered = draft.value.filter((item) => selected.has(item))
    setSelected(ordered)
  }

  function closeModal() {
    open.value = false
  }

  const preview = selectedItems

  function rankOf(item: string) {
    return selectedItems.value.indexOf(item) + 1
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
