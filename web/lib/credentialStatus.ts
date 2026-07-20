import type { UiTone } from '../types/admin'

const CREDENTIAL_STATUS_FILTER_ALL = 'all' as const

const KNOWN_CREDENTIAL_STATUSES = ['enabled', 'disabled'] as const
const KNOWN_THROTTLE_STATUS_PREFIX = 'throttled:' as const
export const CREDENTIAL_THROTTLE_STATUS_ALL = `${KNOWN_THROTTLE_STATUS_PREFIX}all` as const

export type KnownCredentialStatus = typeof KNOWN_CREDENTIAL_STATUSES[number]
export type CredentialStatusFilter = string

const CREDENTIAL_STATUS_LABELS: Record<KnownCredentialStatus, string> = {
  enabled: '启用',
  disabled: '停用',
}

const CREDENTIAL_STATUS_TONES: Record<KnownCredentialStatus, UiTone> = {
  enabled: 'success',
  disabled: 'danger',
}

const CREDENTIAL_STATUS_ICONS: Record<KnownCredentialStatus, string> = {
  enabled: 'mdi-check',
  disabled: 'mdi-close-circle-outline',
}

const THROTTLE_TIER_LABELS: Record<string, string> = {
  all: '节流中',
  default: '主节流',
  spark: 'Spark节流',
  pro: 'Pro节流',
  flash: 'Flash节流',
  flashlite: 'Lite节流',
  claude: 'Claude节流',
  tab: 'Tab节流',
  image: 'Image节流',
}

export function isKnownCredentialStatus(status?: string | null): status is KnownCredentialStatus {
  return KNOWN_CREDENTIAL_STATUSES.includes(String(status || '') as KnownCredentialStatus)
}

export function credentialStatusLabel(status?: string | null) {
  if (isKnownCredentialStatus(status)) {
    return CREDENTIAL_STATUS_LABELS[status]
  }
  if (isThrottleTierStatus(status)) {
    const value = String(status)
    return THROTTLE_TIER_LABELS[value.slice(KNOWN_THROTTLE_STATUS_PREFIX.length)] || value
  }
  return status || '-'
}

export function credentialStatusTone(status?: string | null): UiTone {
  if (isKnownCredentialStatus(status)) {
    return CREDENTIAL_STATUS_TONES[status]
  }
  if (isThrottleTierStatus(status)) {
    return 'warning'
  }
  return 'neutral'
}

export function credentialStatusIcon(status?: string | null) {
  if (isKnownCredentialStatus(status)) {
    return CREDENTIAL_STATUS_ICONS[status]
  }
  if (isThrottleTierStatus(status)) {
    return 'mdi-timer-sand'
  }
  return 'mdi-close-circle-outline'
}

export function isThrottleTierStatus(status?: string | null) {
  return String(status || '').startsWith(KNOWN_THROTTLE_STATUS_PREFIX)
}

export function credentialStatusQueryValue(statuses: CredentialStatusFilter[]): string[] | undefined {
  const selected = statuses.filter((status) => status && status !== CREDENTIAL_STATUS_FILTER_ALL)
  return selected.length ? selected : undefined
}

function credentialBaseStatus(statuses?: string[] | string | null) {
  const list = Array.isArray(statuses) ? statuses : [String(statuses || '')]
  return list.find(isKnownCredentialStatus) || ''
}

export function credentialStatusBadges(statuses?: string[] | string | null) {
  const list = Array.isArray(statuses) ? statuses : [String(statuses || '')]
  return list.filter((status) => isKnownCredentialStatus(status) || isThrottleTierStatus(status))
}

export function shouldShowCredentialReason(statuses?: string[] | string | null) {
  const baseStatus = credentialBaseStatus(statuses)
  return baseStatus === 'disabled'
}

export function credentialReasonLabel(statuses?: string[] | string | null) {
  const baseStatus = credentialBaseStatus(statuses)
  if (baseStatus === 'disabled') {
    return '停用原因'
  }
  return '状态原因'
}
