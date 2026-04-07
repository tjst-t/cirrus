import { Header } from '@/components/Header'
import { useTenant } from '@/hooks/useTenant'

export function DashboardPage() {
  const { tenantId } = useTenant()

  return (
    <div className="min-h-screen flex flex-col bg-[var(--color-bg-secondary)]">
      <Header />
      <main className="flex-1 p-5">
        <h2 className="text-lg font-semibold text-[var(--color-text)] mb-4">
          ダッシュボード
        </h2>
        {tenantId ? (
          <p className="text-sm text-[var(--color-text-secondary)]">
            テナント ID: <code className="font-mono">{tenantId}</code>
          </p>
        ) : (
          <p className="text-sm text-[var(--color-text-secondary)]">
            ヘッダーからテナントを選択してください
          </p>
        )}
      </main>
    </div>
  )
}
