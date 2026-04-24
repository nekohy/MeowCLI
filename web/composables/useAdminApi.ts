import type {
  AuthKeyItem,
  BatchCreateResponse,
  BatchDeleteResponse,
  BatchStatusResponse,
  CredentialHandlerKey,
  CredentialItem,
  CreateAuthKeyResponse,
  ModelItem,
  OverviewResponse,
  LogItem,
  PaginatedResponse,
  SettingsSnapshot,
  SetupResult,
} from '~/types/admin'

const PRIMARY_TOKEN_KEY = 'meowcli_admin_token'
const LEGACY_TOKEN_KEY = 'admin_token'

export const AUTH_INVALID_EVENT = 'meowcli-admin-auth-invalid'

export class ApiError extends Error {
  status: number
  data: unknown

  constructor(message: string, status: number, data: unknown) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.data = data
  }
}

interface RequestOptions {
  token?: string
  method?: 'GET' | 'POST' | 'PUT' | 'DELETE'
  body?: unknown
  query?: Record<string, string | number | boolean | undefined | null>
}

type QueryOptions = NonNullable<RequestOptions['query']>

interface PaginationOptions {
  page?: number
  pageSize?: number
}

type ListOptions<TExtra extends QueryOptions = Record<never, never>> = PaginationOptions & TExtra

function buildUrl(path: string, query?: RequestOptions['query']) {
  const url = new URL(`/admin/api${path}`, window.location.origin)
  if (query) {
    Object.entries(query).forEach(([key, value]) => {
      if (value !== undefined && value !== null && value !== '') {
        url.searchParams.set(key, String(value))
      }
    })
  }
  return url.toString()
}

async function parseResponse(response: Response) {
  const contentType = response.headers.get('content-type') || ''
  if (contentType.includes('application/json')) {
    return response.json()
  }

  const text = await response.text()
  if (!text) {
    return null
  }
  return { error: text }
}

export function getStoredToken() {
  if (!import.meta.client) {
    return ''
  }
  return localStorage.getItem(PRIMARY_TOKEN_KEY) || localStorage.getItem(LEGACY_TOKEN_KEY) || ''
}

export function setStoredToken(token: string) {
  if (!import.meta.client) {
    return
  }

  if (!token) {
    localStorage.removeItem(PRIMARY_TOKEN_KEY)
    localStorage.removeItem(LEGACY_TOKEN_KEY)
    return
  }

  localStorage.setItem(PRIMARY_TOKEN_KEY, token)
  localStorage.setItem(LEGACY_TOKEN_KEY, token)
}

export async function apiRequest<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const { token = '', method = 'GET', body, query } = options
  const headers: Record<string, string> = {}

  if (body !== undefined) {
    headers['Content-Type'] = 'application/json'
  }
  if (token) {
    headers.Authorization = `Bearer ${token}`
  }

  const response = await fetch(buildUrl(path, query), {
    method,
    headers,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  })

  const data = await parseResponse(response)
  if (!response.ok) {
    if ((response.status === 401 || response.status === 403) && token && getStoredToken() === token && import.meta.client) {
      setStoredToken('')
      window.dispatchEvent(new CustomEvent(AUTH_INVALID_EVENT, { detail: { status: response.status } }))
    }
    throw new ApiError((data as { error?: string } | null)?.error || `Request failed (${response.status})`, response.status, data)
  }

  return data as T
}

type CredentialListFilters = {
  search?: string
  status?: 'enabled' | 'disabled'
  planType?: string
}

function normalizeCredentialEndpoint(endpoint = '') {
  const trimmed = endpoint.trim()
  if (!trimmed) {
    return ''
  }

  let path = trimmed
  try {
    path = new URL(trimmed, window.location.origin).pathname
  } catch {
    path = trimmed
  }

  path = path.replace(/^\/admin\/api/, '')
  return path.startsWith('/') ? path : `/${path}`
}

function credentialsPathForHandler(handler: CredentialHandlerKey, endpoint = '') {
  return normalizeCredentialEndpoint(endpoint) || `/${encodeURIComponent(handler)}`
}

