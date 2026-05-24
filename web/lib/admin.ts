import type {
  NavItem,
  SettingsForm,
  SettingsSnapshot,
  ThemeMode,
  UiTone,
} from '~/types/admin'
import { credentialStatusLabel, credentialStatusTone, isKnownCredentialStatus, isThrottleTierStatus } from './credentialStatus'

export const THEME_STORAGE_KEY = 'meowcli-admin-theme'
const THEME_META_COLORS: Record<ThemeMode, string> = {
  light: '#EEF2EC',
  dark: '#0F1511',
}

export const NAV_ITEMS: NavItem[] = [
  {
    key: 'dashboard',
    to: '/',
    icon: 'mdi-view-dashboard-outline',
    label: '总览',
    eyebrow: '运行',
  },
  {
    key: 'settings',
    to: '/settings',
    icon: 'mdi-cog-outline',
    label: '设置',
    eyebrow: '策略',
  },
  {
    key: 'credentials',
    to: '/credentials',
    icon: 'mdi-key-outline',
    label: '凭据',
    eyebrow: '凭据池',
  },
  {
    key: 'models',
    to: '/models',
    icon: 'mdi-compare-horizontal',
    label: '模型',
    eyebrow: '映射',
  },
  {
    key: 'logs',
    to: '/logs',
    icon: 'mdi-text-box-outline',
    label: '日志',
    eyebrow: '诊断',
  },
  {
    key: 'keys',
    to: '/keys',
    icon: 'mdi-shield-key-outline',
    label: '密钥',
    eyebrow: '访问',
  },
]


export const PAGE_SIZE_OPTIONS = [
  { title: '25 条 / 页', value: 25 },
  { title: '50 条 / 页', value: 50 },
  { title: '100 条 / 页', value: 100 },
]

export const CREDENTIAL_PAGE_SIZE_OPTIONS = [
  { title: '6 条 / 页', value: 6 },
  { title: '12 条 / 页', value: 12 },
  { title: '30 条 / 页', value: 30 },
  { title: '60 条 / 页', value: 60 },
]

export const GEMINI_BASE_URL_OPTIONS = [
  { title: 'Prod', value: 'prod' },
  { title: 'Daily', value: 'daily' },
  { title: 'Daily Sandbox', value: 'daily_sandbox' },
]

export const ANTIGRAVITY_API_ENDPOINT_OPTIONS = GEMINI_BASE_URL_OPTIONS

export const DEFAULT_SETTINGS_FORM: SettingsForm = {
  global_proxy: '',
  codex_proxy: '',
  gemini_proxy: '',
  antigravity_proxy: '',
  gemini_base_urls: GEMINI_BASE_URL_OPTIONS[0]!.value,
  codex_preferred_plan_types: '',
  gemini_preferred_plan_types: '',
  antigravity_preferred_plan_types: '',
  antigravity_api_endpoint: 'prod',
  refresh_before_seconds: '30',
  poll_interval_milliseconds: '200',
  quota_sync_interval_seconds: '900',
  score_refresh_interval_seconds: '60',
  throttle_base_seconds: '60',
  throttle_max_seconds: '1800',
  logs_retention_seconds: '86400',
  relay_max_retries: '3',
}

const STATUS_LABELS: Record<string, string> = {
  available: '可用',
  planned: '规划中',
}

const ROLE_LABELS: Record<string, string> = {
  admin: '管理员',
  user: '普通成员',
}

const PLAN_TYPE_SPLIT_RE = /[,\s;]+/
const CREDENTIAL_ID_SEPARATOR = '__'

export function normalizeTheme(value?: string | null): ThemeMode {
  return value === 'dark' ? 'dark' : 'light'
}

export function resolveInitialTheme(): ThemeMode {
  if (!import.meta.client) {
    return 'light'
  }

  try {
    const stored = window.localStorage.getItem(THEME_STORAGE_KEY)
    if (stored === 'light' || stored === 'dark') {
      return stored
    }
  } catch {
    return 'light'
  }

  return window.matchMedia?.('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
}

export function applyTheme(theme: ThemeMode) {
  if (!import.meta.client) {
    return
  }

  const normalized = normalizeTheme(theme)
  document.documentElement.dataset.theme = normalized
  document.documentElement.style.colorScheme = normalized

  const meta = document.querySelector('meta[name="color-scheme"]')
  meta?.setAttribute('content', normalized === 'dark' ? 'dark light' : 'light dark')

  const themeMeta = document.querySelector('meta[name="theme-color"]')
  themeMeta?.setAttribute('content', THEME_META_COLORS[normalized])
}

export async function copyText(value: string) {
  if (!import.meta.client) {
    return false
  }

  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(value)
      return true
    }

    return false
  } catch {
    return false
  }
}

