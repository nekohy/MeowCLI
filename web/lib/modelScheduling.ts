import type { ModelScheduling } from '~/types/admin'

export interface ModelSchedulingOption {
  value: string
  label: string
  description: string
  icon: string
}

export const MODEL_SCHEDULING_STRATEGIES = [
  {
    value: 'default',
    label: '常规调度',
    description: '按凭据评分与可用状态选择凭据',
    icon: 'mdi-tune-variant',
  },
  {
    value: 'content_affinity',
    label: '内容亲和',
    description: '测试性功能，复用请求 hash 命中率高的凭据，可能存在bug，可能影响性能',
    icon: 'mdi-vector-link',
  },
  {
    value: 'fill_first',
    label: '凭据续用',
    description: '同一模型持续复用同一凭据，发生需换号的错误后再切换',
    icon: 'mdi-account-sync-outline',
  },
] as const satisfies readonly ModelSchedulingOption[]

export const PRESERVE_MODEL_SCHEDULING_OPTION = {
  value: 'preserve',
  label: '保持不变',
  description: '保留每个已选模型当前的调度策略',
  icon: 'mdi-content-save-outline',
} as const satisfies ModelSchedulingOption

export type ModelSchedulingStrategy = typeof MODEL_SCHEDULING_STRATEGIES[number]['value']
export type ModelSchedulingSelection = ModelSchedulingStrategy | typeof PRESERVE_MODEL_SCHEDULING_OPTION.value

export function resolveModelSchedulingStrategy(scheduling: ModelScheduling): ModelSchedulingStrategy {
  if (scheduling.content_affinity) {
    return 'content_affinity'
  }
  if (scheduling.fill_first) {
    return 'fill_first'
  }
  return 'default'
}

export function modelSchedulingFields(strategy: ModelSchedulingStrategy): ModelScheduling {
  return {
    content_affinity: strategy === 'content_affinity',
    fill_first: strategy === 'fill_first',
  }
}
