import { NavLink, Outlet } from 'react-router-dom'
import { logout } from '@/lib/auth'
import { cn } from '@/lib/utils'
import { useTenant } from '@/hooks/useTenant'
import { useState, useEffect } from 'react'
import { organizationsApi, type Tenant } from '@/api/organizations'

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
  const [orgs, setOrgs] = useState<{ id: string; name: string }[]>([])
  const [dropdownOpen, setDropdownOpen] = useState(false)

  useEffect(() => {
    organizationsApi
      .list()
      .then((os) => {
        setOrgs(os)
        return Promise.all(os.map((o) => organizationsApi.listTenants(o.id)))
      })
      .then((results) => setTenants(results.flat()))
      .catch(() => {})
  }, [])

  const currentTenant = tenants.find((t) => t.id === tenantId)

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
            <button
              onClick={() => setDropdownOpen((o) => !o)}
              className="inline-flex items-center gap-1 h-7 px-2 text-xs font-medium rounded border border-[var(--color-border)] bg-white text-[var(--color-text)] hover:bg-[var(--color-bg-secondary)] transition-colors"
            >
              {currentTenant ? currentTenant.name : 'テナントを選択'}
              <span className="text-[10px]">▾</span>
            </button>
            {dropdownOpen && (
              <div
                className="absolute right-0 top-9 z-50 min-w-[180px] rounded-xl border border-[var(--color-border)] bg-white py-1 shadow-sm"
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