export function formatTime(value?: string | null) {
  if (!value || value === '0001-01-01T00:00:00Z' || value === '0001-01-01 00:00:00') {
    return '-'
  }

  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return value
  }

  return new Intl.DateTimeFormat('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }).format(date)
}

export function formatPercent(value?: number | null) {
  if (typeof value !== 'number' || Number.isNaN(value)) {
    return '-'
  }
  return `${Math.round(value * 100)}%`
}

export function statusText(status?: string | null) {
  if (isKnownCredentialStatus(status) || isThrottleTierStatus(status)) {
    return credentialStatusLabel(status)
  }
  return STATUS_LABELS[status ?? ''] || status || '-'
}

export function roleText(role?: string | null) {
  return ROLE_LABELS[role ?? ''] || role || '-'
}

export function toneForStatus(status?: string | null): UiTone {
  if (isKnownCredentialStatus(status) || isThrottleTierStatus(status)) {
    return credentialStatusTone(status)
  }
  switch (status) {
    case 'available':
      return 'success'
    case 'planned':
      return 'accent'
    case 'admin':
      return 'warning'
    default:
      return 'neutral'
  }
}

export function colorForTone(tone?: UiTone) {
  switch (tone) {
    case 'success':
      return 'success'
    case 'danger':
      return 'error'
    case 'warning':
      return 'warning'
    case 'accent':
      return 'tertiary'
    case 'muted':
      return 'surface-variant'
    case 'secondary':
      return 'secondary'
    default:
      return 'primary'
  }
}

export function safeStringify(value: unknown) {
  try {
    return JSON.stringify(value)
  } catch {
    return '{}'
  }
}

export function isZeroTime(value?: string | null) {
  if (!value) {
    return true
  }
  const text = String(value)
  if (text === '' || text === '0001-01-01T00:00:00Z' || text === '0001-01-01 00:00:00') {
    return true
  }

  const date = new Date(text)
  return Number.isNaN(date.getTime()) || date.getFullYear() <= 1
}

export function isPastTime(value?: string | null) {
  if (!value || isZeroTime(value)) return true
  const date = new Date(value)
  return Number.isNaN(date.getTime()) || date.getTime() <= Date.now()
}

export function apiTypesText(types?: string[]) {
  return types?.join(' / ') || '未声明'
}

export function normalizePlanType(value?: string | null) {
  if (!value) {
    return ''
  }
  const text = value.trim().toLowerCase()
  if (!text) {
    return ''
  }
  if (text === '-') {
    return 'unknown'
  }
  return text
}

export function planTypeText(value?: string | null) {
  return normalizePlanType(value) || 'unknown'
}

export function codexCredentialEmail(id?: string | null) {
  const text = id?.trim() || ''
  const idx = text.lastIndexOf(CREDENTIAL_ID_SEPARATOR)
  if (idx <= 0) {
    return '-'
  }
  return text.slice(0, idx) || '-'
}

export function codexCredentialAccountID(id?: string | null) {
  const text = id?.trim() || ''
  const idx = text.lastIndexOf(CREDENTIAL_ID_SEPARATOR)
  if (idx < 0 || idx + CREDENTIAL_ID_SEPARATOR.length >= text.length) {
    return '-'
  }
  return text.slice(idx + CREDENTIAL_ID_SEPARATOR.length) || '-'
}

export function splitPlanTypeInput(value?: string | null, allowedPlanTypes?: string[]) {
  if (!value) {
    return []
  }

  const allowed = allowedPlanTypes?.length
    ? new Set(allowedPlanTypes.map((item) => normalizePlanType(item)).filter(Boolean))
    : null
  const planTypes: string[] = []
  const seen = new Set<string>()
  for (const part of value.split(PLAN_TYPE_SPLIT_RE)) {
    const planType = normalizePlanType(part)
    if (!planType || seen.has(planType) || (allowed && !allowed.has(planType))) {
      continue
    }
    seen.add(planType)
    planTypes.push(planType)
  }
  return planTypes
}

export function joinPlanTypeInput(planTypes: string[], allowedPlanTypes?: string[]) {
  return splitPlanTypeInput(planTypes.join(','), allowedPlanTypes).join(',')
}

export function splitGeminiBaseURLInput(value?: string | null) {
  return splitEndpointInput(value, GEMINI_BASE_URL_OPTIONS)
}

