import { useEffect, useState } from 'react'
import { quotaApi, type TenantQuota } from '@/api/quota'
import { ApiError } from '@/api/client'
import { useTenant } from '@/hooks/useTenant'
import { QuotaBar } from '@/components/tenant/QuotaBar'
import { ErrorMessage } from '@/components/ErrorMessage'

export function DashboardPage() {
  const { tenantId, clearTenant } = useTenant()
  const [quota, setQuota] = useState<TenantQuota | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (!tenantId) {
      setLoading(false)
      return
    }
    setLoading(true)
    setError(null)
    quotaApi
      .get(tenantId)
      .then((q) => setQuota(q))
      .catch((e: Error) => {
        // 401 means the current tenant credential is no longer valid.
        // Clear the tenant selection so the user can pick a valid one.
        if (e instanceof ApiError && e.status === 401) {
          clearTenant()
        } else {
          setError(e.message)
        }
      })
      .finally(() => setLoading(false))
  }, [tenantId, clearTenant])

  // ── テナント未選択 ──
  if (!tenantId) {
    return (
      <div
        data-testid="dashboard-no-tenant-message"
        className="flex items-center justify-center h-64 text-[var(--color-text-secondary)] text-sm"
      >
        ヘッダーからテナントを選択してください
      </div>
    )
  }

  // ── ローディング ──
  if (loading) {
    return (
      <div
        data-testid="dashboard-loading"
        className="flex items-center justify-center h-64 text-[var(--color-text-secondary)] text-sm"
      >
        読み込み中...
      </div>
    )
  }

  // ── エラー ──
  if (error) {
    return <ErrorMessage data-testid="dashboard-error-message" message={error} />
  }

  // ── データなし（念のため） ──
  if (!quota) {
    return null
  }

  const { limits, usage } = quota
  const memUsedGb = Math.floor(usage.memory_mb_used / 1024)
  const memLimitGb = Math.floor(limits.memory_mb / 1024)

  return (
    <div className="space-y-5">
      <h2 className="text-lg font-semibold text-[var(--color-text)]">ダッシュボード</h2>

      {/* ── サマリカード ── */}
      <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-5">
        {/* VM 数 */}
        <div className="bg-[var(--color-surface)] rounded-xl border border-[var(--color-border)] p-4">
          <p className="text-xs text-[var(--color-text-secondary)] mb-1">VM 数</p>
          <p
            className="text-2xl font-semibold text-[var(--color-text)]"
            data-testid="dashboard-vm-count"
          >
            {usage.vm_count_used}
          </p>
          <p className="text-xs text-[var(--color-text-secondary)] mt-1">台</p>
        </div>

        {/* ネットワーク数 */}
        <div className="bg-[var(--color-surface)] rounded-xl border border-[var(--color-border)] p-4">
          <p className="text-xs text-[var(--color-text-secondary)] mb-1">ネットワーク数</p>
          <p
            className="text-2xl font-semibold text-[var(--color-text)]"
            data-testid="dashboard-network-count"
          >
            {usage.networks_used}
          </p>
          <p className="text-xs text-[var(--color-text-secondary)] mt-1">個</p>
        </div>

        {/* ボリューム容量 */}
        <div className="bg-[var(--color-surface)] rounded-xl border border-[var(--color-border)] p-4">
          <p className="text-xs text-[var(--color-text-secondary)] mb-1">ボリューム容量</p>
          <p
            className="text-2xl font-semibold text-[var(--color-text)]"
            data-testid="dashboard-volume-gb"
          >
            {usage.volume_gb_used}
          </p>
          <p className="text-xs text-[var(--color-text-secondary)] mt-1">GB</p>
        </div>

        {/* vCPU */}
        <div className="bg-[var(--color-surface)] rounded-xl border border-[var(--color-border)] p-4">
          <p className="text-xs text-[var(--color-text-secondary)] mb-1">vCPU</p>
          <p
            className="text-2xl font-semibold text-[var(--color-text)]"
            data-testid="dashboard-vcpu-usage"
          >
            {usage.vcpus_used} / {limits.vcpus}
          </p>
          <p className="text-xs text-[var(--color-text-secondary)] mt-1">コア</p>
        </div>

        {/* メモリ */}
        <div className="bg-[var(--color-surface)] rounded-xl border border-[var(--color-border)] p-4">
          <p className="text-xs text-[var(--color-text-secondary)] mb-1">メモリ</p>
          <p
            className="text-2xl font-semibold text-[var(--color-text)]"
            data-testid="dashboard-memory-usage"
          >
            {memUsedGb} / {memLimitGb} GB
          </p>
        </div>
      </div>

      {/* ── Quota バー ── */}
      <div className="bg-[var(--color-surface)] rounded-xl border border-[var(--color-border)] p-5">
        <h3 className="text-sm font-semibold text-[var(--color-text)] mb-4">クォータ使用量</h3>
        <div className="grid grid-cols-1 gap-5 sm:grid-cols-2">
          <QuotaBar
            data-testid="quota-bar-vcpus"
            label="vCPU"
            used={usage.vcpus_used}
            limit={limits.vcpus}
            unit=" コア"
          />
          <QuotaBar
            data-testid="quota-bar-memory"
            label="メモリ"
            used={memUsedGb}
            limit={memLimitGb}
            unit=" GB"
          />
          <QuotaBar
            data-testid="quota-bar-vm-count"
            label="VM 数"
            used={usage.vm_count_used}
            limit={limits.vm_count}
            unit=" 台"
          />
          <QuotaBar
            data-testid="quota-bar-volume-gb"
            label="ボリューム容量"
            used={usage.volume_gb_used}
            limit={limits.volume_gb}
            unit=" GB"
          />
          <QuotaBar
            data-testid="quota-bar-networks"
            label="ネットワーク数"
            used={usage.networks_used}
            limit={limits.networks}
            unit=" 個"
          />
          <QuotaBar
            data-testid="quota-bar-volumes"
            label="ボリューム数"
            used={usage.volumes_used}
            limit={limits.volumes}
            unit=" 個"
          />
          <QuotaBar
            data-testid="quota-bar-egresses"
            label="Egress 数"
            used={usage.egresses_used}
            limit={limits.egresses}
            unit=" 個"
          />
          <QuotaBar
            data-testid="quota-bar-ingresses"
            label="Ingress 数"
            used={usage.ingresses_used}
            limit={limits.ingresses}
            unit=" 個"
          />
        </div>
      </div>
    </div>
  )
}
