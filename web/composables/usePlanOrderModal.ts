import { joinPlanTypeInput, splitPlanTypeInput } from '~/lib/admin'

export function usePlanOrderModal(
  getValue: () => string,
  setValue: (v: string) => void,
  allPlanTypes: () => string[],
) {
  function planTypes() {
    return splitPlanTypeInput(allPlanTypes().join(','))
  }

  return useOrderedSelectionModal(
    () => splitPlanTypeInput(getValue(), planTypes()),
    (value) => { setValue(joinPlanTypeInput(value, planTypes())) },
    planTypes,
  )
}
