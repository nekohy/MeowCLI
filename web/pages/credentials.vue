<script setup lang="ts">
import { adminApi } from '~/composables/useAdminApi'
import { newlyCompletedImportJobs } from '~/composables/useImportJobs'
import {
  CREDENTIAL_THROTTLE_STATUS_ALL,
  credentialStatusQueryValue,
  type CredentialStatusFilter,
} from '~/lib/credentialStatus'
import {
  CREDENTIAL_PAGE_SIZE_OPTIONS,
  codexCredentialAccountID,
  codexCredentialEmail,
  formatCreditsAmount,
  formatPercent,
  formatTime,
  formatUsdFromCents,
  isPastTime,
  isZeroTime,
  normalizePlanType,
  planTypeText,
  statusText,
} from '~/lib/admin'
import type {
  CredentialHandlerKey,
  AntigravityCredentialItem,
  CodexItem,
  CodexRateLimitResetCredits,
  CredentialItem,
  CredentialSortOption,
  CredentialThrottleStatusOption,
  GeminiCredentialItem,
  OpenCodeGoCredentialItem,
  OpenCodeGoReferralReward,
  OpenCodeGoReferralRewards,
  UiTone,
} from '~/types/admin'

definePageMeta({
  navKey: 'credentials',
})

const admin = useAdminApp()
const confirm = useConfirmDialog()
const importJobs = useImportJobs()

const rows = ref<CredentialItem[]>([])
const rowsHandlerKey = ref('')
const planTypes = ref<string[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(6)
const loading = ref(false)
const { input: searchInput, query: searchQuery } = useDebouncedRef()
const statusFilter = ref<CredentialStatusFilter[]>([])
const throttleFiltersExpanded = ref(false)
const planFilter = ref('all')
const sortMetric = ref('')
const sortModel = ref('')
const sortOrder = ref<'desc' | 'asc'>('desc')
const selectedIds = ref<string[]>([])
const actionBusy = ref(false)

const importOpen = ref(false)
const importTokens = ref('')
const importError = ref('')
const oauthOpen = ref(false)
const oauthBusy = ref(false)
const oauthCodeInput = ref('')
const oauthError = ref('')
const oauthFlow = ref<{ provider: string; state: string; authorizeUrl: string } | null>(null)
const creditsDialogItem = ref<AntigravityCredentialItem | null>(null)
const codexResetDialogItem = ref<CodexItem | null>(null)
const codexResetCredits = ref<CodexRateLimitResetCredits | null>(null)
const codexResetLoading = ref(false)
const codexResetConsuming = ref(false)
const codexResetError = ref('')
const openCodeGoRewardsDialogItem = ref<OpenCodeGoCredentialItem | null>(null)
const openCodeGoRewards = ref<OpenCodeGoReferralRewards | null>(null)
const openCodeGoRewardsLoading = ref(false)
const openCodeGoRewardApplying = ref('')
const openCodeGoRewardsError = ref('')

const credentialHandlerKey = computed<CredentialHandlerKey>(() => admin.activeHandler.value?.key || '')
const credentialEndpoint = computed(() => admin.activeHandler.value?.credential_endpoint || '')
const activeHandlerLabel = computed(() => admin.activeHandler.value?.label || '当前处理器')
const activeCredentialField = computed(() => (
  admin.activeHandler.value?.credential_fields?.find((field) => field.preferred)
  || admin.activeHandler.value?.credential_fields?.[0]
  || null
))
const isCodexHandler = computed(() => credentialHandlerKey.value === 'codex')
const isGeminiHandler = computed(() => credentialHandlerKey.value === 'gemini')
const isAntigravityHandler = computed(() => credentialHandlerKey.value === 'antigravity')
const isOpenCodeGoHandler = computed(() => credentialHandlerKey.value === 'opencode-go')
const supportsOAuth = computed(() => isCodexHandler.value || isGeminiHandler.value || isAntigravityHandler.value)

const codexRows = computed(() => rows.value.filter(isCodexItem))
const geminiRows = computed(() => rows.value.filter(isGeminiItem))
const antigravityRows = computed(() => rows.value.filter(isAntigravityItem))
const openCodeGoRows = computed(() => rows.value.filter(isOpenCodeGoItem))
const genericRows = computed(() => rows.value.filter((item) => !isCodexItem(item) && !isGeminiItem(item) && !isAntigravityItem(item) && !isOpenCodeGoItem(item)))
const rowsMatchActiveHandler = computed(() => rowsHandlerKey.value === credentialHandlerKey.value)
const showHandlerLoadingState = computed(() => (
  Boolean(admin.activeHandler.value?.supports_credentials)
  && (!rowsMatchActiveHandler.value || (loading.value && !rows.value.length))
))

const importLines = computed(() => (
  importTokens.value
    .split('\n')
    .map((line) => line.trim())
    .filter(Boolean)
))
const importInputLabel = computed(() => activeCredentialField.value?.label || '凭据列表')
const importInputPlaceholder = computed(() => activeCredentialField.value?.placeholder || '每行填写一个凭据')
const oauthModalTitle = computed(() => `${activeHandlerLabel.value} OAuth`)
const oauthCallbackPlaceholder = '粘贴回调链接'
const oauthSubmitDisabled = computed(() => !oauthFlow.value || !oauthCodeInput.value.trim())
const creditsDialogOpen = computed(() => Boolean(creditsDialogItem.value))
const codexResetDialogOpen = computed(() => Boolean(codexResetDialogItem.value))
const openCodeGoRewardsDialogOpen = computed(() => Boolean(openCodeGoRewardsDialogItem.value))
const creditsDialogTypes = computed(() => (
  creditsDialogItem.value ? antigravityCreditTypes(creditsDialogItem.value) : []
))

function credentialPlanType(item: CredentialItem) {
  return (item as { plan_type?: string | null }).plan_type || ''
}

const availablePlanTypes = computed(() => {
  const sourcePlanTypes = planTypes.value.length
    ? planTypes.value
    : rows.value.map(credentialPlanType)
  const orderedPlanTypes: string[] = []
  const seen = new Set<string>()
  sourcePlanTypes.forEach((rawPlanType) => {
    const planType = normalizePlanType(rawPlanType)
    if (!planType || seen.has(planType)) {
      return
    }
    seen.add(planType)
    orderedPlanTypes.push(planType)
  })

  const preferredOrder = (admin.activeHandler.value?.plan_list || [])
    .map((plan) => normalizePlanType(plan))
    .filter((planType): planType is string => Boolean(planType))

  const presentPlanTypes = new Set(orderedPlanTypes)
  const sortedPlanTypes = preferredOrder.filter((planType) => presentPlanTypes.has(planType))
  orderedPlanTypes.forEach((planType) => {
    if (!sortedPlanTypes.includes(planType)) {
      sortedPlanTypes.push(planType)
    }
  })

  return ['all', ...sortedPlanTypes]
})

watch(availablePlanTypes, (planTypes) => {
  if (!planTypes.includes(planFilter.value)) {
    planFilter.value = 'all'
  }
})

const baseStatusFilters = computed(() => (
  ['enabled', 'disabled'].map((status) => ({ value: status, label: statusText(status) }))
))
const throttleStatusFilters = computed(() => (
  throttleTiersForHandler(credentialHandlerKey.value)
    .map((tier) => ({ value: tier.status, label: statusText(tier.status) }))
))
const hasThrottleStatusFilters = computed(() => throttleStatusFilters.value.length > 0)
const selectedThrottleStatusFilters = computed(() => {
  const throttleValues = new Set(throttleStatusFilters.value.map((option) => option.value))
  return statusFilter.value.filter((status) => throttleValues.has(status))
})
const throttleFilterActive = computed(() => (
  statusFilter.value.includes(CREDENTIAL_THROTTLE_STATUS_ALL)
  || selectedThrottleStatusFilters.value.length > 0
))
const allStatusFilterActive = computed(() => statusFilter.value.length === 0)

type ThrottleTierDetail = {
  status: string
  label: string
  metric: string
}

function throttleTierDetailFromOption(option: CredentialThrottleStatusOption): ThrottleTierDetail {
  return {
    status: option.value,
    label: option.label,
    metric: option.metric,
  }
}

const defaultSortMetricOption = { title: '默认', value: '' }

function credentialSortSelectOptions(options?: CredentialSortOption[]) {
  return (options || []).map((option) => ({ title: option.label, value: option.value }))
}

const sortOrderOptions = [
  { title: '降序', value: 'desc' },
  { title: '升序', value: 'asc' },
]

const credentialSortMetricOptions = computed(() => [
  defaultSortMetricOption,
  ...credentialSortSelectOptions(admin.activeHandler.value?.credential_sort?.metrics),
])

const credentialSortModelOptions = computed(() => (
  credentialSortSelectOptions(admin.activeHandler.value?.credential_sort?.models)
))

watch(
  credentialSortModelOptions,
  (options) => {
    if (!options.some((option) => option.value === sortModel.value)) {
      sortModel.value = options[0]?.value || ''
    }
  },
  { immediate: true },
)

const hasActiveFilters = computed(() => (
  Boolean(searchInput.value.trim())
  || statusFilter.value.length > 0
  || planFilter.value !== 'all'
))
const emptyStateTitle = computed(() => {
  if (hasActiveFilters.value) {
    return '当前条件下没有匹配的凭据'
  }
  return `还没有可管理的 ${activeHandlerLabel.value} 凭据`
})
const emptyStateDescription = computed(() => {
  if (hasActiveFilters.value) {
    return isGeminiHandler.value
      ? '调整搜索、状态或套餐筛选，或者先新增一组 Gemini CLI 凭据'
      : '调整搜索、状态或套餐筛选，或者先导入新凭据'
  }
  return activeCredentialField.value?.help_text || '先导入一批凭据，系统才会开始调度和额度同步'
})
const selectedSet = computed(() => new Set(selectedIds.value))
const allVisibleSelected = computed(() => (
  rows.value.length > 0 && rows.value.every((item) => selectedSet.value.has(item.id))
))
const maxPage = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)))
const pageSizeOptions = CREDENTIAL_PAGE_SIZE_OPTIONS
const importDescription = computed(() => (
  activeCredentialField.value?.help_text || '一行一个凭据，保存后会纳入当前处理器调度'
))

