import { NavLink, Outlet } from 'react-router-dom'
import { logout } from '@/lib/auth'
import { cn } from '@/lib/utils'
import { useTenant } from '@/hooks/useTenant'
import { useState, useEffect, useMemo } from 'react'
import { api } from '@/api/client'
import type { Tenant } from '@/api/organizations'
import { toast } from 'sonner'

interface NavItem {
  to: string
  label: string
  icon: string
}

const navItems: NavItem[] = [
  { to: '/', label: 'ダッシュボード', icon: '⊞' },
  { to: '/vms', label: 'VM', icon: '▣' },
  { to: '/networks', label: 'ネットワーク', icon: '⊕' },
  { to: '/volumes', label: 'ボリューム', icon: '⊘' },
  { to: '/egress', label: 'Egress', icon: '↗' },
  { to: '/ingress', label: 'Ingress', icon: '↙' },
]

export function TenantLayout() {
  const { tenantId, selectTenant } = useTenant()
  const [tenants, setTenants] = useState<Tenant[]>([])
  const [loading, setLoading] = useState(true)
  const [dropdownOpen, setDropdownOpen] = useState(false)

  useEffect(() => {
    // Show "switched" toast when returning from a reload after tenant switch
    if (sessionStorage.getItem('tenant_just_switched') === 'true') {
      sessionStorage.removeItem('tenant_just_switched')
      toast.success(<span data-testid="toast-success">テナントを切り替えました</span>)
    }

    // Use /me/tenants to avoid requiring infra_admin for org listing.
    // Works for all roles: infra_admin, org_admin, tenant_admin, tenant_member.
    api
      .list<Tenant>('/me/tenants')
      .then((ts) => {
        setTenants(ts)
        // Auto-select if only one tenant and none selected yet
        if (ts.length === 1 && !tenantId) {
          selectTenant(ts[0].id)
        }
      })
      .catch(() => {
        toast.error(<span data-testid="toast-error">テナント一覧の取得に失敗しました</span>, { id: 'tenant-load-error' })
      })
      .finally(() => setLoading(false))
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const currentTenant = useMemo(
    () => tenants.find((t) => t.id === tenantId),
    [tenants, tenantId],
  )

  function handleSelectTenant(id: string) {
    setDropdownOpen(false)
    if (id === tenantId) return
    // Store in sessionStorage so the value survives the reload
    sessionStorage.setItem('pending_tenant_switch', id)
    sessionStorage.setItem('tenant_just_switched', 'true')
    selectTenant(id)
    window.location.reload()
  }

  return (
    <div className="min-h-screen flex flex-col bg-[var(--color-bg-secondary)]">
      {/* Top header */}
      <header className="h-12 border-b border-[var(--color-border)] flex items-center justify-between px-5 bg-white shrink-0">
        <div className="flex items-center gap-3">
          <span className="font-semibold text-[var(--color-text)]">Cirrus</span>
        </div>
        <div className="flex items-center gap-3">
          {/* Tenant switcher */}
          <div className="relative">
            {loading ? (
              <span data-testid="tenant-switcher-spinner" className="inline-flex items-center h-7 px-2">
                <svg className="h-4 w-4 animate-spin text-[var(--color-text-secondary)]" viewBox="0 0 24 24" fill="none">
                  <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
                  <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z" />
                </svg>
              </span>
            ) : (
              <button
                data-testid="tenant-switcher"
                onClick={() => setDropdownOpen((o) => !o)}
                disabled={tenants.length === 0}
                className="inline-flex items-center gap-1 h-7 px-2 text-xs font-medium rounded border border-[var(--color-border)] bg-white text-[var(--color-text)] hover:bg-[var(--color-bg-secondary)] transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
              >
                <span data-testid="tenant-switcher-label">
                  {currentTenant ? currentTenant.name : 'テナントを選択'}
                </span>
                <span className="text-[10px]">▾</span>
              </button>
            )}
            {dropdownOpen && (
              <div className="absolute right-0 top-9 z-50 min-w-[180px] rounded-xl border border-[var(--color-border)] bg-white py-1 shadow-sm">
                {tenants.map((t) => (
                  <button
                    key={t.id}
                    data-testid={`tenant-option-${t.id}`}
                    onClick={() => handleSelectTenant(t.id)}
                    className={cn(
                      'w-full text-left px-3 py-2 text-sm hover:bg-[var(--color-bg-secondary)] transition-colors',
                      t.id === tenantId && 'text-accent font-medium',
                    )}
                  >
                    {t.name}
                  </button>
                ))}
                {tenants.length === 0 && (
                  <div className="px-3 py-2 text-sm text-[var(--color-text-secondary)]">
                    テナントなし
                  </div>
                )}
              </div>
            )}
          </div>
          <button
            onClick={logout}
            className="inline-flex items-center h-7 px-2 text-xs text-[var(--color-text)] hover:bg-[var(--color-bg-secondary)] rounded transition-colors"
          >
            ログアウト
          </button>
        </div>
      </header>

      <div className="flex flex-1 min-h-0">
        {/* Sidebar */}
        <nav className="w-48 shrink-0 border-r border-[var(--color-border)] bg-white flex flex-col py-3">
          {navItems.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.to === '/'}
              className={({ isActive }) =>
                cn(
                  'flex items-center gap-2 px-4 py-2 text-sm transition-colors',
                  isActive
                    ? 'text-accent font-medium bg-accent-subtle'
                    : 'text-[var(--color-text)] hover:bg-[var(--color-bg-secondary)]',
                )
              }
            >
              <span className="text-base leading-none">{item.icon}</span>
              {item.label}
            </NavLink>
          ))}
        </nav>

        {/* Main content */}
        <main className="flex-1 overflow-auto p-5">
          <Outlet />
        </main>
      </div>
    </div>
  )
}
