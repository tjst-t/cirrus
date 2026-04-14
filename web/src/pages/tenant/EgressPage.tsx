import { useCallback, useEffect, useState } from 'react'
import { egressApi, type Egress } from '@/api/egress'
import { networksApi, type Network } from '@/api/networks'
import { useTenant } from '@/hooks/useTenant'
import { Button } from '@/components/Button'
import { ErrorMessage } from '@/components/ErrorMessage'

// ---- Create Egress Dialog ----
function CreateEgressDialog({
  tenantId,
  networkId,
  onClose,
  onCreated,
}: {
  tenantId: string
  networkId: string
  onClose: () => void
  onCreated: () => void
}) {
  const [egressType, setEgressType] = useState('nat_gateway')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setLoading(true)
    setError(null)
    try {
      await egressApi.create(tenantId, networkId, { type: egressType, config: {} })
      onCreated()
      onClose()
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'エラーが発生しました')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div data-testid="egress-create-dialog" className="fixed inset-0 z-50 flex items-center justify-center">
      <div className="absolute inset-0 bg-black/40 backdrop-blur-sm" onClick={onClose} />
      <div className="relative bg-white rounded-2xl border border-[var(--color-border)] p-6 w-full max-w-md shadow-xl">
        <div className="flex items-center gap-3 mb-5">
          <h3 className="text-base font-semibold text-[var(--color-text)]">Egress を作成</h3>
        </div>
        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label className="block text-xs font-medium text-[var(--color-text)] mb-1.5">
              タイプ <span className="text-red-500">*</span>
            </label>
            <select
              data-testid="egress-type-select"
              value={egressType}
              onChange={(e) => setEgressType(e.target.value)}
              className="w-full h-9 px-3 text-sm border border-[var(--color-border)] rounded-lg focus:outline-none focus:ring-2 focus:ring-accent/30 focus:border-accent/50 transition-all bg-white"
            >
              <option value="nat_gateway">nat_gateway</option>
            </select>
          </div>
          {error && <ErrorMessage message={error} data-testid="egress-error-message" />}
          <div className="flex gap-2 justify-end pt-1">
            <Button type="button" variant="ghost" size="sm" onClick={onClose}>キャンセル</Button>
            <Button
              data-testid="egress-create-submit"
              type="submit"
              variant="primary"
              size="sm"
              disabled={loading}
            >
              {loading ? '作成中...' : '作成'}
            </Button>
          </div>
        </form>
      </div>
    </div>
  )
}