function buildPaginatedQuery<TExtra extends QueryOptions>(options: PaginationOptions, extraQuery?: TExtra): QueryOptions {
  const { page = 1, pageSize = 25 } = options
  return {
    page,
    page_size: pageSize,
    ...(extraQuery || {}),
  }
}

export const adminApi = {
  status() {
    return apiRequest<{ need_setup: boolean }>('/status')
  },
  setup(payload: Partial<{ key: string; note: string }>) {
    return apiRequest<SetupResult>('/setup', { method: 'POST', body: payload })
  },
  overview(token: string) {
    return apiRequest<OverviewResponse>('/overview', { token })
  },
  getSettings(token: string) {
    return apiRequest<SettingsSnapshot>('/settings', { token })
  },
  updateSettings(token: string, payload: SettingsSnapshot) {
    return apiRequest<{ settings: SettingsSnapshot }>('/settings', {
      token,
      method: 'PUT',
      body: payload,
    })
  },
  listCredentials(token: string, handler: CredentialHandlerKey, options: ListOptions<CredentialListFilters> = {}, endpoint = '') {
    const {
      search = '',
      status,
      planType,
    } = options
    return apiRequest<PaginatedResponse<CredentialItem>>(credentialsPathForHandler(handler, endpoint), {
      token,
      query: buildPaginatedQuery(options, {
        search,
        status,
        plan_type: planType,
      }),
    })
  },
  createCredentials(token: string, handler: CredentialHandlerKey, payload: { tokens: string[] }, endpoint = '') {
    return apiRequest<BatchCreateResponse>(credentialsPathForHandler(handler, endpoint), {
      token,
      method: 'POST',
      body: payload,
    })
  },
  updateCredentialStatus(token: string, handler: CredentialHandlerKey, payload: { ids: string[]; status: string }, endpoint = '') {
    return apiRequest<BatchStatusResponse>(`${credentialsPathForHandler(handler, endpoint)}/status`, {
      token,
      method: 'PUT',
      body: payload,
    })
  },
  deleteCredentials(token: string, handler: CredentialHandlerKey, payload: { ids: string[] }, endpoint = '') {
    return apiRequest<BatchDeleteResponse>(credentialsPathForHandler(handler, endpoint), {
      token,
      method: 'DELETE',
      body: payload,
    })
  },
  listModels(token: string) {
    return apiRequest<ModelItem[]>('/models', { token })
  },
  createModel(token: string, payload: { alias: string; origin: string; handler: string; plan_types: string; extra: Record<string, unknown> }) {
    return apiRequest<ModelItem>('/models', { token, method: 'POST', body: payload })
  },
  updateModel(token: string, alias: string, payload: { origin: string; handler: string; plan_types: string; extra: Record<string, unknown> }) {
    return apiRequest<ModelItem>(`/models/${encodeURIComponent(alias)}`, {
      token,
      method: 'PUT',
      body: payload,
    })
  },
  deleteModel(token: string, alias: string) {
    return apiRequest<{ ok: boolean }>(`/models/${encodeURIComponent(alias)}`, {
      token,
      method: 'DELETE',
    })
  },
  listLogs(token: string, options: ListOptions = {}) {
    return apiRequest<PaginatedResponse<LogItem>>('/logs', {
      token,
      query: buildPaginatedQuery(options),
    })
  },
  listAuthKeys(token: string) {
    return apiRequest<AuthKeyItem[]>('/auth-keys', { token })
  },
  createAuthKey(token: string, payload: { key?: string; role: string; note: string }) {
    return apiRequest<CreateAuthKeyResponse>('/auth-keys', { token, method: 'POST', body: payload })
  },
  updateAuthKey(token: string, key: string, payload: { role: string; note: string }) {
    return apiRequest<CreateAuthKeyResponse>(`/auth-keys/${encodeURIComponent(key)}`, {
      token,
      method: 'PUT',
      body: payload,
    })
  },
  deleteAuthKey(token: string, key: string) {
    return apiRequest<{ ok: boolean }>(`/auth-keys/${encodeURIComponent(key)}`, {
      token,
      method: 'DELETE',
    })
  },
}
