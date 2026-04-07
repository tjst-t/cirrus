import { NavLink, Outlet } from 'react-router-dom'
import { logout } from '@/lib/auth'
import { Button } from '@/components/Button'
import { cn } from '@/lib/utils'

const NAV_ITEMS = [
  { to: '/admin/organizations', label: '組織・テナント' },
  { to: '/admin/hosts', label: 'ホスト管理' },
  { to: '/admin/storage', label: 'ストレージ管理' },
  { to: '/admin/quotas', label: 'Quota 設定' },
  { to: '/admin/drift-events', label: 'Drift Events' },
]

export function AdminLayout() {
  return (
    <div className="min-h-screen flex flex-col">
      {/* Top header */}
      <header className="h-12 border-b border-[var(--color-border)] flex items-center justify-between px-5 bg-white shrink-0">
        <div className="flex items-center gap-3">
          <NavLink to="/" className="font-semibold text-[var(--color-text)] hover:text-accent transition-colors">
            Cirrus
          </NavLink>
          <span className="text-[var(--color-text-secondary)] text-sm">/</span>
          <span className="text-sm font-medium text-[var(--color-text-secondary)]">管理者コンソール</span>
        </div>
        <div className="flex items-center gap-3">
          <NavLink to="/" className="text-sm text-[var(--color-text-secondary)] hover:text-accent transition-colors">
            ダッシュボードへ
          </NavLink>
          <Button variant="ghost" size="sm" onClick={logout}>
            ログアウト
          </Button>
        </div>
      </header>

      <div className="flex flex-1 overflow-hidden">
        {/* Sidebar */}
        <aside className="w-52 shrink-0 border-r border-[var(--color-border)] bg-white py-4 flex flex-col">
          <nav className="flex flex-col gap-0.5 px-2">
            {NAV_ITEMS.map((item) => (
              <NavLink
                key={item.to}
                to={item.to}
                className={({ isActive }) =>
                  cn(
                    'block px-3 py-2 rounded text-sm transition-colors',
                    isActive
                      ? 'bg-[var(--color-accent-subtle)] text-accent font-medium'
                      : 'text-[var(--color-text)] hover:bg-[var(--color-bg-secondary)]',
                  )
                }
              >
                {item.label}
              </NavLink>
            ))}
          </nav>
        </aside>

        {/* Main content */}
        <main className="flex-1 overflow-auto bg-[var(--color-bg-secondary)] p-6">
          <Outlet />
        </main>
      </div>
    </div>
  )
}
