import { useState, useCallback } from 'react'
import { getTenantId, setTenantId, clearTenantId } from '@/lib/auth'

export function useTenant() {
  const [tenantId, setTenantIdState] = useState<string | null>(() =>
    getTenantId(),
  )

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
