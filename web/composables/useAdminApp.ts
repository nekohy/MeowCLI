import {
  adminApi,
  getStoredToken,
  setStoredToken,
} from '~/composables/useAdminApi'
import {
  normalizeTheme,
  resolveInitialTheme,
} from '~/lib/admin'
import type {
  BuildInfo,
  HandlerOverview,
  OverviewResponse,
  SetupResult,
  SetupState,
  ThemeMode,
  ToastMessage,
  UiTone,
} from '~/types/admin'

const EMPTY_OVERVIEW: OverviewResponse = {
  summary: {
    credentials_enabled: 0,
    credentials_total: 0,
    models_total: 0,
    logs_total: 0,
    auth_keys_total: 0,
  },
  handlers: [],
  recent_logs: [],
}

const DEFAULT_BUILD_INFO: BuildInfo = {
  version: 'dev',
  build_time: 'unknown',
}

export function useAdminApp() {
  const theme = useState<ThemeMode>('admin-theme', () => 'light')
  const token = useState<string>('admin-token', () => '')
  const loginInput = useState<string>('admin-login-input', () => '')
  const authReady = useState<boolean>('admin-auth-ready', () => false)
  const booting = useState<boolean>('admin-booting', () => true)
  const needSetup = useState<boolean>('admin-need-setup', () => false)
  const setupState = useState<SetupState>('admin-setup-state', () => ({ key: '', note: '' }))
  const setupDone = useState<boolean>('admin-setup-done', () => false)
  const setupResult = useState<SetupResult | null>('admin-setup-result', () => null)
  const loginError = useState<string>('admin-login-error', () => '')
  const overview = useState<OverviewResponse>('admin-overview', () => EMPTY_OVERVIEW)
  const buildInfo = useState<BuildInfo>('admin-build-info', () => DEFAULT_BUILD_INFO)
  const toast = useState<ToastMessage | null>('admin-toast', () => null)
  const selectedHandler = useState<string>('admin-selected-handler', () => 'codex')

  const handlers = computed(() => overview.value.handlers)
  const activeHandler = computed<HandlerOverview | null>(
    () => handlers.value.find((item) => item.key === selectedHandler.value) || handlers.value[0] || null,
  )
  const handlerLookup = computed(() => new Map(handlers.value.map((item) => [item.key, item])))

  function initializeClient() {
    if (!import.meta.client) {
      return
    }

    token.value = getStoredToken()
    loginInput.value = token.value
    theme.value = normalizeTheme(resolveInitialTheme())
  }

  function notify(text: string, tone: UiTone = 'success') {
    toast.value = { text, tone }
  }

  function dismissToast() {
    toast.value = null
  }

  function resetAuthState(message = '') {
    setStoredToken('')
    token.value = ''
    loginInput.value = ''
    authReady.value = false
    needSetup.value = false
    setupDone.value = false
    setupResult.value = null
    loginError.value = message
    overview.value = EMPTY_OVERVIEW
  }

  async function loadOverview(nextToken = token.value, quiet = false) {
    if (!quiet) {
      booting.value = true
    }

    try {
      const data = await adminApi.overview(nextToken)
      overview.value = data
      authReady.value = true
      loginError.value = ''
      needSetup.value = false

      if (nextToken) {
        token.value = nextToken
        loginInput.value = nextToken
        setStoredToken(nextToken)
      }

      selectedHandler.value = data.handlers.some((item) => item.key === selectedHandler.value)
        ? selectedHandler.value
        : (data.handlers[0]?.key || '')

      return true
    } catch (error) {
      const status = error instanceof Error && 'status' in error ? Number((error as { status?: number }).status) : 0
      if (status === 401 || status === 403) {
        resetAuthState('管理员密钥无效或已失效')
        return false
      }

      authReady.value = false
      loginError.value = error instanceof Error ? error.message : '管理台初始化失败'
      return false
    } finally {
      booting.value = false
    }
  }

  async function boot() {
    booting.value = true
    try {
      const status = await adminApi.status()
      buildInfo.value = status.build_info
      if (status.need_setup) {
        needSetup.value = true
        authReady.value = false
        setupDone.value = false
        setupResult.value = null
        booting.value = false
        return
      }
    } catch (error) {
      authReady.value = false
      needSetup.value = false
      loginError.value = error instanceof Error ? error.message : '管理台状态检查失败'
      booting.value = false
      return
    }

    if (!token.value) {
      authReady.value = false
      loginError.value = ''
      booting.value = false
      return
    }

    await loadOverview(token.value)
  }

  async function submitLogin() {
    const nextToken = (setupDone.value && setupResult.value ? setupResult.value.key : loginInput.value).trim()
    if (!nextToken) {
      loginError.value = '请输入管理员密钥'
      return false
    }
    token.value = nextToken
    loginInput.value = nextToken

    if (await loadOverview(nextToken)) {
      needSetup.value = false
      setupDone.value = false
      setupResult.value = null
      notify('登录成功')
      return true
    }

    return false
  }

  async function setupAdmin() {
    booting.value = true
    loginError.value = ''

    try {
      const payload: Partial<SetupState> = {}
      if (setupState.value.key.trim()) {
        payload.key = setupState.value.key.trim()
      }
      if (setupState.value.note.trim()) {
        payload.note = setupState.value.note.trim()
      }
      setupResult.value = await adminApi.setup(payload)
      setupDone.value = true
      return true
    } catch (error) {
      loginError.value = error instanceof Error ? error.message : '初始化失败'
      return false
    } finally {
      booting.value = false
    }
  }

  function toggleTheme() {
    theme.value = theme.value === 'dark' ? 'light' : 'dark'
  }

  function logout() {
    resetAuthState('')
  }

  return {
    activeHandler,
    authReady,
    boot,
    booting,
    buildInfo,
    dismissToast,
    handlerLookup,
    handlers,
    initializeClient,
    loadOverview,
    loginError,
    loginInput,
    logout,
    needSetup,
    notify,
    overview,
    resetAuthState,
    selectedHandler,
    setStoredToken,
    setupAdmin,
    setupDone,
    setupResult,
    setupState,
    submitLogin,
    theme,
    toast,
    toggleTheme,
    token,
  }
}