export function splitAntigravityAPIEndpointInput(value?: string | null) {
  return splitEndpointInput(value, ANTIGRAVITY_API_ENDPOINT_OPTIONS)
}

function splitEndpointInput(value: string | null | undefined, options: Array<{ value: string }>) {
  const allowed = new Set(options.map((option) => option.value))
  const selected: string[] = []
  const seen = new Set<string>()
  for (const part of String(value || '').split(PLAN_TYPE_SPLIT_RE)) {
    const endpoint = part.trim().replace(/\/+$/, '')
    if (!allowed.has(endpoint) || seen.has(endpoint)) {
      continue
    }
    seen.add(endpoint)
    selected.push(endpoint)
  }
  return selected.length ? selected : [options[0]!.value]
}

export function joinGeminiBaseURLInput(values: string[]) {
  return splitGeminiBaseURLInput(values.join(',')).join(',')
}

export function joinAntigravityAPIEndpointInput(values: string[]) {
  return splitAntigravityAPIEndpointInput(values.join(',')).join(',')
}

export function geminiBaseURLText(value: string) {
  return GEMINI_BASE_URL_OPTIONS.find((option) => option.value === value)?.title || value
}

export function antigravityAPIEndpointText(value: string) {
  return ANTIGRAVITY_API_ENDPOINT_OPTIONS.find((option) => option.value === value)?.title || value
}

export function settingsToForm(data: SettingsSnapshot): SettingsForm {
  const form: SettingsForm = {
    global_proxy: data.global_proxy,
    codex_proxy: data.codex_proxy,
    gemini_proxy: data.gemini_proxy,
    antigravity_proxy: data.antigravity_proxy,
    gemini_base_urls: joinGeminiBaseURLInput([data.gemini_base_urls]),
    codex_preferred_plan_types: data.codex_preferred_plan_types.trim(),
    gemini_preferred_plan_types: data.gemini_preferred_plan_types.trim(),
    antigravity_preferred_plan_types: data.antigravity_preferred_plan_types.trim(),
    refresh_before_seconds: String(data.refresh_before_seconds),
    poll_interval_milliseconds: String(data.poll_interval_milliseconds),
    quota_sync_interval_seconds: String(data.quota_sync_interval_seconds),
    score_refresh_interval_seconds: String(data.score_refresh_interval_seconds),
    throttle_base_seconds: String(data.throttle_base_seconds),
    throttle_max_seconds: String(data.throttle_max_seconds),
    logs_retention_seconds: String(data.logs_retention_seconds),
    relay_max_retries: String(data.relay_max_retries),
  }
  if (typeof data.antigravity_api_endpoint === 'string') {
    form.antigravity_api_endpoint = joinAntigravityAPIEndpointInput([data.antigravity_api_endpoint])
  }
  if (typeof data.antigravity_use_credits === 'boolean') {
    form.antigravity_use_credits = data.antigravity_use_credits
  }
  return form
}

export function settingsToPayload(form: SettingsForm): SettingsSnapshot {
  const payload: SettingsSnapshot = {
    global_proxy: form.global_proxy.trim(),
    codex_proxy: form.codex_proxy.trim(),
    gemini_proxy: form.gemini_proxy.trim(),
    antigravity_proxy: form.antigravity_proxy.trim(),
    gemini_base_urls: joinGeminiBaseURLInput([form.gemini_base_urls]),
    codex_preferred_plan_types: form.codex_preferred_plan_types.trim(),
    gemini_preferred_plan_types: form.gemini_preferred_plan_types.trim(),
    antigravity_preferred_plan_types: form.antigravity_preferred_plan_types.trim(),
    refresh_before_seconds: Number(form.refresh_before_seconds),
    poll_interval_milliseconds: Number(form.poll_interval_milliseconds),
    quota_sync_interval_seconds: Number(form.quota_sync_interval_seconds),
    score_refresh_interval_seconds: Number(form.score_refresh_interval_seconds),
    throttle_base_seconds: Number(form.throttle_base_seconds),
    throttle_max_seconds: Number(form.throttle_max_seconds),
    logs_retention_seconds: Number(form.logs_retention_seconds),
    relay_max_retries: Number(form.relay_max_retries),
  }
  if (typeof form.antigravity_api_endpoint === 'string') {
    payload.antigravity_api_endpoint = joinAntigravityAPIEndpointInput([form.antigravity_api_endpoint])
  }
  if (typeof form.antigravity_use_credits === 'boolean') {
    payload.antigravity_use_credits = form.antigravity_use_credits
  }
  return payload
}
