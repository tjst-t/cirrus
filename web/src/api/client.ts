import { logout, clearTenantId, TOKEN_KEY, TENANT_ID_KEY } from '@/lib/auth'

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
    // Clear stale credentials (token + tenant_id) and redirect to login
    logout()
    throw new ApiError(401, null, 'Unauthorized')
  }

  if (!res.ok) {
    let errBody: unknown
    try {
      errBody = await res.json()
    } catch {
      errBody = await res.text()
    }
    // Use the API's error message if available
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
