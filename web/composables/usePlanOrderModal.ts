import { joinPlanTypeInput, splitPlanTypeInput } from '~/lib/admin'

export function usePlanOrderModal(
  getValue: () => string,
  setValue: (v: string) => void,
  allPlanTypes: () => string[],
) {
  const open = ref(false)
  const draft = ref<string[]>([])
  const dragIdx = ref<number | null>(null)

  function openModal() {
    const selected = splitPlanTypeInput(getValue())
    const unselected = allPlanTypes().filter(t => !selected.includes(t))
    draft.value = [...selected, ...unselected]
    open.value = true
  }

  function isSelected(planType: string) {
    return splitPlanTypeInput(getValue()).includes(planType)
  }

  function toggle(planType: string) {
    const selected = splitPlanTypeInput(getValue())
    const idx = selected.indexOf(planType)
    if (idx >= 0) {
      selected.splice(idx, 1)
    } else {
      selected.push(planType)
    }
    setValue(joinPlanTypeInput(selected))
    const newSelected = splitPlanTypeInput(getValue())
    const remaining = draft.value.filter(t => !newSelected.includes(t))
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
    const selected = new Set(splitPlanTypeInput(getValue()))
    const ordered = draft.value.filter(t => selected.has(t))
    setValue(joinPlanTypeInput(ordered))
  }

  function closeModal() {
    open.value = false
  }

  const preview = computed(() => splitPlanTypeInput(getValue()))

  function rankOf(planType: string) {
    return splitPlanTypeInput(getValue()).indexOf(planType) + 1
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
