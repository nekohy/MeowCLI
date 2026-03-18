import type {
  NavItem,
  SettingsForm,
  SettingsSnapshot,
  ThemeMode,
  UiTone,
} from '~/types/admin'

export const THEME_STORAGE_KEY = 'meowcli-admin-theme'

export const NAV_ITEMS: NavItem[] = [
  {
    key: 'dashboard',
    to: '/',
    label: '总览',
    eyebrow: '运行状态',
    description: '查看处理器状态、凭据规模与最近请求。',
  },
  {
    key: 'settings',
    to: '/settings',
    label: '设置',
    eyebrow: '运行参数',
    description: '调整代理、刷新节奏、重试退避和日志保留时间。',
  },
  {
    key: 'credentials',
    to: '/credentials',
    label: '凭据',
    eyebrow: 'CLI 令牌池',
    description: '集中管理导入令牌、状态切换和配额同步。',
  },
  {
    key: 'models',
    to: '/models',
    label: '模型',
    eyebrow: '别名映射',
    description: '维护外部模型别名与上游模型的映射关系。',
  },
  {
    key: 'logs',
    to: '/logs',
    label: '日志',
    eyebrow: '请求记录',
    description: '排查内存中的近期请求、状态码与错误输出。',
  },
  {
    key: 'keys',
    to: '/keys',
    label: '密钥',
    eyebrow: '访问控制',
    description: '管理后台和 API 的访问密钥。',
  },
]

export const DEFAULT_SETTINGS_FORM: SettingsForm = {
  allow_user_plan_type_header: false,
  global_proxy: '',
  codex_proxy: '',
  codex_delete_free_accounts: false,
  codex_allow_user_plan_type_header: false,
  codex_preferred_plan_types: '',
  refresh_before_seconds: '30',
  poll_interval_milliseconds: '200',
  quota_sync_interval_seconds: '900',
  throttle_base_seconds: '60',
  throttle_max_seconds: '1800',
  logs_retention_seconds: '86400',
  relay_max_retries: '3',
}

const STATUS_LABELS: Record<string, string> = {
  enabled: '启用',
  disabled: '停用',
  available: '可用',
  planned: '规划中',
}

const ROLE_LABELS: Record<string, string> = {
  admin: '管理员',
  user: '普通成员',
}

const PLAN_TYPE_SPLIT_RE = /[,\s;]+/

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
  return STATUS_LABELS[status ?? ''] || status || '-'
}

export function roleText(role?: string | null) {
  return ROLE_LABELS[role ?? ''] || role || '-'
}

export function toneForStatus(status?: string | null): UiTone {
  switch (status) {
    case 'enabled':
    case 'available':
      return 'success'
    case 'disabled':
      return 'muted'
    case 'planned':
      return 'accent'
    case 'admin':
      return 'warning'
    default:
      return 'neutral'
  }
}

export function safeStringify(value: unknown) {
  try {
    return JSON.stringify(value)
  } catch {
    return '{}'
  }
}

export function isUnsynced(value?: string | null) {
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

export function isZeroTime(value?: string | null) {
  return isUnsynced(value)
}

export function apiTypesText(types?: string[]) {
  return types?.join(' / ') || '未声明'
}

export function normalizePlanType(value?: string | null) {
  if (!value) {
    return ''
  }
  const text = value.trim()
  if (!text) {
    return ''
  }
  return text.toLowerCase()
}

export function planTypeText(value?: string | null) {
  return normalizePlanType(value) || '-'
}

export function splitPlanTypeInput(value?: string | null) {
  if (!value) {
    return []
  }

  const planTypes: string[] = []
  const seen = new Set<string>()
  for (const part of value.split(PLAN_TYPE_SPLIT_RE)) {
    const planType = normalizePlanType(part)
    if (!planType || seen.has(planType)) {
      continue
    }
    seen.add(planType)
    planTypes.push(planType)
  }
  return planTypes
}

export function joinPlanTypeInput(planTypes: string[]) {
  return splitPlanTypeInput(planTypes.join(',')).join(',')
}

export function settingsToForm(data?: Partial<SettingsSnapshot>): SettingsForm {
  return {
    allow_user_plan_type_header: Boolean(data?.allow_user_plan_type_header),
    global_proxy: data?.global_proxy || '',
    codex_proxy: data?.codex_proxy || '',
    codex_delete_free_accounts: Boolean(data?.codex_delete_free_accounts),
    codex_allow_user_plan_type_header: Boolean(data?.codex_allow_user_plan_type_header),
    codex_preferred_plan_types: data?.codex_preferred_plan_types?.trim() || '',
    refresh_before_seconds: String(data?.refresh_before_seconds ?? DEFAULT_SETTINGS_FORM.refresh_before_seconds),
    poll_interval_milliseconds: String(data?.poll_interval_milliseconds ?? DEFAULT_SETTINGS_FORM.poll_interval_milliseconds),
    quota_sync_interval_seconds: String(data?.quota_sync_interval_seconds ?? DEFAULT_SETTINGS_FORM.quota_sync_interval_seconds),
    throttle_base_seconds: String(data?.throttle_base_seconds ?? DEFAULT_SETTINGS_FORM.throttle_base_seconds),
    throttle_max_seconds: String(data?.throttle_max_seconds ?? DEFAULT_SETTINGS_FORM.throttle_max_seconds),
    logs_retention_seconds: String(data?.logs_retention_seconds ?? DEFAULT_SETTINGS_FORM.logs_retention_seconds),
    relay_max_retries: String(data?.relay_max_retries ?? DEFAULT_SETTINGS_FORM.relay_max_retries),
  }
}

export function settingsToPayload(form: SettingsForm): SettingsSnapshot {
  return {
    allow_user_plan_type_header: Boolean(form.allow_user_plan_type_header),
    global_proxy: form.global_proxy.trim(),
    codex_proxy: form.codex_proxy.trim(),
    codex_delete_free_accounts: Boolean(form.codex_delete_free_accounts),
    codex_allow_user_plan_type_header: Boolean(form.codex_allow_user_plan_type_header),
    codex_preferred_plan_types: form.codex_preferred_plan_types.trim(),
    refresh_before_seconds: Number(form.refresh_before_seconds),
    poll_interval_milliseconds: Number(form.poll_interval_milliseconds),
    quota_sync_interval_seconds: Number(form.quota_sync_interval_seconds),
    throttle_base_seconds: Number(form.throttle_base_seconds),
    throttle_max_seconds: Number(form.throttle_max_seconds),
    logs_retention_seconds: Number(form.logs_retention_seconds),
    relay_max_retries: Number(form.relay_max_retries),
  }
}
