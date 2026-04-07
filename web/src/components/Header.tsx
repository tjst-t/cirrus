import { useState, useEffect, useCallback } from 'react'
import { organizationsApi, type Tenant } from '@/api/organizations'
import { useTenant } from '@/hooks/useTenant'
import { logout } from '@/lib/auth'
import { Button } from './Button'
import { cn } from '@/lib/utils'

export function Header() {
  const { tenantId, selectTenant } = useTenant()
  const [tenants, setTenants] = useState<Tenant[]>([])
  const [orgs, setOrgs] = useState<{ id: string; name: string }[]>([])
  const [dropdownOpen, setDropdownOpen] = useState(false)
  const [loading, setLoading] = useState(false)

  const fetchTenants = useCallback(() => {
    setLoading(true)
    organizationsApi
      .list()
      .then((orgs) => {
        setOrgs(orgs)
        return Promise.all(orgs.map((o) => organizationsApi.listTenants(o.id)))
      })
      .then((results) => {
        setTenants(results.flat())
      })
      .catch(() => {
        // ignore: might not be authenticated yet
      })
      .finally(() => setLoading(false))
  }, [])

  // Fetch on mount
  useEffect(() => { fetchTenants() }, [fetchTenants])

  const currentTenant = tenants.find((t) => t.id === tenantId)

  const handleToggleDropdown = () => {
    if (!dropdownOpen) {
      // Refresh tenant list every time the dropdown is opened
      fetchTenants()
    }
    setDropdownOpen((o) => !o)
  }

  return (
    <header className="h-12 border-b border-[var(--color-border)] flex items-center justify-between px-5 bg-white">
      <div className="flex items-center gap-3">
        <span className="font-semibold text-[var(--color-text)]">Cirrus</span>
      </div>

      <div className="flex items-center gap-3">
        {/* Tenant switcher */}
        <div className="relative">
          <Button
            variant="secondary"
            size="sm"
            onClick={handleToggleDropdown}
            disabled={loading}
          >
            {currentTenant ? currentTenant.name : 'テナントを選択'}
            <span className="ml-1 text-xs">▾</span>
          </Button>

          {dropdownOpen && (
            <div
              className="absolute right-0 top-9 z-50 min-w-[180px] rounded-xl border border-[var(--color-border)] bg-white py-1"
              style={{ boxShadow: '0 4px 12px rgba(0,0,0,0.08)' }}
            >
              {orgs.map((org) => {
                const orgTenants = tenants.filter((t) => t.organization_id === org.id)
                return (
                  <div key={org.id}>
                    <div className="px-3 py-1 text-xs font-medium text-[var(--color-text-secondary)] uppercase tracking-wide">
                      {org.name}
                    </div>
                    {orgTenants.map((t) => (
                      <button
                        key={t.id}
                        onClick={() => {
                          selectTenant(t.id)
                          setDropdownOpen(false)
                        }}
                        className={cn(
                          'w-full text-left px-3 py-2 text-sm hover:bg-[var(--color-bg-secondary)] transition-colors',
                          t.id === tenantId && 'text-accent font-medium',
                        )}
                      >
                        {t.name}
                      </button>
                    ))}
                  </div>
                )
              })}
              {tenants.length === 0 && (
                <div className="px-3 py-2 text-sm text-[var(--color-text-secondary)]">
                  テナントなし
                </div>
              )}
            </div>
          )}
        </div>

        <Button variant="ghost" size="sm" onClick={logout}>
          ログアウト
        </Button>
      </div>
    </header>
  )
}
