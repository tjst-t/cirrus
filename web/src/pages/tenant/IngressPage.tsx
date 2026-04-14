import { useCallback, useEffect, useState } from 'react'
import { ingressApi, type Ingress, type IpPool } from '@/api/ingress'
import { networksApi, type Network } from '@/api/networks'
import { vmsApi, type Vm } from '@/api/vms'
import { Button } from '@/components/Button'
import { ErrorMessage } from '@/components/ErrorMessage'

// ---- Create Ingress Dialog ----
function CreateIngressDialog({
  networkId,
  ipPools,
  vms,
  onClose,
  onCreated,
  onError,
}: {
  networkId: string
  ipPools: IpPool[]
  vms: Vm[]
  onClose: () => void
  onCreated: () => void
  onError: (msg: string) => void
}) {
  const [ipPoolId, setIpPoolId] = useState('')
  const [publicIp, setPublicIp] = useState('')
  const [targetVmId, setTargetVmId] = useState('')
  const [targetIp, setTargetIp] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!ipPoolId) {
      setError('IP プールを選択してください')
      return
    }
    if (!publicIp.trim()) {
      setError('パブリック IP を入力してください')
      return
    }
    if (!targetVmId && !targetIp.trim()) {
      setError('ターゲット VM またはターゲット IP を指定してください')
      return
    }
    setLoading(true)
    setError(null)
    try {
      await ingressApi.create(networkId, {
        type: 'direct_ip',
        public_ip: publicIp.trim(),
        ip_pool_id: ipPoolId,
        config: {
          target_vm_id: targetVmId || undefined,
          target_ip: targetVmId ? undefined : targetIp.trim(),
        },
      })
      onCreated()
      onClose()
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : 'エラーが発生しました'
      setError(msg)
      onError(msg)
    } finally {
      setLoading(false)
    }
  }

  return (
    <div data-testid="ingress-create-dialog" className="fixed inset-0 z-50 flex items-center justify-center">
      <div className="absolute inset-0 bg-black/40 backdrop-blur-sm" onClick={onClose} />
      <div className="relative bg-white rounded-2xl border border-[var(--color-border)] p-6 w-full max-w-md shadow-xl">
        <div className="flex items-center gap-3 mb-5">
          <h3 className="text-base font-semibold text-[var(--color-text)]">Ingress を作成</h3>
        </div>
        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label className="block text-xs font-medium text-[var(--color-text)] mb-1.5">IP プール</label>
            <select
              data-testid="ingress-ip-pool-select"
              value={ipPoolId}
              onChange={(e) => setIpPoolId(e.target.value)}
              className="w-full h-9 px-3 text-sm border border-[var(--color-border)] rounded-lg focus:outline-none focus:ring-2 focus:ring-accent/30 focus:border-accent/50 transition-all bg-white"
            >
              <option value="">選択してください</option>
              {ipPools.map((p) => (
                <option key={p.id} value={p.id}>{p.name} ({p.cidr})</option>
              ))}
            </select>
          </div>
          <div>
            <label className="block text-xs font-medium text-[var(--color-text)] mb-1.5">
              パブリック IP <span className="text-red-500">*</span>
            </label>
            <input
              data-testid="ingress-public-ip-input"
              type="text"
              value={publicIp}
              onChange={(e) => setPublicIp(e.target.value)}
              className="w-full h-9 px-3 text-sm border border-[var(--color-border)] rounded-lg focus:outline-none focus:ring-2 focus:ring-accent/30 focus:border-accent/50 transition-all"
              placeholder="203.0.113.1"
            />
          </div>
          <div>
            <label className="block text-xs font-medium text-[var(--color-text)] mb-1.5">ターゲット VM（任意）</label>
            <select
              data-testid="ingress-target-vm-select"
              value={targetVmId}
              onChange={(e) => setTargetVmId(e.target.value)}
              className="w-full h-9 px-3 text-sm border border-[var(--color-border)] rounded-lg focus:outline-none focus:ring-2 focus:ring-accent/30 focus:border-accent/50 transition-all bg-white"
            >
              <option value="">なし</option>
              {vms.map((v) => (
                <option key={v.id} value={v.id}>{v.name}</option>
              ))}
            </select>
          </div>
          {!targetVmId && (
            <div>
              <label className="block text-xs font-medium text-[var(--color-text)] mb-1.5">
                ターゲット IP <span className="text-red-500">*</span>
              </label>
              <input
                data-testid="ingress-target-ip-input"
                type="text"
                value={targetIp}
                onChange={(e) => setTargetIp(e.target.value)}
                className="w-full h-9 px-3 text-sm border border-[var(--color-border)] rounded-lg focus:outline-none focus:ring-2 focus:ring-accent/30 focus:border-accent/50 transition-all"
                placeholder="10.0.0.10"
              />
            </div>
          )}
          {error && <ErrorMessage message={error} data-testid="ingress-error-message" />}
          <div className="flex gap-2 justify-end pt-1">
            <Button type="button" variant="ghost" size="sm" onClick={onClose}>キャンセル</Button>
            <Button
              data-testid="ingress-create-submit"
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
export function IngressPage() {
  const [networks, setNetworks] = useState<Network[]>([])
  const [selectedNetwork, setSelectedNetwork] = useState<string>('')
  const [ingresses, setIngresses] = useState<Ingress[]>([])
  const [ipPools, setIpPools] = useState<IpPool[]>([])
  const [vms, setVms] = useState<Vm[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [showCreate, setShowCreate] = useState(false)
  const [deleteId, setDeleteId] = useState<string | null>(null)
  const [deleting, setDeleting] = useState(false)

  useEffect(() => {
    networksApi.list().then((ns) => {
      setNetworks(ns)
      if (ns.length > 0) setSelectedNetwork(ns[0].id)
    }).catch(() => {})
    ingressApi.listIpPools().then(setIpPools).catch(() => {})
  }, [])

  useEffect(() => {
    vmsApi.list().then(setVms).catch(() => {})
  }, [])

  const load = useCallback(() => {
    if (!selectedNetwork) return
    setLoading(true)
    setError(null)
    ingressApi.list(selectedNetwork)
      .then(setIngresses)
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false))
  }, [selectedNetwork])

  useEffect(() => {
    if (selectedNetwork) load()
  }, [load])

  const handleDelete = async (id: string) => {
    if (!selectedNetwork) return
    setDeleting(true)
    try {
      await ingressApi.delete(selectedNetwork, id)
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
          <h2 className="text-lg font-semibold text-[var(--color-text)]">Ingress 管理</h2>
          <p className="text-xs text-[var(--color-text-secondary)] mt-0.5">テナントのインバウンドエンドポイントを管理します</p>
        </div>
        <Button
          data-testid="ingress-create-button"
          variant="primary"
          size="sm"
          onClick={() => setShowCreate(true)}
          disabled={!selectedNetwork}
        >
          <svg className="w-3.5 h-3.5 mr-1.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2.5} d="M12 4v16m8-8H4" />
          </svg>
          Ingress を作成
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
          {networks.length === 0 && <option value="">ネットワークなし</option>}
          {networks.map((n) => (
            <option key={n.id} value={n.id}>{n.name}</option>
          ))}
        </select>
      </div>

      {/* Error */}
      {error && <ErrorMessage message={error} data-testid="ingress-error-message" />}

      {/* List */}
      {loading ? (
        <div className="flex items-center justify-center h-40 text-[var(--color-text-secondary)] text-sm">
          読み込み中...
        </div>
      ) : ingresses.length === 0 ? (
        <div
          data-testid="ingress-empty-state"
          className="bg-white rounded-2xl border border-dashed border-[var(--color-border)] p-12 text-center"
        >
          <p className="text-sm font-medium text-[var(--color-text)] mb-1">Ingress がありません</p>
          <p className="text-xs text-[var(--color-text-secondary)]">「Ingress を作成」ボタンから作成してください</p>
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
              {ingresses.map((ig) => (
                <tr
                  key={ig.id}
                  data-testid={`ingress-row-${ig.id}`}
                  className="border-t border-[var(--color-border)]"
                >
                  <td data-testid={`ingress-type-${ig.id}`} className="px-4 py-3 font-medium text-[var(--color-text)]">
                    {ig.type}
                  </td>
                  <td data-testid={`ingress-public-ip-${ig.id}`} className="px-4 py-3 text-[var(--color-text-secondary)]">
                    {ig.public_ip ?? '—'}
                  </td>
                  <td className="px-4 py-3 text-right">
                    {deleteId === ig.id ? (
                      <span className="flex items-center justify-end gap-2">
                        <span className="text-xs text-[var(--color-text-secondary)]">削除しますか？</span>
                        <Button
                          data-testid={`ingress-delete-confirm-${ig.id}`}
                          variant="danger"
                          size="sm"
                          disabled={deleting}
                          onClick={() => handleDelete(ig.id)}
                        >
                          削除する
                        </Button>
                        <Button
                          data-testid={`ingress-delete-cancel-${ig.id}`}
                          variant="ghost"
                          size="sm"
                          onClick={() => setDeleteId(null)}
                        >
                          キャンセル
                        </Button>
                      </span>
                    ) : (
                      <Button
                        data-testid={`ingress-delete-button-${ig.id}`}
                        variant="danger"
                        size="sm"
                        onClick={() => setDeleteId(ig.id)}
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
      )}

      {/* Create Dialog */}
      {showCreate && selectedNetwork && (
        <CreateIngressDialog
          networkId={selectedNetwork}
          ipPools={ipPools}
          vms={vms}
          onClose={() => setShowCreate(false)}
          onCreated={load}
          onError={(msg) => setError(msg)}
        />
      )}
    </div>
  )
}
