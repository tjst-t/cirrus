export const TOKEN_KEY = 'auth_token'
export const TENANT_ID_KEY = 'selected_tenant_id'

export function getToken(): string | null {
  return localStorage.getItem(TOKEN_KEY)
}

export function setToken(token: string): void {
  localStorage.setItem(TOKEN_KEY, token)
}

export function clearToken(): void {
  localStorage.removeItem(TOKEN_KEY)
}

export function isAuthenticated(): boolean {
  return getToken() !== null
}

export function getTenantId(): string | null {
  return localStorage.getItem(TENANT_ID_KEY)
}

export function setTenantId(tenantId: string): void {
  localStorage.setItem(TENANT_ID_KEY, tenantId)
}

export function clearTenantId(): void {
  localStorage.removeItem(TENANT_ID_KEY)
}

export function logout(): void {
  clearToken()
  clearTenantId()
  window.location.href = '/login'
}