// ---- Main Page ----
export function EgressPage() {
  const { tenantId } = useTenant()
  const [networks, setNetworks] = useState<Network[]>([])
  const [networksLoaded, setNetworksLoaded] = useState(false)
  const [selectedNetwork, setSelectedNetwork] = useState<string>('')
  const [egresses, setEgresses] = useState<Egress[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [showCreate, setShowCreate] = useState(false)
  const [deleteId, setDeleteId] = useState<string | null>(null)
  const [deleting, setDeleting] = useState(false)

  useEffect(() => {
    networksApi.list().then((ns) => {
      setNetworks(ns)
      setNetworksLoaded(true)
      if (ns.length > 0) setSelectedNetwork(ns[0].id)
    }).catch(() => {
      setNetworksLoaded(true)
    })
  }, [])

  const load = useCallback(() => {
    if (!tenantId || !selectedNetwork) return
    setLoading(true)
    setError(null)
    egressApi.list(tenantId, selectedNetwork)
      .then(setEgresses)
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false))
  }, [tenantId, selectedNetwork])

  useEffect(() => {
    if (selectedNetwork) load()
  }, [load])

  const handleDelete = async (id: string) => {
    if (!tenantId || !selectedNetwork) return
    setDeleting(true)
    try {
      await egressApi.delete(tenantId, selectedNetwork, id)
      setDeleteId(null)
      load()
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : '削除に失敗しました')
    } finally {
      setDeleting(false)
    }
  }

  return (
    <div className="space-y-5">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-lg font-semibold text-[var(--color-text)]">Egress 管理</h2>
          <p className="text-xs text-[var(--color-text-secondary)] mt-0.5">テナントのアウトバウンドゲートウェイを管理します</p>
        </div>
        <Button
          data-testid="egress-create-button"
          variant="primary"
          size="sm"
          onClick={() => setShowCreate(true)}
          disabled={!selectedNetwork}
        >
          <svg className="w-3.5 h-3.5 mr-1.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2.5} d="M12 4v16m8-8H4" />
          </svg>
          Egress を作成
        </Button>
      </div>

      {/* Network selector */}
      <div className="flex items-center gap-3">
        <label className="text-sm font-medium text-[var(--color-text)]">ネットワーク</label>
        <select
          value={selectedNetwork}
          onChange={(e) => setSelectedNetwork(e.target.value)}
          className="rounded-lg border border-[var(--color-border)] px-3 py-1.5 text-sm bg-white focus:outline-none focus:ring-2 focus:ring-accent/30"
        >
          {networks.map((n) => (
            <option key={n.id} value={n.id}>{n.name}</option>
          ))}
        </select>
      </div>

      {/* No networks message */}
      {networksLoaded && networks.length === 0 && (
        <div
          data-testid="egress-no-network-message"
          className="bg-white rounded-2xl border border-dashed border-[var(--color-border)] p-12 text-center"
        >
          <p className="text-sm font-medium text-[var(--color-text)] mb-1">ネットワークがありません</p>
          <p className="text-xs text-[var(--color-text-secondary)]">先にネットワークを作成してください</p>
        </div>
      )}

      {/* Error */}
      {error && <ErrorMessage message={error} data-testid="egress-error-message" />}

      {/* List */}
      {networksLoaded && networks.length > 0 && !error && (
        loading ? (
          <div className="flex items-center justify-center h-40 text-[var(--color-text-secondary)] text-sm">
            読み込み中...
          </div>
        ) : egresses.length === 0 ? (
          <div
            data-testid="egress-empty-state"
            className="bg-white rounded-2xl border border-dashed border-[var(--color-border)] p-12 text-center"
          >
            <p className="text-sm font-medium text-[var(--color-text)] mb-1">Egress がありません</p>
            <p className="text-xs text-[var(--color-text-secondary)]">「Egress を作成」ボタンから作成してください</p>
          </div>
        ) : (
          <div className="bg-white rounded-2xl border border-[var(--color-border)] overflow-hidden">
            <table className="w-full text-sm">
              <thead className="bg-[var(--color-bg-secondary)]">
                <tr>
                  <th className="text-left px-4 py-2.5 font-medium text-[var(--color-text-secondary)]">タイプ</th>
                  <th className="text-left px-4 py-2.5 font-medium text-[var(--color-text-secondary)]">パブリック IP</th>
                  <th />
                </tr>
              </thead>
              <tbody>
                {egresses.map((eg) => (
                  <tr
                    key={eg.id}
                    data-testid={`egress-row-${eg.id}`}
                    className="border-t border-[var(--color-border)]"
                  >
                    <td data-testid={`egress-type-${eg.id}`} className="px-4 py-3 font-medium text-[var(--color-text)]">
                      {eg.type}
                    </td>
                    <td data-testid={`egress-public-ip-${eg.id}`} className="px-4 py-3 text-[var(--color-text-secondary)]">
                      {eg.config?.public_ip ?? '—'}
                    </td>
                    <td className="px-4 py-3 text-right">
                      {deleteId === eg.id ? (
                        <span className="flex items-center justify-end gap-2">
                          <span className="text-xs text-[var(--color-text-secondary)]">削除しますか？</span>
                          <Button
                            data-testid={`egress-delete-confirm-${eg.id}`}
                            variant="danger"
                            size="sm"
                            disabled={deleting}
                            onClick={() => handleDelete(eg.id)}
                          >
                            削除する
                          </Button>
                          <Button
                            data-testid={`egress-delete-cancel-${eg.id}`}
                            variant="ghost"
                            size="sm"
                            onClick={() => setDeleteId(null)}
                          >
                            キャンセル
                          </Button>
                        </span>
                      ) : (
                        <Button
                          data-testid={`egress-delete-button-${eg.id}`}
                          variant="danger"
                          size="sm"
                          onClick={() => setDeleteId(eg.id)}
                        >
                          削除
                        </Button>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )
      )}

      {/* Create Dialog */}
      {showCreate && tenantId && selectedNetwork && (
        <CreateEgressDialog
          tenantId={tenantId}
          networkId={selectedNetwork}
          onClose={() => setShowCreate(false)}
          onCreated={load}
        />
      )}
    </div>
  )
}
