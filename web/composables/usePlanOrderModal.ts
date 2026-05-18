import { joinPlanTypeInput, splitPlanTypeInput } from '~/lib/admin'

export function usePlanOrderModal(
  getValue: () => string,
  setValue: (v: string) => void,
  allPlanTypes: () => string[],
) {
  const open = ref(false)
  const draft = ref<string[]>([])
  const dragIdx = ref<number | null>(null)

  function planTypes() {
    return splitPlanTypeInput(allPlanTypes().join(','))
  }

  function selectedPlanTypes() {
    return splitPlanTypeInput(getValue(), planTypes())
  }

  function openModal() {
    const selected = selectedPlanTypes()
    const unselected = planTypes().filter(t => !selected.includes(t))
    draft.value = [...selected, ...unselected]
    open.value = true
  }

  function isSelected(planType: string) {
    return selectedPlanTypes().includes(planType)
  }

  function toggle(planType: string) {
    const allowed = planTypes()
    const selected = selectedPlanTypes()
    const idx = selected.indexOf(planType)
    if (idx >= 0) {
      selected.splice(idx, 1)
    } else {
      selected.push(planType)
    }
    setValue(joinPlanTypeInput(selected, allowed))
    const newSelected = selectedPlanTypes()
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
    const selected = new Set(selectedPlanTypes())
    const ordered = draft.value.filter(t => selected.has(t))
    setValue(joinPlanTypeInput(ordered, planTypes()))
  }

  function closeModal() {
    open.value = false
  }

  const preview = computed(() => selectedPlanTypes())

  function rankOf(planType: string) {
    return selectedPlanTypes().indexOf(planType) + 1
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
