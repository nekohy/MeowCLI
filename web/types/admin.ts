export type ThemeMode = 'light' | 'dark'

export type UiTone =
  | 'neutral'
  | 'success'
  | 'danger'
  | 'warning'
  | 'accent'
  | 'muted'
  | 'secondary'

export type NavKey =
  | 'dashboard'
  | 'settings'
  | 'credentials'
  | 'models'
  | 'logs'
  | 'keys'

export interface NavItem {
  key: NavKey
  to: string
  icon: string
  label: string
  eyebrow: string
  description?: string
}

export interface CredentialField {
  key: string
  label: string
  kind: string
  placeholder?: string
  help_text?: string
  optional?: boolean
  preferred?: boolean
}

export interface HandlerOverview {
  key: string
  label: string
  status: string
  supported_api_types: string[]
  plan_list?: string[]
  supports_credentials: boolean
  credential_endpoint?: string
  credential_fields?: CredentialField[]
  credential_status_options?: string[]
  models_total: number
  credentials_total: number
  credentials_enabled: number
}

export interface LogItem {
  handler: string
  credential_id: string
  text: string
  status_code: number
  created_at: string
}

export interface OverviewSummary {
  credentials_enabled: number
  credentials_total: number
  models_total: number
  logs_total: number
  auth_keys_total: number
}

export interface OverviewResponse {
  summary: OverviewSummary
  handlers: HandlerOverview[]
  recent_logs: LogItem[]
}

export interface SettingsSnapshot {
  allow_user_plan_type_header: boolean
  global_proxy: string
  codex_proxy: string
  codex_allow_user_plan_type_header: boolean
  codex_preferred_plan_types: string
  refresh_before_seconds: number
  poll_interval_milliseconds: number
  quota_sync_interval_seconds: number
  throttle_base_seconds: number
  throttle_max_seconds: number
  logs_retention_seconds: number
  relay_max_retries: number
}

export interface SettingsForm {
  allow_user_plan_type_header: boolean
  global_proxy: string
  codex_proxy: string
  codex_allow_user_plan_type_header: boolean
  codex_preferred_plan_types: string
  refresh_before_seconds: string
  poll_interval_milliseconds: string
  quota_sync_interval_seconds: string
  throttle_base_seconds: string
  throttle_max_seconds: string
  logs_retention_seconds: string
  relay_max_retries: string
}

export interface CodexItem {
  id: string
  status: string
  expired: string
  synced_at: string
  throttled_until: string
  quota_5h: number
  quota_7d: number
  reset_5h: string
  reset_7d: string
  plan_type: string | null
  plan_expired: string
  reason: string
}

export interface ModelItem {
  alias: string
  origin: string
  handler: string
  extra: Record<string, unknown>
}

export interface AuthKeyItem {
  key: string
  role: string
  note: string
  created_at: string
}

export interface SetupState {
  key: string
  note: string
}

export interface SetupResult {
  key: string
  role: string
  note: string
  created_at: string
}

export interface ToastMessage {
  text: string
  tone: UiTone
}

export interface PaginatedResponse<T> {
  total: number
  page: number
  page_size: number
  data: T[]
}

export interface BatchOperationError {
  input: string
  error: string
}

export interface BatchCreateResponse {
  created: Array<{ id: string }>
  errors: BatchOperationError[]
}

export interface BatchStatusResponse {
  updated: string[]
  errors: BatchOperationError[]
}

export interface BatchDeleteResponse {
  deleted: string[]
  errors: BatchOperationError[]
}

export interface CreateAuthKeyResponse {
  key: string
  role: string
  note: string
  created_at: string
}
