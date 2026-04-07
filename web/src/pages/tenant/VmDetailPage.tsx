import { useCallback, useEffect, useState } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { vmsApi, type VmDetail, type VmAction } from '@/api/vms'
import { Button } from '@/components/Button'
import { ErrorMessage } from '@/components/ErrorMessage'
import { VmStatusBadge } from '@/components/tenant/VmStatusBadge'

function DetailRow({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="flex items-start py-3 border-b border-[var(--color-border)] last:border-0">
      <dt className="w-40 shrink-0 text-xs font-medium text-[var(--color-text-secondary)]">{label}</dt>
      <dd className="flex-1 text-sm text-[var(--color-text)]">{value}</dd>
    </div>
  )
}

export function VmDetailPage() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const [vm, setVm] = useState<VmDetail | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [actionLoading, setActionLoading] = useState<VmAction | null>(null)
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false)

  const load = useCallback(() => {
    if (!id) return
    setLoading(true)
    vmsApi.get(id)
      .then(setVm)
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false))
  }, [id])

  useEffect(() => { load() }, [load])

  const handleAction = async (action: VmAction) => {
    if (!id) return
    setActionLoading(action)
    try {
      await vmsApi.action(id, action)
      load()
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'エラーが発生しました')
    } finally {
      setActionLoading(null)
    }
  }

  const handleDelete = async () => {
    if (!id) return
    try {
      await vmsApi.delete(id)
      navigate('/vms')
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'エラーが発生しました')
    }
  }

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64 text-[var(--color-text-secondary)] text-sm">
        読み込み中...
      </div>
    )
  }

  if (error || !vm) {
    return <ErrorMessage message={error ?? 'VM が見つかりません'} />
  }

  return (
    <div className="space-y-4 max-w-2xl">
      <div className="flex items-center gap-3">
        <button
          onClick={() => navigate('/vms')}
          className="text-sm text-[var(--color-text-secondary)] hover:text-[var(--color-text)]"
        >
          ← VM 一覧
        </button>
        <h2 className="text-lg font-semibold text-[var(--color-text)]">{vm.name}</h2>
        <VmStatusBadge status={vm.status} />
      </div>

      {error && <ErrorMessage message={error} />}

      {/* Actions */}
      <div className="bg-white rounded-xl border border-[var(--color-border)] p-4">
        <h3 className="text-sm font-semibold text-[var(--color-text)] mb-3">アクション</h3>
        <div className="flex gap-2 flex-wrap">
          <Button
            variant="primary"
            size="sm"
            onClick={() => handleAction('start')}
            disabled={vm.status !== 'stopped' || actionLoading !== null}
          >
            {actionLoading === 'start' ? '起動中...' : '起動'}
          </Button>
          <Button
            variant="secondary"
            size="sm"
            onClick={() => handleAction('stop')}
            disabled={vm.status !== 'running' || actionLoading !== null}
          >
            {actionLoading === 'stop' ? '停止中...' : '停止'}
          </Button>
          <Button
            variant="secondary"
            size="sm"
            onClick={() => handleAction('reboot')}
            disabled={vm.status !== 'running' || actionLoading !== null}
          >
            {actionLoading === 'reboot' ? '再起動中...' : '再起動'}
          </Button>
          <Button
            variant="danger"
            size="sm"
            onClick={() => setShowDeleteConfirm(true)}
          >
            削除
          </Button>
        </div>
      </div>

      {/* Details */}
      <div className="bg-white rounded-xl border border-[var(--color-border)] p-4">
        <h3 className="text-sm font-semibold text-[var(--color-text)] mb-2">VM 情報</h3>
        <dl>
          <DetailRow label="ID" value={<span className="font-mono text-xs">{vm.id}</span>} />
          <DetailRow label="名前" value={vm.name} />
          <DetailRow label="状態" value={<VmStatusBadge status={vm.status} />} />
          <DetailRow
            label="vCPU"
            value={`${vm.vcpu} コア`}
          />
          <DetailRow
            label="メモリ"
            value={`${Math.round(vm.memory_mb / 1024)} GB`}
          />
          <DetailRow
            label="IPアドレス"
            value={vm.ip_address ? <span className="font-mono">{vm.ip_address}</span> : '—'}
          />
          <DetailRow
            label="ネットワーク"
            value={vm.network_name ?? <span className="font-mono text-xs">{vm.network_id}</span>}
          />
          {vm.host_id && (
            <DetailRow
              label="ホスト"
              value={<span className="font-mono text-xs">{vm.host_id}</span>}
            />
          )}
          <DetailRow
            label="作成日時"
            value={new Date(vm.created_at).toLocaleString('ja-JP')}
          />
        </dl>
      </div>

      {showDeleteConfirm && (
        <div className="fixed inset-0 z-50 flex items-center justify-center">
          <div className="absolute inset-0 bg-black/40" onClick={() => setShowDeleteConfirm(false)} />
          <div className="relative bg-white rounded-xl border border-[var(--color-border)] p-6 w-full max-w-sm shadow-lg">
            <h3 className="text-base font-semibold text-[var(--color-text)] mb-2">VM を削除</h3>
            <p className="text-sm text-[var(--color-text-secondary)] mb-4">
              「{vm.name}」を削除してもよろしいですか？この操作は取り消せません。
            </p>
            <div className="flex gap-2 justify-end">
              <Button variant="ghost" size="sm" onClick={() => setShowDeleteConfirm(false)}>
                キャンセル
              </Button>
              <Button variant="danger" size="sm" onClick={handleDelete}>
                削除する
              </Button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
