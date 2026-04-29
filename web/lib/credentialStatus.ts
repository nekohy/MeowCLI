import type { UiTone } from '../types/admin'

export const CREDENTIAL_STATUS_FILTER_ALL = 'all' as const

export const KNOWN_CREDENTIAL_STATUSES = ['enabled', 'disabled', 'throttled'] as const

export type KnownCredentialStatus = typeof KNOWN_CREDENTIAL_STATUSES[number]
export type CredentialStatusFilter = typeof CREDENTIAL_STATUS_FILTER_ALL | KnownCredentialStatus

const CREDENTIAL_STATUS_LABELS: Record<KnownCredentialStatus, string> = {
  enabled: '启用',
  disabled: '停用',
  throttled: '节流中',
}

const CREDENTIAL_STATUS_TONES: Record<KnownCredentialStatus, UiTone> = {
  enabled: 'success',
  disabled: 'danger',
  throttled: 'warning',
}

const CREDENTIAL_STATUS_ICONS: Record<KnownCredentialStatus, string> = {
  enabled: 'mdi-check',
  disabled: 'mdi-close-circle',
  throttled: 'mdi-timer-sand',
}

export function isKnownCredentialStatus(status?: string | null): status is KnownCredentialStatus {
  return KNOWN_CREDENTIAL_STATUSES.includes(String(status || '') as KnownCredentialStatus)
}

export function credentialStatusLabel(status?: string | null) {
  if (isKnownCredentialStatus(status)) {
    return CREDENTIAL_STATUS_LABELS[status]
  }
  return status || '-'
}

export function credentialStatusTone(status?: string | null): UiTone {
  if (isKnownCredentialStatus(status)) {
    return CREDENTIAL_STATUS_TONES[status]
  }
  return 'neutral'
}

export function credentialStatusIcon(status?: string | null) {
  if (isKnownCredentialStatus(status)) {
    return CREDENTIAL_STATUS_ICONS[status]
  }
  return 'mdi-close-circle'
}

export function credentialStatusQueryValue(status: CredentialStatusFilter): KnownCredentialStatus | undefined {
  return isKnownCredentialStatus(status) ? status : undefined
}

export function credentialStatusFilterOptions(statuses: Iterable<string>) {
  const available = new Set<KnownCredentialStatus>()
  for (const status of statuses) {
    if (isKnownCredentialStatus(status)) {
      available.add(status)
    }
  }

  return [
    { value: CREDENTIAL_STATUS_FILTER_ALL, label: '全部状态' },
    ...KNOWN_CREDENTIAL_STATUSES
      .filter((status) => available.has(status))
      .map((status) => ({ value: status, label: credentialStatusLabel(status) })),
  ]
}

export function shouldShowCredentialReason(status?: string | null) {
  return status === 'disabled' || status === 'throttled'
}

export function credentialReasonLabel(status?: string | null) {
  if (status === 'throttled') {
    return '节流原因'
  }
  if (status === 'disabled') {
    return '停用原因'
  }
  return '状态原因'
}
