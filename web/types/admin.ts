export type ThemeMode = 'light' | 'dark'
export type ThemePreference = 'system' | ThemeMode

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

export interface CredentialThrottleStatusOption {
  value: string
  label: string
  metric: string
}

export interface CredentialSortOption {
  value: string
  label: string
}

export interface CredentialSortCapabilities {
  models: CredentialSortOption[]
  metrics: CredentialSortOption[]
}

export interface PluginInfo {
  name: string
  label: string
  description: string
  handlers: string[]
  api_types: string[]
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
  credential_throttle_status_options?: CredentialThrottleStatusOption[]
  credential_sort?: CredentialSortCapabilities
  plugins?: PluginInfo[]
  models_total: number
  credentials_total: number
  credentials_enabled: number
}

export interface LogItem {
  handler: string
  credential_id: string
  status_code: number
  model: string
  api_type: string
  stream: boolean
  first_byte: number
  duration: number
  error: string
  created_at: string
}

export interface LogStatusCount {
  status_code: number
  total: number
}

export interface LogListSummary {
  total: number
  status_codes: LogStatusCount[]
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

export interface BuildInfo {
  version: string
  build_time: string
}

export interface StatusResponse {
  need_setup: boolean
  build_info: BuildInfo
}

export interface SettingsSnapshot {
  global_proxy: string
  quota_sync_interval_seconds: number
  score_refresh_interval_seconds: number
  throttle_base_seconds: number
  throttle_max_seconds: number
  relay_max_retries: number
  weighted_best_count: number
  content_affinity_max_entries: number

  import_concurrency: number
  logs_retention_seconds: number
  max_log_rows: number

  codex_proxy: string
  codex_preferred_plan_types: string
  codex_user_agent: string
  codex_enable_sticky_session: boolean

  gemini_proxy: string
  gemini_base_urls: string
  gemini_preferred_plan_types: string

  antigravity_proxy: string
  antigravity_preferred_plan_types: string
  antigravity_api_endpoint?: string
  antigravity_use_credits?: boolean
  antigravity_user_agent: string

  opencode_go_proxy: string
}

export interface SettingsForm {
  global_proxy: string
  quota_sync_interval_seconds: string
  score_refresh_interval_seconds: string
  throttle_base_seconds: string
  throttle_max_seconds: string
  relay_max_retries: string
  weighted_best_count: string
  content_affinity_max_entries: string

  import_concurrency: string
  logs_retention_seconds: string
  max_log_rows: string

  codex_proxy: string
  codex_preferred_plan_types: string
  codex_user_agent: string
  codex_enable_sticky_session: boolean

  gemini_proxy: string
  gemini_base_urls: string
  gemini_preferred_plan_types: string

  antigravity_proxy: string
  antigravity_preferred_plan_types: string
  antigravity_api_endpoint?: string
  antigravity_use_credits?: boolean
  antigravity_user_agent: string

  opencode_go_proxy: string
}

export interface CodexSchedulingMetric {
  available: boolean
  quota_5h: number
  quota_7d: number
  quota_1mo: number
  reset_5h: string
  reset_7d: string
  reset_1mo: string
  throttled_until?: string
  score: number
  weight: number
}

export interface GeminiSchedulingMetric {
  available: boolean
  quota: number
  reset: string
  throttled_until?: string
  score: number
  weight: number
}

export interface AntigravityCreditsMetric {
  available: boolean
  amount: number
  types: string[]
}

export interface CodexItem {
  handler: 'codex'
  id: string
  status: string[]
  expired: string
  synced_at: string
  plan_type: string | null
  reason: string
  default: CodexSchedulingMetric
  spark: CodexSchedulingMetric
  reset_credits_count?: number
}

export interface CodexRateLimitResetCredit {
  title?: string
  expires_at?: string | null
  status: string
}

export interface CodexRateLimitResetCredits {
  credits: CodexRateLimitResetCredit[]
  available_count: number
}

export interface GeminiCredentialItem {
  handler: 'gemini'
  id: string
  status: string[]
  email: string
  project_id: string
  plan_type: string
  expired: string
  reason: string
  synced_at: string
  pro: GeminiSchedulingMetric
  flash: GeminiSchedulingMetric
  flashlite: GeminiSchedulingMetric
}

export interface AntigravityCredentialItem {
  handler: 'antigravity'
  id: string
  status: string[]
  email: string
  project_id: string
  plan_type: string
  expired: string
  reason: string
  synced_at: string
  claude: GeminiSchedulingMetric
  pro: GeminiSchedulingMetric
  flash: GeminiSchedulingMetric
  flashlite: GeminiSchedulingMetric
  tab: GeminiSchedulingMetric
  image: GeminiSchedulingMetric
  credits: AntigravityCreditsMetric
}

export interface OpenCodeGoCredentialItem {
  handler: 'opencode-go'
  id: string
  email: string
  status: string[]
  reason: string
  workspace_id: string
  synced_at: string
  quota: CodexSchedulingMetric
  rewards_count: number
}

export interface OpenCodeGoReferralReward {
  id: string
  source: string
  status: string
  email?: string
  amount_cents: number
  created_at: string
}

export interface OpenCodeGoReferralRewards {
  rewards: OpenCodeGoReferralReward[]
  total_cents: number
}

export interface GenericCredentialItem {
  handler: string
  id: string
  status: string[]
  plan_type?: string | null
  expired?: string
  synced_at?: string
  reason?: string
  [key: string]: unknown
}

export type CredentialItem = CodexItem | GeminiCredentialItem | AntigravityCredentialItem | OpenCodeGoCredentialItem | GenericCredentialItem

export type CredentialHandlerKey = string

export type OAuthProvider = 'codex' | 'gemini' | 'antigravity'

export interface OAuthStartResponse {
  provider: OAuthProvider | string
  state: string
  authorize_url: string
}

export interface OAuthCallbackResponse {
  provider: OAuthProvider | string
  id: string
}

export interface ModelScheduling {
  content_affinity: boolean
  fill_first: boolean
}

export interface ModelInput extends ModelScheduling {
  origin: string
  handler: string
  plan_types: string
  plugin: string
  extra: Record<string, unknown>
}

export interface CreateModelInput extends ModelInput {
  alias: string
}

export interface BatchModelInput extends Partial<ModelScheduling> {
  aliases: string[]
  handler: string
  plan_types: string
  plugin: string
  extra: Record<string, unknown>
}

export interface ModelItem extends ModelScheduling {
  alias: string
  origin: string
  handler: string
  plan_types: string
  plugin: string
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
  id: number
  text: string
  tone: UiTone
}

export interface PaginatedResponse<T> {
  total: number
  page: number
  page_size: number
  plan_types?: string[]
  data: T[]
}

export interface LogListResponse extends PaginatedResponse<LogItem> {
  summary: LogListSummary
}

export interface BatchOperationError {
  input: string
  error: string
}

export type ImportJobStatus = 'running' | 'completed'

export interface ImportJobStartResponse {
  id: string
  handler: string
  status: ImportJobStatus
  total: number
}

export interface ImportJobSnapshot extends ImportJobStartResponse {
  processed: number
  succeeded: number
  error: Array<Record<string, string>>
  created_at: string
  updated_at: string
}

export interface ImportJobListResponse {
  data: ImportJobSnapshot[]
}

export interface BatchStatusResponse {
  updated: string[]
  errors: BatchOperationError[]
}

export interface BatchModelUpdateResponse {
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
