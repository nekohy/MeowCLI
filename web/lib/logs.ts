import { formatTime } from '~/lib/admin'
import type { LogItem } from '~/types/admin'

export function hasLogError(error = '') {
  return error
    .split(/\r?\n/)
    .map((item) => item.trim())
    .some(Boolean)
}

export function logMetaItems(item: LogItem, missingCredentialLabel = '未记录凭据') {
  return [
    { label: '模型', value: item.model || '未记录模型' },
    { label: '接口', value: formatApiType(item.api_type) },
    { label: '模式', value: streamMode(item) },
    { label: '首字', value: formatLogSeconds(item.first_byte) },
    { label: '用时', value: formatLogSeconds(item.duration) },
    { label: '时间', value: formatTime(item.created_at) },
    { label: '凭据', value: item.credential_id || missingCredentialLabel, wide: true },
  ]
}

export function logItemKey(item: LogItem) {
  return `${item.handler}-${item.credential_id}-${item.created_at}-${item.status_code}-${item.model || ''}`
}

function formatApiType(apiType = '') {
  return apiType ? apiType.charAt(0).toUpperCase() + apiType.slice(1) : '未记录接口'
}

function formatLogSeconds(value?: number) {
  return typeof value === 'number' && value > 0 ? `${value.toFixed(2)}s` : '-'
}

function streamMode(item: LogItem) {
  return item.stream ? '流式' : '非流'
}
