import { useState, useCallback } from 'react'
import { getTenantId, setTenantId, clearTenantId } from '@/lib/auth'

export function useTenant() {
  const [tenantId, setTenantIdState] = useState<string | null>(() => {
    // Restore pending tenant switch from sessionStorage (set before reload).
    // This ensures the value persists even if external scripts reset localStorage.
    const pending = sessionStorage.getItem('pending_tenant_switch')
    if (pending) {
      sessionStorage.removeItem('pending_tenant_switch')
      setTenantId(pending)
      return pending
    }
    return getTenantId()
  })

  const selectTenant = useCallback((id: string) => {
    setTenantId(id)
    setTenantIdState(id)
  }, [])

  const clearTenant = useCallback(() => {
    clearTenantId()
    setTenantIdState(null)
  }, [])

  return { tenantId, selectTenant, clearTenant }
}
