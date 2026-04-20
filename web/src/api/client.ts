import { clearTenantId, TOKEN_KEY, TENANT_ID_KEY } from '@/lib/auth'
import { ApiErrorClass } from '@/lib/errorMessages'

const BASE_URL = '/api/v1'

export class ApiError extends Error {
  constructor(
    public readonly status: number,
    public readonly body: unknown,
    message?: string,
  ) {
    super(message ?? `API error: ${status}`)
    this.name = 'ApiError'
  }
}

function getHeaders(): HeadersInit {
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
  }

  const token = localStorage.getItem(TOKEN_KEY)
  if (token) {
    headers['Authorization'] = `Bearer ${token}`
  }

  const tenantId = localStorage.getItem(TENANT_ID_KEY)
  if (tenantId) {
    headers['X-Tenant-ID'] = tenantId
  }

  return headers
}

async function request<T>(
  method: string,
  path: string,
  body?: unknown,
): Promise<T> {
  const url = `${BASE_URL}${path}`
  const res = await fetch(url, {
    method,
    headers: getHeaders(),
    body: body !== undefined ? JSON.stringify(body) : undefined,
  })

  if (res.status === 401) {
    // Throw without calling logout() here so callers can decide how to handle.
    // Pages that receive a 401 can clear their own state (e.g., clearTenant)
    // and let ProtectedRoute handle the redirect when the token is truly gone.
    throw new ApiErrorClass({ code: 'ERR_UNAUTHORIZED', message: 'Unauthorized' })
  }

  if (!res.ok) {
    const rawText = await res.text()
    let errBody: unknown
    try {
      errBody = JSON.parse(rawText)
    } catch {
      errBody = rawText
    }

    // Handle structured error format: { code, message, detail }
    if (
      errBody !== null &&
      typeof errBody === 'object' &&
      'code' in (errBody as object)
    ) {
      const structured = errBody as { code: string; message?: string; detail?: Record<string, unknown> }
      // Clear stale tenant ID when the backend rejects it
      if (res.status === 400 && structured.message === 'invalid tenant') {
        clearTenantId()
      }
      throw new ApiErrorClass({ code: structured.code, message: structured.message ?? '', detail: structured.detail })
    }

    // Legacy error format: { error: "..." }
    const apiMessage =
      errBody !== null &&
      typeof errBody === 'object' &&
      'error' in (errBody as object)
        ? (errBody as { error: string }).error
        : undefined
    // Clear stale tenant ID when the backend rejects it
    if (res.status === 400 && apiMessage === 'invalid tenant') {
      clearTenantId()
    }
    throw new ApiError(res.status, errBody, apiMessage)
  }

  if (res.status === 204) {
    return undefined as T
  }

  return res.json() as Promise<T>
}

export interface ListResponse<T> {
  items: T[]
  next_cursor: string
}

export const api = {
  get: <T>(path: string) => request<T>('GET', path),
  // list() fetches a paginated endpoint and returns the items array directly
  list: <T>(path: string) =>
    request<ListResponse<T>>('GET', path).then((r) => r.items),
  post: <T>(path: string, body?: unknown) => request<T>('POST', path, body),
  put: <T>(path: string, body?: unknown) => request<T>('PUT', path, body),
  patch: <T>(path: string, body?: unknown) => request<T>('PATCH', path, body),
  delete: <T>(path: string) => request<T>('DELETE', path),
}
