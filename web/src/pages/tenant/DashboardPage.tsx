import { useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { quotaApi, type TenantQuota } from '@/api/quota'
import { vmsApi, type Vm } from '@/api/vms'
import { useTenant } from '@/hooks/useTenant'
import { QuotaBar } from '@/components/tenant/QuotaBar'
import { VmStatusBadge } from '@/components/tenant/VmStatusBadge'
import { cn } from '@/lib/utils'

export function DashboardPage() {
  const { tenantId } = useTenant()
  const [quota, setQuota] = useState<TenantQuota | null>(null)
  const [vms, setVms] = useState<Vm[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (!tenantId) {
      setLoading(false)
      return
    }
    setLoading(true)
    setError(null)
    Promise.all([
      quotaApi.get(tenantId),
      vmsApi.list(5),
    ])
      .then(([q, vs]) => {
        setQuota(q)
        setVms(vs)
      })
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false))
  }, [tenantId])

  // useMemo must be called unconditionally (hooks rules)
  const statusCount = useMemo(
    () => vms.reduce<Record<string, number>>((acc, vm) => {
      acc[vm.status] = (acc[vm.status] ?? 0) + 1
      return acc
    }, {}),
    [vms]
  )

  if (!tenantId) {
    return (
      <div className="flex items-center justify-center h-64 text-[var(--color-text-secondary)] text-sm">
        ヘッダーからテナントを選択してください
      </div>
    )
  }

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64 text-[var(--color-text-secondary)] text-sm">
        読み込み中...
      </div>
    )
  }

  if (error) {
    return (
      <div className="rounded-xl border border-danger/30 bg-danger/5 p-4 text-sm text-danger">
        エラー: {error}
      </div>
    )
  }

  return (
    <div className="space-y-5">
      <h2 className="text-lg font-semibold text-[var(--color-text)]">ダッシュボード</h2>

      {/* Status summary cards */}
      <div className="grid grid-cols-3 gap-4">
        {(['running', 'stopped', 'error'] as const).map((s) => {
          const label = { running: '実行中', stopped: '停止', error: 'エラー' }[s]
          const color = { running: 'text-success', stopped: 'text-[var(--color-text-secondary)]', error: 'text-danger' }[s]
          return (
            <div key={s} className="bg-white rounded-xl border border-[var(--color-border)] p-4">
              <p className="text-xs text-[var(--color-text-secondary)] mb-1">{label} VM</p>
              <p className={cn('text-2xl font-semibold', color)}>{statusCount[s] ?? 0}</p>
            </div>
          )
        })}
      </div>

      {/* Quota section */}
      {quota && (
        <div className="bg-white rounded-xl border border-[var(--color-border)] p-5">
          <h3 className="text-sm font-semibold text-[var(--color-text)] mb-4">クォータ使用量</h3>
          <div className="grid grid-cols-2 gap-5">
            <QuotaBar label="vCPU" used={quota.usage.VcpusUsed} limit={quota.limits.Vcpus} unit=" コア" />
            <QuotaBar label="メモリ" used={Math.round(quota.usage.RAMMBUsed / 1024)} limit={Math.round(quota.limits.RAMMB / 1024)} unit=" GB" />
            <QuotaBar label="VM 数" used={quota.usage.VMsCount} limit={quota.limits.VMs} unit=" 台" />
            <QuotaBar label="ボリューム容量" used={quota.usage.VolumeGBUsed} limit={quota.limits.VolumeGB} unit=" GB" />
          </div>
        </div>
      )}

      {/* Recent VMs */}
      <div className="bg-white rounded-xl border border-[var(--color-border)] p-5">
        <div className="flex items-center justify-between mb-4">
          <h3 className="text-sm font-semibold text-[var(--color-text)]">最新 VM（最大5件）</h3>
          <Link to="/vms" className="text-xs text-accent hover:underline">
            すべて表示 →
          </Link>
        </div>
        {vms.length === 0 ? (
          <p className="text-sm text-[var(--color-text-secondary)]">VM がありません</p>
        ) : (
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-[var(--color-border)]">
                <th className="text-left py-2 pr-4 text-xs font-medium text-[var(--color-text-secondary)]">名前</th>
                <th className="text-left py-2 pr-4 text-xs font-medium text-[var(--color-text-secondary)]">状態</th>
                <th className="text-left py-2 text-xs font-medium text-[var(--color-text-secondary)]">作成日時</th>
              </tr>
            </thead>
            <tbody>
              {vms.map((vm) => (
                <tr key={vm.id} className="border-b border-[var(--color-border)] last:border-0">
                  <td className="py-2 pr-4">
                    <Link to={`/vms/${vm.id}`} className="text-accent hover:underline font-medium">
                      {vm.name}
                    </Link>
                  </td>
                  <td className="py-2 pr-4">
                    <VmStatusBadge status={vm.status} />
                  </td>
                  <td className="py-2 text-[var(--color-text-secondary)]">
                    {new Date(vm.created_at).toLocaleDateString('ja-JP')}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  )
}