const credentialLoadGuard = useStaleGuard()

function isCodexItem(item: CredentialItem): item is CodexItem {
  return item.handler === 'codex'
}

function isGeminiItem(item: CredentialItem): item is GeminiCredentialItem {
  return item.handler === 'gemini'
}

function isAntigravityItem(item: CredentialItem): item is AntigravityCredentialItem {
  return item.handler === 'antigravity'
}

function isOpenCodeGoItem(item: CredentialItem): item is OpenCodeGoCredentialItem {
  return item.handler === 'opencode-go'
}

function genericDetailEntries(item: CredentialItem) {
  const raw = item as Record<string, unknown>
  return [
    { label: '凭据 ID', value: item.id },
    { label: '邮箱', value: typeof raw.email === 'string' ? raw.email : '' },
    { label: 'Project ID', value: typeof raw.project_id === 'string' ? raw.project_id : '' },
    { label: '套餐类型', value: planTypeText(credentialPlanType(item)) },
    { label: 'AT到期', value: raw.expired ? formatTime(String(raw.expired)) : '-' },
    { label: '最近同步', value: item.synced_at ? formatTime(String(item.synced_at)) : '-' },
  ].filter((entry) => entry.value && entry.value !== 'unknown')
}

// gemini/antigravity 分支共享的项目详情块
function projectDetailEntries(item: GeminiCredentialItem | AntigravityCredentialItem) {
  return [
    { label: '项目 ID', value: item.project_id || '-' },
    { label: 'AT到期', value: formatTime(item.expired) },
    { label: '最近同步', value: item.synced_at ? formatTime(item.synced_at) : '-' },
  ]
}

function throttleTiersForHandler(handler: string) {
  const overview = admin.handlers.value.find((item) => item.key === handler)
  return (overview?.credential_throttle_status_options || []).map(throttleTierDetailFromOption)
}

function clearStatusFilters() {
  statusFilter.value = []
}

function toggleBaseStatusFilter(status: string) {
  const baseValues = new Set(baseStatusFilters.value.map((option) => option.value))
  if (statusFilter.value.includes(status)) {
    statusFilter.value = statusFilter.value.filter((value) => value !== status)
    return
  }
  statusFilter.value = [
    ...statusFilter.value.filter((value) => !baseValues.has(value)),
    status,
  ]
}

function toggleThrottleAllFilter() {
  const throttleValues = new Set(throttleStatusFilters.value.map((option) => option.value))
  if (throttleFilterActive.value) {
    statusFilter.value = statusFilter.value.filter((status) => (
      status !== CREDENTIAL_THROTTLE_STATUS_ALL && !throttleValues.has(status)
    ))
    return
  }
  statusFilter.value = [...statusFilter.value, CREDENTIAL_THROTTLE_STATUS_ALL]
}

function toggleThrottleTierFilter(status: string) {
  statusFilter.value = statusFilter.value.includes(status)
    ? statusFilter.value.filter((value) => value !== status)
    : [
        ...statusFilter.value.filter((value) => value !== CREDENTIAL_THROTTLE_STATUS_ALL),
        status,
      ]
}

function toggleThrottleFiltersExpanded() {
  throttleFiltersExpanded.value = !throttleFiltersExpanded.value
}

function closeImportModal() {
  importOpen.value = false
  importTokens.value = ''
  importError.value = ''
}

function closeOAuthModal(force = false) {
  if (oauthBusy.value && !force) {
    return
  }
  oauthOpen.value = false
  oauthCodeInput.value = ''
  oauthError.value = ''
  oauthFlow.value = null
}

