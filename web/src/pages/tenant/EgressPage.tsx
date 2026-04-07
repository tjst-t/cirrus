import { useCallback, useEffect, useState } from 'react'
import { egressApi, type EgressGateway } from '@/api/egress'
import { networksApi, type Network } from '@/api/networks'
import { useTenant } from '@/hooks/useTenant'
import { Button } from '@/components/Button'
import { ErrorMessage } from '@/components/ErrorMessage'
import { StatusBadge } from '@/components/tenant/StatusBadge'

export function EgressPage() {
  const { tenantId } = useTenant()
  const [networks, setNetworks] = useState<Network[]>([])
  const [selectedNetwork, setSelectedNetwork] = useState<string>('')
  const [gateways, setGateways] = useState<EgressGateway[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [creating, setCreating] = useState(false)
  const [newName, setNewName] = useState('')
  const [deleteId, setDeleteId] = useState<string | null>(null)

  useEffect(() => {
    networksApi.list().then((ns) => {
      setNetworks(ns)
      if (ns.length > 0) setSelectedNetwork(ns[0].id)
    }).catch(() => {})
  }, [])

  const load = useCallback(() => {
    if (!tenantId || !selectedNetwork) return
    setLoading(true)
    setError(null)
    egressApi.list(tenantId, selectedNetwork)
      .then(setGateways)
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false))
  }, [tenantId, selectedNetwork])

  useEffect(() => { load() }, [load])

  const handleCreate = async () => {
    if (!tenantId || !selectedNetwork || !newName.trim()) return
    setCreating(true)
    try {
      await egressApi.create(tenantId, selectedNetwork, { name: newName.trim(), network_id: selectedNetwork })
      setNewName('')
      load()
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'エラーが発生しました')
    } finally {
      setCreating(false)
    }
  }

  const handleDelete = async (id: string) => {
    if (!tenantId || !selectedNetwork) return
    try {
      await egressApi.delete(tenantId, selectedNetwork, id)
      setDeleteId(null)
      load()
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : '削除に失敗しました')
    }
  }

  if (!tenantId) {
    return <p className="text-sm text-[var(--color-text-secondary)]">テナントを選択してください</p>
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-xl font-semibold text-[var(--color-text)]">Egress 管理</h1>
      </div>

      <div className="mb-4 flex items-center gap-3">
        <label className="text-sm font-medium">ネットワーク</label>
        <select
          value={selectedNetwork}
          onChange={(e) => setSelectedNetwork(e.target.value)}
          className="rounded border border-[var(--color-border)] px-3 py-1.5 text-sm bg-white"
        >
          {networks.length === 0 && <option value="">ネットワークなし</option>}
          {networks.map((n) => (
            <option key={n.id} value={n.id}>{n.name}</option>
          ))}
        </select>
      </div>

      {error && <div className="mb-4"><ErrorMessage message={error} /></div>}

      {selectedNetwork && (
        <div className="mb-4 flex items-center gap-2">
          <input
            value={newName}
            onChange={(e) => setNewName(e.target.value)}
            placeholder="ゲートウェイ名"
            className="rounded border border-[var(--color-border)] px-3 py-1.5 text-sm"
            onKeyDown={(e) => e.key === 'Enter' && handleCreate()}
          />
          <Button size="sm" onClick={handleCreate} disabled={creating || !newName.trim()}>
            {creating ? '作成中...' : '+ 作成'}
          </Button>
        </div>
      )}

      {loading ? (
        <p className="text-sm text-[var(--color-text-secondary)]">読み込み中...</p>
      ) : gateways.length === 0 ? (
        <div className="rounded-lg border border-[var(--color-border)] bg-white p-8 text-center">
          <p className="text-sm text-[var(--color-text-secondary)]">Egress ゲートウェイがありません</p>
        </div>
      ) : (
        <div className="rounded-lg border border-[var(--color-border)] bg-white overflow-hidden">
          <table className="w-full text-sm">
            <thead className="bg-[var(--color-bg-secondary)]">
              <tr>
                <th className="text-left px-4 py-2.5 font-medium text-[var(--color-text-secondary)]">名前</th>
                <th className="text-left px-4 py-2.5 font-medium text-[var(--color-text-secondary)]">パブリック IP</th>
                <th className="text-left px-4 py-2.5 font-medium text-[var(--color-text-secondary)]">状態</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {gateways.map((gw) => (
                <tr key={gw.id} className="border-t border-[var(--color-border)]">
                  <td className="px-4 py-3 font-medium">{gw.name}</td>
                  <td className="px-4 py-3 text-[var(--color-text-secondary)]">{gw.public_ip ?? '—'}</td>
                  <td className="px-4 py-3"><StatusBadge status={gw.status} /></td>
                  <td className="px-4 py-3 text-right">
                    {deleteId === gw.id ? (
                      <span className="flex items-center justify-end gap-2">
                        <span className="text-xs text-[var(--color-text-secondary)]">削除しますか？</span>
                        <Button variant="danger" size="sm" onClick={() => handleDelete(gw.id)}>確認</Button>
                        <Button variant="secondary" size="sm" onClick={() => setDeleteId(null)}>キャンセル</Button>
                      </span>
                    ) : (
                      <Button variant="danger" size="sm" onClick={() => setDeleteId(gw.id)}>削除</Button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