function parseOAuthCallbackInput(value: string) {
  const trimmed = value.trim()
  if (!trimmed) {
    return { code: '', state: '' }
  }

  const parseParams = (params: URLSearchParams) => ({
    code: (params.get('code') || '').trim(),
    state: (params.get('state') || '').trim(),
  })

  try {
    const url = new URL(trimmed)
    const searchParsed = parseParams(url.searchParams)
    if (searchParsed.code) {
      return searchParsed
    }
    const hashParsed = parseParams(new URLSearchParams(url.hash.replace(/^#/, '')))
    if (hashParsed.code) {
      return hashParsed
    }
  } catch {
    // Fall through and treat the input as a query string or raw code.
  }

  const queryStart = trimmed.indexOf('?')
  const queryLike = queryStart >= 0 ? trimmed.slice(queryStart + 1) : trimmed
  if (queryLike.includes('=')) {
    const parsed = parseParams(new URLSearchParams(queryLike.replace(/^#/, '')))
    if (parsed.code) {
      return parsed
    }
  }

  return { code: trimmed, state: '' }
}

async function startOAuthLogin() {
  const provider = credentialHandlerKey.value
  if (!admin.token.value || !provider || !supportsOAuth.value) {
    return
  }

  oauthBusy.value = true
  oauthError.value = ''
  oauthCodeInput.value = ''
  oauthFlow.value = null
  oauthOpen.value = true

  try {
    const flow = await adminApi.startOAuth(admin.token.value, provider)
    if (!flow.state || !flow.authorize_url) {
      throw new Error('OAuth 授权链接无效')
    }

    oauthFlow.value = {
      provider: flow.provider || provider,
      state: flow.state,
      authorizeUrl: flow.authorize_url,
    }

  } catch (error) {
    oauthError.value = error instanceof Error ? error.message : '启动 OAuth 登录失败'
  } finally {
    oauthBusy.value = false
  }
}

function reopenOAuthAuthorizeUrl() {
  if (!import.meta.client || !oauthFlow.value?.authorizeUrl) {
    return
  }
  window.open(oauthFlow.value.authorizeUrl, '_blank', 'noopener,noreferrer')
}

async function completeOAuthLogin() {
  const flow = oauthFlow.value
  if (!flow) {
    oauthError.value = '请先打开 OAuth 授权链接'
    return
  }

  const parsed = parseOAuthCallbackInput(oauthCodeInput.value)
  if (!parsed.code) {
    oauthError.value = '请粘贴回调地址或授权 code'
    return
  }
  if (parsed.state && parsed.state !== flow.state) {
    oauthError.value = '回调 state 与当前授权流程不一致，请重新发起 OAuth 登录'
    return
  }

  oauthBusy.value = true
  oauthError.value = ''
  try {
    const result = await adminApi.completeOAuth(admin.token.value, flow.provider, {
      state: flow.state,
      code: parsed.code,
    })
    closeOAuthModal(true)
    admin.notify(`OAuth 凭据已添加：${result.id}`, 'success')
    await admin.refreshAfterMutation(() => loadCredentials(1, pageSize.value))
  } catch (error) {
    oauthError.value = error instanceof Error ? error.message : 'OAuth 回调兑换失败'
  } finally {
    oauthBusy.value = false
  }
}

function toggleSelectAll() {
  if (allVisibleSelected.value) {
    selectedIds.value = []
    return
  }
  selectedIds.value = rows.value.map((item) => item.id)
}

function toggleSelectOne(id: string) {
  selectedIds.value = selectedSet.value.has(id)
    ? selectedIds.value.filter((value) => value !== id)
    : [...selectedIds.value, id]
}

type CodexQuotaKey = 'quota_5h' | 'quota_7d' | 'quota_1mo'
type CodexResetKey = 'reset_5h' | 'reset_7d' | 'reset_1mo'

function hasCodexQuotaWindow(metric: CodexItem['default'], resetKey: CodexResetKey) {
  return !isZeroTime(metric[resetKey])
}

function codexQuotaPercentValue(metric: CodexItem['default'], quotaKey: CodexQuotaKey, resetKey: CodexResetKey) {
  if (!hasCodexQuotaWindow(metric, resetKey)) return null
  return Math.max(0, Math.min(100, Math.round((metric[quotaKey] || 0) * 100)))
}

function geminiQuotaPercentValue(metric: GeminiCredentialItem['pro']) {
  if (metric.quota === 1 && isZeroTime(metric.reset)) {
    return null
  }
  return Math.max(0, Math.min(100, Math.round((metric.quota || 0) * 100)))
}

// 配额语义色（只染进度条，数值文字保持墨色）：充裕 success / 偏紧 warning / 将尽 danger / 退避 accent
function quotaTone(percent: number | null): UiTone {
  if (percent === null) {
    return 'secondary'
  }
  if (percent >= 65) {
    return 'success'
  }
  if (percent >= 30) {
    return 'warning'
  }
  return 'danger'
}

function activeThrottleUntil(value?: string | null) {
  return value && !isPastTime(value) ? value : ''
}

function quotaCaption(reset: string, throttledUntil?: string) {
  const parts = [`重置 ${formatTime(reset)}`]
  if (throttledUntil) {
    parts.push(`退避至 ${formatTime(throttledUntil)}`)
  }
  return parts
}

function renderCodexQuotaValue(metric: CodexItem['default'], quotaKey: CodexQuotaKey, resetKey: CodexResetKey) {
  if (!hasCodexQuotaWindow(metric, resetKey)) return ''
  return formatPercent(metric[quotaKey])
}

function renderGeminiQuotaValue(metric: GeminiCredentialItem['pro']) {
  if (metric.quota === 1 && isZeroTime(metric.reset)) {
    return '不适用'
  }
  return formatPercent(metric.quota)
}

function resolveQuotaTone(percent: number | null, throttledUntil: string): UiTone {
  return throttledUntil ? 'accent' : quotaTone(percent)
}

interface QuotaMetricLike {
  throttled_until?: string | null
  score: number
  weight: number
}

// codex/gemini/opencode-go 共用同一配额卡片结构,仅 percent/value/reset 的来源不同
function buildQuotaCard(
  label: string,
  metric: QuotaMetricLike,
  percent: number | null,
  reset: string,
  value: string,
) {
  const throttledUntil = activeThrottleUntil(metric.throttled_until)
  return {
    label,
    score: quotaScoreLabel(metric),
    percent,
    tone: resolveQuotaTone(percent, throttledUntil),
    value,
    caption: quotaCaption(reset, throttledUntil),
  }
}

function codexQuotaCard(label: string, metric: CodexItem['default'], quotaKey: CodexQuotaKey, resetKey: CodexResetKey) {
  if (!hasCodexQuotaWindow(metric, resetKey)) return null
  return buildQuotaCard(
    label,
    metric,
    codexQuotaPercentValue(metric, quotaKey, resetKey),
    metric[resetKey],
    renderCodexQuotaValue(metric, quotaKey, resetKey),
  )
}

function codexQuotaCards(item: CodexItem) {
  const cards = [
    codexQuotaCard('5 小时额度', item.default, 'quota_5h', 'reset_5h'),
    codexQuotaCard('7 天额度', item.default, 'quota_7d', 'reset_7d'),
    codexQuotaCard('月额度', item.default, 'quota_1mo', 'reset_1mo'),
  ]
  if (isSparkAvailable(item)) {
    cards.push(
      codexQuotaCard('Spark 5h', item.spark, 'quota_5h', 'reset_5h'),
      codexQuotaCard('Spark 7d', item.spark, 'quota_7d', 'reset_7d'),
      codexQuotaCard('Spark 月额度', item.spark, 'quota_1mo', 'reset_1mo'),
    )
  }
  return cards.filter((card): card is NonNullable<typeof card> => card !== null)
}

function openCodeGoQuotaCards(item: OpenCodeGoCredentialItem) {
  return [
    codexQuotaCard('5 小时额度', item.quota, 'quota_5h', 'reset_5h'),
    codexQuotaCard('7 天额度', item.quota, 'quota_7d', 'reset_7d'),
    codexQuotaCard('月额度', item.quota, 'quota_1mo', 'reset_1mo'),
  ].filter((card): card is NonNullable<typeof card> => card !== null)
}

function geminiQuotaCard(label: string, metric: GeminiCredentialItem['pro']) {
  return buildQuotaCard(
    label,
    metric,
    geminiQuotaPercentValue(metric),
    metric.reset,
    renderGeminiQuotaValue(metric),
  )
}

function geminiQuotaCards(item: GeminiCredentialItem) {
  return [
    geminiQuotaCard('Pro 额度', item.pro),
    geminiQuotaCard('Flash 额度', item.flash),
    geminiQuotaCard('Lite 额度', item.flashlite),
  ]
}

function antigravityQuotaCards(item: AntigravityCredentialItem) {
  return [
    geminiQuotaCard('Claude 额度', item.claude),
    geminiQuotaCard('Pro 额度', item.pro),
    geminiQuotaCard('Flash 额度', item.flash),
    geminiQuotaCard('Lite 额度', item.flashlite),
    geminiQuotaCard('Tab 额度', item.tab),
    geminiQuotaCard('Image 额度', item.image),
  ]
}

function isSparkAvailable(item: CodexItem) {
  return item.spark.available
}

function formatScore(value: number) {
  if (typeof value !== 'number' || Number.isNaN(value)) {
    return '-'
  }
  return value.toFixed(2)
}

function errorRateFromWeight(metric: { weight: number }) {
  return Math.max(0, Math.min(1, 1 - metric.weight))
}

function quotaScoreLabel(metric: { score: number; weight: number }) {
  return `Score ${formatScore(metric.score)}(${formatPercent(errorRateFromWeight(metric))})`
}

function antigravityCreditTypes(item: AntigravityCredentialItem) {
  return Array.isArray(item.credits?.types) ? item.credits.types.filter(Boolean) : []
}

function openCreditsDialog(item: AntigravityCredentialItem) {
  creditsDialogItem.value = item
}

function closeCreditsDialog() {
  creditsDialogItem.value = null
}

function codexResetCreditsCount(item: CodexItem): number {
  return typeof item.reset_credits_count === 'number' ? item.reset_credits_count : 0
}

function canOpenCodexReset(item: CodexItem) {
  return codexResetCreditsCount(item) > 0
}

function codexResetCreditStatusLabel(status: string) {
  return (status || '').toLowerCase() === 'available' ? '可用' : (status || '-')
}

const codexResetGuard = useStaleGuard()

async function openCodexResetDialog(item: CodexItem) {
  const requestToken = codexResetGuard.next()
  codexResetDialogItem.value = item
  codexResetCredits.value = null
  codexResetError.value = ''
  codexResetLoading.value = true
  try {
    const credits = await adminApi.fetchCodexResetCredits(admin.token.value, item.id)
    // 弹窗已切换目标/已关闭时,晚到的响应直接丢弃
    if (codexResetGuard.isStale(requestToken) || codexResetDialogItem.value !== item) {
      return
    }
    codexResetCredits.value = credits
  } catch (error) {
    if (codexResetGuard.isStale(requestToken) || codexResetDialogItem.value !== item) {
      return
    }
    codexResetError.value = error instanceof Error ? error.message : '加载重置次数失败'
  } finally {
    if (!codexResetGuard.isStale(requestToken)) {
      codexResetLoading.value = false
    }
  }
}

function closeCodexResetDialog() {
  // 使 in-flight 请求立即失效
  codexResetGuard.invalidate()
  codexResetDialogItem.value = null
  codexResetCredits.value = null
  codexResetError.value = ''
}

function consumeCodexReset() {
  const item = codexResetDialogItem.value
  if (!item) return
  confirm.show({
    title: '使用重置次数',
    message: '确认消耗一次额度重置吗？此操作将重置该账号的额度窗口。',
    confirmText: '确认重置',
    confirmVariant: 'secondary',
    action: async () => {
      codexResetConsuming.value = true
      try {
        await adminApi.consumeCodexResetCredit(admin.token.value, item.id)
        admin.notify('已使用一次重置', 'success')
        closeCodexResetDialog()
        await loadCredentials(page.value, pageSize.value)
      } catch (error) {
        admin.notifyError(error, '重置失败')
      } finally {
        codexResetConsuming.value = false
      }
    },
  })
}

function openCodeGoRewardDescription(reward: OpenCodeGoReferralReward) {
  if (reward.source === 'invitee') {
    return reward.email ? `受邀加入（${reward.email}）` : '受邀加入奖励'
  }
  return reward.email ? `邀请 ${reward.email}` : '邀请奖励'
}

const openCodeGoRewardsGuard = useStaleGuard()

async function openOpenCodeGoRewardsDialog(item: OpenCodeGoCredentialItem) {
  const requestToken = openCodeGoRewardsGuard.next()
  openCodeGoRewardsDialogItem.value = item
  openCodeGoRewards.value = null
  openCodeGoRewardsError.value = ''
  openCodeGoRewardsLoading.value = true
  try {
    const rewards = await adminApi.fetchOpenCodeGoReferralRewards(admin.token.value, item.id)
    // 弹窗已切换目标/已关闭时,晚到的响应直接丢弃
    if (openCodeGoRewardsGuard.isStale(requestToken) || openCodeGoRewardsDialogItem.value !== item) {
      return
    }
    openCodeGoRewards.value = rewards
  } catch (error) {
    if (openCodeGoRewardsGuard.isStale(requestToken) || openCodeGoRewardsDialogItem.value !== item) {
      return
    }
    openCodeGoRewardsError.value = error instanceof Error ? error.message : '加载可用奖励失败'
  } finally {
    if (!openCodeGoRewardsGuard.isStale(requestToken)) {
      openCodeGoRewardsLoading.value = false
    }
  }
}

function closeOpenCodeGoRewardsDialog() {
  if (openCodeGoRewardApplying.value) return
  // 使 in-flight 请求立即失效
  openCodeGoRewardsGuard.invalidate()
  openCodeGoRewardsDialogItem.value = null
  openCodeGoRewards.value = null
  openCodeGoRewardsError.value = ''
}

function applyOpenCodeGoReward(reward: OpenCodeGoReferralReward) {
  const item = openCodeGoRewardsDialogItem.value
  if (!item) return
  confirm.show({
    title: '增加余额',
    message: `确认应用 ${formatUsdFromCents(reward.amount_cents)} 奖励吗？官方会将奖励抵扣到当前 OpenCode Go 用量窗口。`,
    confirmText: '确认增加',
    confirmVariant: 'secondary',
    action: async () => {
      openCodeGoRewardApplying.value = reward.id
      try {
        await adminApi.applyOpenCodeGoReferralReward(admin.token.value, item.id, reward.id)
        admin.notify('OpenCode Go 奖励已应用', 'success')
        openCodeGoRewardApplying.value = ''
        closeOpenCodeGoRewardsDialog()
        await loadCredentials(page.value, pageSize.value)
      } catch (error) {
        admin.notifyError(error, '增加余额失败')
      } finally {
        openCodeGoRewardApplying.value = ''
      }
    },
  })
}

function currentQueryOptions(nextPage = page.value, nextPageSize = pageSize.value) {
  const search = searchQuery.value.trim()
  return {
    page: nextPage,
    pageSize: nextPageSize,
    search: search || undefined,
    status: credentialStatusQueryValue(statusFilter.value),
    planType: planFilter.value !== 'all' ? planFilter.value : undefined,
    sortMetric: sortMetric.value || undefined,
    sortModel: sortMetric.value ? sortModel.value || undefined : undefined,
    sortOrder: sortOrder.value,
  }
}

async function loadCredentials(nextPage = page.value, nextPageSize = pageSize.value) {
  const requestToken = credentialLoadGuard.next()
  const handlerKey = credentialHandlerKey.value
  const endpoint = credentialEndpoint.value
  const supportsCredentials = Boolean(admin.activeHandler.value?.supports_credentials)

  if (!admin.token.value || !handlerKey || !supportsCredentials) {
    rows.value = []
    rowsHandlerKey.value = handlerKey
    planTypes.value = []
    total.value = 0
    page.value = 1
    selectedIds.value = []
    loading.value = false
    return
  }

  loading.value = true
  try {
    const data = await adminApi.queryCredentials(admin.token.value, endpoint, currentQueryOptions(nextPage, nextPageSize))
    if (credentialLoadGuard.isStale(requestToken)) {
      return
    }
    rows.value = data.data
    rowsHandlerKey.value = handlerKey
    planTypes.value = data.plan_types || []
    total.value = data.total
    page.value = data.page
    pageSize.value = data.page_size
    selectedIds.value = []
  } catch (error) {
    if (!credentialLoadGuard.isStale(requestToken)) {
      rows.value = []
      rowsHandlerKey.value = handlerKey
      planTypes.value = []
      total.value = 0
      selectedIds.value = []
      admin.notifyError(error, '加载凭据失败')
    }
  } finally {
    if (!credentialLoadGuard.isStale(requestToken)) {
      loading.value = false
    }
  }
}

async function createCredential() {
  actionBusy.value = true
  importError.value = ''

  try {
    if (importLines.value.length === 0) {
      importError.value = '请至少输入一行令牌'
      return
    }

    const job = await adminApi.importCredentials(admin.token.value, credentialEndpoint.value, {
      tokens: importLines.value,
    })

    importJobs.add(job)
    importJobs.ensurePolling(admin.token.value)
    closeImportModal()
    admin.notify(`导入任务已提交：${job.total} 条凭据`, 'success')
  } catch (error) {
    importError.value = error instanceof Error ? error.message : '导入凭据失败'
  } finally {
    actionBusy.value = false
  }
}

function batchSetStatus(status: string) {
  const ids = [...selectedIds.value]
  if (!ids.length) {
    return
  }

  confirm.show({
    title: `${statusText(status)}凭据`,
    message: `确认将 ${ids.length} 个凭据设为"${statusText(status)}"吗？`,
    confirmText: `确认${statusText(status)}`,
    confirmVariant: 'secondary',
    action: async () => {
      actionBusy.value = true
      try {
        const result = await adminApi.updateCredentialStatus(admin.token.value, credentialEndpoint.value, { ids, status })
        const updatedCount = result.updated.length
        const errorCount = result.errors.length
        admin.notify(
          errorCount > 0
            ? `处理完成：${updatedCount} 条成功，${errorCount} 条失败`
            : `已更新 ${updatedCount} 条凭据`,
          errorCount > 0 ? 'warning' : 'success',
        )
        await admin.refreshAfterMutation(() => loadCredentials(page.value, pageSize.value))
      } catch (error) {
        admin.notifyError(error, '更新状态失败')
      } finally {
        actionBusy.value = false
      }
    },
  })
}

function batchDelete() {
  const ids = [...selectedIds.value]
  if (!ids.length) {
    return
  }

  confirm.show({
    title: '删除凭据',
    message: `确认删除 ${ids.length} 个凭据吗？此操作不可撤销`,
    confirmText: '确认删除',
    action: async () => {
      actionBusy.value = true
      try {
        const result = await adminApi.deleteCredentials(admin.token.value, credentialEndpoint.value, { ids })
        const deletedCount = result.deleted.length
        const errorCount = result.errors.length
        admin.notify(
          errorCount > 0
            ? `删除完成：${deletedCount} 条成功，${errorCount} 条失败`
            : `已删除 ${deletedCount} 条凭据`,
          errorCount > 0 ? 'warning' : 'success',
        )
        await admin.refreshAfterMutation(() => loadCredentials(1, pageSize.value))
      } catch (error) {
        admin.notifyError(error, '删除失败')
      } finally {
        actionBusy.value = false
      }
    },
  })
}

useAuthReadyLoader(() => loadCredentials(1, pageSize.value))

watch(
  () => admin.selectedHandler.value,
  () => {
    statusFilter.value = []
    planFilter.value = 'all'
    planTypes.value = []
    sortMetric.value = ''
    sortOrder.value = 'desc'
    throttleFiltersExpanded.value = false
    searchInput.value = ''
    searchQuery.value = ''
    if (!admin.activeHandler.value?.supports_credentials) {
      void loadCredentials(1, pageSize.value)
    }
  },
)

watch(
  () => [searchQuery.value, statusFilter.value.join(','), planFilter.value, sortMetric.value, sortModel.value, sortOrder.value, credentialHandlerKey.value],
  () => {
    if (admin.authReady.value && admin.activeHandler.value?.supports_credentials) {
      void loadCredentials(1, pageSize.value)
    }
  },
)

watch(
  () => importJobs.jobs.value,
  (jobs, previousJobs) => {
    const activeHandler = credentialHandlerKey.value
    if (
      !admin.authReady.value
      || !admin.activeHandler.value?.supports_credentials
      || !newlyCompletedImportJobs(jobs, previousJobs).some((job) => job.handler === activeHandler)
    ) {
      return
    }
    void loadCredentials(page.value, pageSize.value)
  },
)
</script>

<template>
  <div class="page-grid">
    <PageHeader
      title="凭据管理"
      icon="mdi-shield-key-outline"
    >
      <template #actions>
        <AdminButton
          v-if="admin.activeHandler.value?.supports_credentials && supportsOAuth"
          variant="secondary"
          prepend-icon="mdi-login"
          :loading="oauthBusy && oauthOpen"
          @click="startOAuthLogin"
        >
          OAuth 登录
        </AdminButton>
        <AdminButton
          v-if="admin.activeHandler.value?.supports_credentials"
          prepend-icon="mdi-import"
          @click="importOpen = true"
        >
          导入凭据
        </AdminButton>
      </template>
    </PageHeader>

    <SectionCard
      title="后端服务"
      icon="mdi-server-network-outline"
    >
      <HandlerSwitchGrid
        :handlers="admin.handlers.value"
        :selected="admin.selectedHandler.value"
        @select="admin.selectedHandler.value = $event"
      />
    </SectionCard>

    <SectionCard
      title="凭据列表"
      icon="mdi-view-list-outline"
    >
      <Transition name="handler-content-fade" mode="out-in">
        <div
          v-if="showHandlerLoadingState"
          :key="`loading-${credentialHandlerKey}`"
          class="credentials-switch-loading"
          aria-live="polite"
        >
          <VProgressCircular
            indeterminate
            color="primary"
            size="32"
            width="3"
          />
          <div class="credentials-switch-copy">
            <div class="credentials-switch-title">正在切换到 {{ activeHandlerLabel }}</div>
            <div class="text-body-2 text-medium-emphasis">加载该后端服务的凭据列表</div>
          </div>
        </div>

        <div
          v-else-if="admin.activeHandler.value?.supports_credentials"
          :key="`credentials-${credentialHandlerKey}`"
          class="d-grid ga-5"
        >
          <div class="toolbar-panel">
            <VProgressLinear
              :active="loading"
              :model-value="loading ? 100 : 0"
              indeterminate
              color="primary"
              class="credentials-inline-progress"
            />
            <div class="filter-toolbar">
              <VTextField
                v-model="searchInput"
                class="filter-grow"
                label="搜索"
                :placeholder="isGeminiHandler || isAntigravityHandler ? '凭据 ID / 邮箱 / Project ID / 状态' : isCodexHandler ? '邮箱 / Account ID / 状态 / 套餐' : '凭据 ID / 状态 / 套餐'"
                prepend-inner-icon="mdi-magnify"
                clearable
              />
              <VSelect
                v-model="pageSize"
                class="filter-select"
                label="每页条数"
                :items="pageSizeOptions"
                @update:model-value="(value) => loadCredentials(1, Number(value))"
              />
              <VSelect
                v-model="sortMetric"
                class="filter-select"
                label="排序指标"
                :items="credentialSortMetricOptions"
              />
              <VSelect
                v-model="sortModel"
                class="filter-select"
                label="模型"
                :items="credentialSortModelOptions"
                :disabled="!credentialSortModelOptions.length"
              />
              <VSelect
                v-model="sortOrder"
                class="filter-select"
                label="排序方向"
                :items="sortOrderOptions"
              />
            </div>

            <div class="status-filter-groups">
              <VChip
                :color="allStatusFilterActive ? 'primary' : undefined"
                :class="{ 'text-primary v-chip--selected': allStatusFilterActive }"
                filter
                @click="clearStatusFilters"
              >
                全部
              </VChip>
              <VChip
                v-for="status in baseStatusFilters"
                :key="status.value"
                :color="statusFilter.includes(status.value) ? 'primary' : undefined"
                :class="{ 'text-primary v-chip--selected': statusFilter.includes(status.value) }"
                filter
                @click="toggleBaseStatusFilter(status.value)"
              >
                {{ status.label }}
              </VChip>

              <div v-if="hasThrottleStatusFilters" class="throttle-filter-group">
                <div class="throttle-filter-row">
                  <VChip
                    :color="throttleFilterActive ? 'primary' : undefined"
                    :class="{ 'text-primary v-chip--selected': throttleFilterActive }"
                    filter
                    @click="toggleThrottleAllFilter"
                  >
                    {{ statusText(CREDENTIAL_THROTTLE_STATUS_ALL) }}
                  </VChip>
                  <button
                    type="button"
                    class="throttle-chip-toggle"
                    :aria-label="throttleFiltersExpanded ? '收起节流子项' : '展开节流子项'"
                    :aria-expanded="throttleFiltersExpanded"
                    @click="toggleThrottleFiltersExpanded"
                  >
                    <VIcon
                      :icon="throttleFiltersExpanded ? 'mdi-chevron-up' : 'mdi-chevron-down'"
                      size="16"
                    />
                  </button>
                </div>

                <div v-if="throttleFiltersExpanded" class="throttle-filter-children">
                  <VChip
                    v-for="status in throttleStatusFilters"
                    :key="status.value"
                    :color="statusFilter.includes(status.value) ? 'primary' : undefined"
                    :class="{ 'text-primary v-chip--selected': statusFilter.includes(status.value) }"
                    filter
                    @click="toggleThrottleTierFilter(status.value)"
                  >
                    {{ status.label }}
                  </VChip>
                </div>
              </div>
            </div>

            <VChipGroup v-if="availablePlanTypes.length > 1 || planFilter !== 'all'" v-model="planFilter" mandatory color="primary">
              <VChip value="all" filter>全部套餐</VChip>
              <VChip
                v-for="plan in availablePlanTypes.filter((item) => item !== 'all')"
                :key="plan"
                :value="plan"
                filter
              >
                {{ planTypeText(plan) }}
              </VChip>
            </VChipGroup>
          </div>

          <div v-if="rows.length" class="d-grid ga-4">
            <PaginationBar
              class="pagination-bar--toolbar"
              :total="total"
              :page="page"
              :max-page="maxPage"
              :total-visible="7"
              density="compact"
              @change="(value) => loadCredentials(value, pageSize)"
            >
              <template #leading>
                <VCheckboxBtn
                  :model-value="allVisibleSelected"
                  density="compact"
                  aria-label="选中当前页全部结果"
                  @update:model-value="toggleSelectAll"
                />
              </template>
            </PaginationBar>

            <div v-if="selectedIds.length" class="selection-bar">
              <div class="selection-bar__summary text-body-1">已选择 {{ selectedIds.length }} 条凭据</div>
              <div class="selection-bar__actions">
                <AdminButton variant="secondary" size="sm" @click="batchSetStatus('enabled')">启用</AdminButton>
                <AdminButton variant="secondary" size="sm" @click="batchSetStatus('disabled')">停用</AdminButton>
                <AdminButton variant="danger" size="sm" @click="batchDelete">删除</AdminButton>
              </div>
            </div>

            <div class="stack-list">
              <template v-if="isCodexHandler">
                <CredentialCardShell
                  v-for="item in codexRows"
                  :key="item.id"
                  :title="codexCredentialEmail(item.id)"
                  :plan-type="item.plan_type"
                  :statuses="item.status"
                  :reason="item.reason"
                  :checked="selectedSet.has(item.id)"
                  @toggle="toggleSelectOne(item.id)"
                >
                  <QuotaCard
                    v-if="canOpenCodexReset(item)"
                    clickable
                    label="重置次数"
                    :value="String(codexResetCreditsCount(item))"
                    tone="accent"
                    @activate="openCodexResetDialog(item)"
                  />

                  <QuotaCardGrid :cards="codexQuotaCards(item)" />

                  <div class="detail-grid">
                    <div class="detail-block">
                      <div class="detail-label text-medium-emphasis">AT到期</div>
                      <div class="detail-value">{{ formatTime(item.expired) }}</div>
                    </div>
                    <div class="detail-block">
                      <div class="detail-label text-medium-emphasis">Account ID</div>
                      <div class="detail-value">{{ codexCredentialAccountID(item.id) }}</div>
                    </div>
                    <div class="detail-block">
                      <div class="detail-label text-medium-emphasis">最近同步</div>
                      <div class="detail-value">{{ formatTime(item.synced_at) }}</div>
                    </div>
                  </div>
                </CredentialCardShell>
              </template>

              <template v-else-if="isGeminiHandler">
                <CredentialCardShell
                  v-for="item in geminiRows"
                  :key="item.id"
                  :title="item.email || item.id"
                  :plan-type="item.plan_type"
                  :statuses="item.status"
                  :reason="item.reason"
                  :checked="selectedSet.has(item.id)"
                  @toggle="toggleSelectOne(item.id)"
                >
                  <QuotaCardGrid :cards="geminiQuotaCards(item)" />

                  <div class="detail-grid">
                    <div
                      v-for="entry in projectDetailEntries(item)"
                      :key="entry.label"
                      class="detail-block"
                    >
                      <div class="detail-label text-medium-emphasis">{{ entry.label }}</div>
                      <div class="detail-value">{{ entry.value }}</div>
                    </div>
                  </div>
                </CredentialCardShell>
              </template>

              <template v-else-if="isAntigravityHandler">
                <CredentialCardShell
                  v-for="item in antigravityRows"
                  :key="item.id"
                  :title="item.email || item.id"
                  :plan-type="item.plan_type"
                  :statuses="item.status"
                  :reason="item.reason"
                  :checked="selectedSet.has(item.id)"
                  @toggle="toggleSelectOne(item.id)"
                >
                  <QuotaCardGrid :cards="antigravityQuotaCards(item)">
                    <QuotaCard
                      v-if="item.credits?.available"
                      clickable
                      label="Credits"
                      :value="formatCreditsAmount(item.credits?.amount || 0)"
                      tone="accent"
                      @activate="openCreditsDialog(item)"
                    />
                  </QuotaCardGrid>

                  <div class="detail-grid">
                    <div
                      v-for="entry in projectDetailEntries(item)"
                      :key="entry.label"
                      class="detail-block"
                    >
                      <div class="detail-label text-medium-emphasis">{{ entry.label }}</div>
                      <div class="detail-value">{{ entry.value }}</div>
                    </div>
                  </div>
                </CredentialCardShell>
              </template>

              <template v-else-if="isOpenCodeGoHandler">
                <CredentialCardShell
                  v-for="item in openCodeGoRows"
                  :key="item.id"
                  :title="item.email"
                  :statuses="item.status"
                  :reason="item.reason"
                  :checked="selectedSet.has(item.id)"
                  @toggle="toggleSelectOne(item.id)"
                >
                  <QuotaCard
                    v-if="item.rewards_count > 0"
                    clickable
                    label="增加余额"
                    :value="`${item.rewards_count} 次`"
                    tone="accent"
                    @activate="openOpenCodeGoRewardsDialog(item)"
                  />

                  <QuotaCardGrid
                    v-if="openCodeGoQuotaCards(item).length"
                    :cards="openCodeGoQuotaCards(item)"
                  />
                  <div class="detail-grid">
                    <div class="detail-block">
                      <div class="detail-label text-medium-emphasis">工作区</div>
                      <div class="detail-value">{{ item.workspace_id || '-' }}</div>
                    </div>
                    <div class="detail-block">
                      <div class="detail-label text-medium-emphasis">最近同步</div>
                      <div class="detail-value">{{ item.synced_at ? formatTime(item.synced_at) : '-' }}</div>
                    </div>
                  </div>
                </CredentialCardShell>
              </template>

              <template v-else>
                <CredentialCardShell
                  v-for="item in genericRows"
                  :key="item.id"
                  :title="item.id"
                  :plan-type="credentialPlanType(item) || undefined"
                  :statuses="item.status"
                  :reason="item.reason"
                  :checked="selectedSet.has(item.id)"
                  @toggle="toggleSelectOne(item.id)"
                >
                  <div class="detail-grid">
                    <div
                      v-for="entry in genericDetailEntries(item)"
                      :key="entry.label"
                      class="detail-block"
                    >
                      <div class="detail-label text-medium-emphasis">{{ entry.label }}</div>
                      <div class="detail-value">{{ entry.value }}</div>
                    </div>
                  </div>
                </CredentialCardShell>
              </template>
            </div>
          </div>

          <EmptyState
            v-else
            :title="emptyStateTitle"
            :description="emptyStateDescription"
            icon="mdi-key-plus"
          >
            <template #action>
              <div class="d-flex flex-wrap justify-center ga-2">
                <AdminButton
                  v-if="supportsOAuth"
                  variant="secondary"
                  prepend-icon="mdi-login"
                  :loading="oauthBusy && oauthOpen"
                  @click="startOAuthLogin"
                >
                  OAuth 登录
                </AdminButton>
                <AdminButton prepend-icon="mdi-import" @click="importOpen = true">
                  导入凭据
                </AdminButton>
              </div>
            </template>
          </EmptyState>
        </div>

        <EmptyState
          v-else
          :key="`unsupported-${credentialHandlerKey}`"
          title="该处理器暂不支持凭据导入"
          description="可以切换到其他处理器，或前往模型页面查看映射能力"
          icon="mdi-key-remove"
        />
      </Transition>
    </SectionCard>

    <ModalDialog
      :open="importOpen"
      :title="admin.activeHandler.value ? `导入 ${admin.activeHandler.value.label} 凭据` : '导入凭据'"
      :description="importDescription"
      max-width="720"
      @close="closeImportModal"
    >
      <div class="d-grid ga-4">
        <VTextarea
          v-model="importTokens"
          rows="8"
          :label="importInputLabel"
          :placeholder="importInputPlaceholder"
          prepend-inner-icon="mdi-text-box-plus-outline"
        />

        <div class="d-flex flex-wrap ga-2">
          <AdminBadge tone="secondary" subtle icon="mdi-text-box-plus-outline">
            待导入 {{ importLines.length }} 条
          </AdminBadge>
          <AdminBadge v-if="activeCredentialField?.help_text" tone="neutral" subtle icon="mdi-information-outline">
            {{ activeCredentialField.help_text }}
          </AdminBadge>
        </div>

        <FormErrorAlert :message="importError" pre-wrap />
      </div>
      <template #footer>
        <AdminButton variant="ghost" @click="closeImportModal">取消</AdminButton>
        <AdminButton
          prepend-icon="mdi-arrow-up-bold-circle-outline"
          :loading="actionBusy"
          @click="createCredential"
        >
          开始导入
        </AdminButton>
      </template>
    </ModalDialog>

    <ModalDialog
      :open="creditsDialogOpen"
      title="Credits 类型"
      icon="mdi-tag-outline"
      max-width="560"
      @close="closeCreditsDialog"
    >
      <div class="d-grid ga-4">
        <div class="detail-grid">
          <div class="detail-block">
            <div class="detail-label text-medium-emphasis">Credits</div>
            <div class="detail-value">{{ formatCreditsAmount(creditsDialogItem?.credits?.amount || 0) }}</div>
          </div>
          <div class="detail-block">
            <div class="detail-label text-medium-emphasis">凭据</div>
            <div class="detail-value">{{ creditsDialogItem?.email || creditsDialogItem?.id || '-' }}</div>
          </div>
        </div>

        <div class="d-flex flex-wrap ga-2">
          <AdminBadge
            v-if="!creditsDialogTypes.length"
            tone="secondary"
            subtle
            icon="mdi-alert-circle-outline"
          >
            无可用类型
          </AdminBadge>
          <AdminBadge
            v-for="creditType in creditsDialogTypes"
            :key="creditType"
            tone="accent"
            subtle
            icon="mdi-tag-outline"
          >
            {{ creditType }}
          </AdminBadge>
        </div>
      </div>
      <template #footer>
        <AdminButton variant="ghost" @click="closeCreditsDialog">关闭</AdminButton>
      </template>
    </ModalDialog>

    <ModalDialog
      :open="codexResetDialogOpen"
      title="重置次数详情"
      icon="mdi-cached"
      max-width="560"
      @close="closeCodexResetDialog"
    >
      <div class="d-grid ga-4">
        <div v-if="codexResetLoading" class="text-medium-emphasis">加载中…</div>
        <div v-else-if="codexResetError" class="text-error">{{ codexResetError }}</div>
        <div v-else-if="!codexResetCredits?.credits?.length" class="text-medium-emphasis">暂无可用重置次数</div>
        <div v-else class="d-grid ga-3">
          <div
            v-for="(credit, idx) in codexResetCredits.credits"
            :key="idx"
            class="detail-block"
          >
            <div class="d-flex align-center justify-space-between ga-2">
              <div class="detail-value">{{ credit.title || '重置额度' }}</div>
              <AdminBadge tone="accent" subtle icon="mdi-check">
                {{ codexResetCreditStatusLabel(credit.status) }}
              </AdminBadge>
            </div>
            <div v-if="credit.expires_at" class="text-medium-emphasis text-body-2">
              过期 {{ formatTime(credit.expires_at) }}
            </div>
          </div>
        </div>
      </div>
      <template #footer>
        <AdminButton
          variant="primary"
          prepend-icon="mdi-cached"
          :disabled="codexResetConsuming || codexResetLoading || !codexResetCredits?.credits?.length"
          @click="consumeCodexReset"
        >
          重置
        </AdminButton>
        <AdminButton variant="ghost" @click="closeCodexResetDialog">关闭</AdminButton>
      </template>
    </ModalDialog>

    <ModalDialog
      :open="openCodeGoRewardsDialogOpen"
      title="增加余额"
      description="应用可用邀请奖励；奖励会按 OpenCode 官方规则抵扣当前 Go 用量窗口"
      icon="mdi-cash-plus"
      max-width="680"
      @close="closeOpenCodeGoRewardsDialog"
    >
      <div class="d-grid ga-4">
        <div v-if="openCodeGoRewardsLoading" class="text-medium-emphasis">加载中…</div>
        <div v-else-if="openCodeGoRewardsError" class="text-error">{{ openCodeGoRewardsError }}</div>
        <div v-else-if="!openCodeGoRewards?.rewards?.length" class="text-medium-emphasis">暂无可用奖励</div>
        <div v-else class="open-code-go-rewards-list d-grid ga-3">
          <div
            v-for="reward in openCodeGoRewards.rewards"
            :key="reward.id"
            class="detail-block"
          >
            <div class="d-flex align-center justify-space-between flex-wrap ga-3">
              <div class="d-grid ga-1">
                <div class="detail-value">{{ formatUsdFromCents(reward.amount_cents) }}</div>
                <div class="text-body-2 text-medium-emphasis">{{ openCodeGoRewardDescription(reward) }}</div>
                <div class="text-body-2 text-medium-emphasis">获得于 {{ formatTime(reward.created_at) }}</div>
              </div>
              <AdminButton
                variant="secondary"
                :loading="openCodeGoRewardApplying === reward.id"
                :disabled="Boolean(openCodeGoRewardApplying)"
                @click="applyOpenCodeGoReward(reward)"
              >
                增加余额
              </AdminButton>
            </div>
          </div>
        </div>
      </div>
      <template #footer>
        <AdminButton variant="ghost" :disabled="Boolean(openCodeGoRewardApplying)" @click="closeOpenCodeGoRewardsDialog">
          关闭
        </AdminButton>
      </template>
    </ModalDialog>

    <ModalDialog
      :open="oauthOpen"
      :title="oauthModalTitle"
      icon="mdi-login"
      max-width="720"
      @close="closeOAuthModal"
    >
      <div class="d-grid ga-4">
        <VTextField
          :model-value="oauthFlow?.authorizeUrl || ''"
          label="授权链接"
          prepend-inner-icon="mdi-vector-link"
          readonly
          :loading="oauthBusy && !oauthFlow"
        />

        <VTextarea
          v-model="oauthCodeInput"
          rows="4"
          label="回调地址"
          :placeholder="oauthCallbackPlaceholder"
          prepend-inner-icon="mdi-keyboard-return"
          :disabled="!oauthFlow"
        />

        <FormErrorAlert :message="oauthError" pre-wrap />
      </div>
      <template #footer>
        <AdminButton variant="ghost" :disabled="oauthBusy" @click="closeOAuthModal">取消</AdminButton>
        <AdminButton
          variant="secondary"
          prepend-icon="mdi-open-in-new"
          :disabled="!oauthFlow"
          @click="reopenOAuthAuthorizeUrl"
        >
          打开授权链接
        </AdminButton>
        <AdminButton
          prepend-icon="mdi-key-plus"
          :loading="oauthBusy"
          :disabled="oauthSubmitDisabled"
          @click="completeOAuthLogin"
        >
          完成添加
        </AdminButton>
      </template>
    </ModalDialog>

    <ConfirmDialogHost
      :confirm="confirm"
      :action-busy="actionBusy"
      description="操作会立即提交到后台"
    />
  </div>
</template>
