import { useCallback, useEffect, useState } from 'react'
import { ingressApi, type IngressEndpoint } from '@/api/ingress'
import { networksApi, type Network } from '@/api/networks'
import { vmsApi, type Vm } from '@/api/vms'
import { Button } from '@/components/Button'
import { ErrorMessage } from '@/components/ErrorMessage'
import { StatusBadge } from '@/components/tenant/StatusBadge'

export function IngressPage() {
  const [networks, setNetworks] = useState<Network[]>([])
  const [selectedNetwork, setSelectedNetwork] = useState<string>('')
  const [endpoints, setEndpoints] = useState<IngressEndpoint[]>([])
  const [vms, setVms] = useState<Vm[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [creating, setCreating] = useState(false)
  const [showCreate, setShowCreate] = useState(false)
  const [deleteId, setDeleteId] = useState<string | null>(null)
  const [form, setForm] = useState({ name: '', vm_id: '', port: '80', protocol: 'tcp' })

  useEffect(() => {
    networksApi.list().then((ns) => {
      setNetworks(ns)
      if (ns.length > 0) setSelectedNetwork(ns[0].id)
    }).catch(() => {})
    vmsApi.list().then(setVms).catch(() => {})
  }, [])

  const load = useCallback(() => {
    if (!selectedNetwork) return
    setLoading(true)
    setError(null)
    ingressApi.list(selectedNetwork)
      .then(setEndpoints)
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false))
  }, [selectedNetwork])

  useEffect(() => { load() }, [load])

  const handleCreate = async () => {
    if (!selectedNetwork || !form.name.trim()) return
    setCreating(true)
    try {
      await ingressApi.create(selectedNetwork, {
        name: form.name.trim(),
        vm_id: form.vm_id || undefined,
        port: parseInt(form.port, 10),
        protocol: form.protocol,
      })
      setShowCreate(false)
      setForm({ name: '', vm_id: '', port: '80', protocol: 'tcp' })
      load()
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'エラーが発生しました')
    } finally {
      setCreating(false)
    }
  }

  const handleDelete = async (id: string) => {
    if (!selectedNetwork) return
    try {
      await ingressApi.delete(selectedNetwork, id)
      setDeleteId(null)
      load()
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : '削除に失敗しました')
    }
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-xl font-semibold text-[var(--color-text)]">Ingress 管理</h1>
        <Button size="sm" onClick={() => setShowCreate(true)} disabled={!selectedNetwork}>+ 作成</Button>
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

      {showCreate && (
        <div className="mb-4 rounded-lg border border-[var(--color-border)] bg-white p-4 flex flex-col gap-3">
          <p className="font-medium text-sm">Ingress エンドポイントを作成</p>
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="block text-xs font-medium mb-1">名前</label>
              <input className="w-full rounded border border-[var(--color-border)] px-3 py-1.5 text-sm"
                value={form.name} onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))} />
            </div>
            <div>
              <label className="block text-xs font-medium mb-1">VM（任意）</label>
              <select className="w-full rounded border border-[var(--color-border)] px-3 py-1.5 text-sm bg-white"
                value={form.vm_id} onChange={(e) => setForm((f) => ({ ...f, vm_id: e.target.value }))}>
                <option value="">なし</option>
                {vms.map((v) => <option key={v.id} value={v.id}>{v.name}</option>)}
              </select>
            </div>
            <div>
              <label className="block text-xs font-medium mb-1">ポート</label>
              <input type="number" className="w-full rounded border border-[var(--color-border)] px-3 py-1.5 text-sm"
                value={form.port} onChange={(e) => setForm((f) => ({ ...f, port: e.target.value }))} />
            </div>
            <div>
              <label className="block text-xs font-medium mb-1">プロトコル</label>
              <select className="w-full rounded border border-[var(--color-border)] px-3 py-1.5 text-sm bg-white"
                value={form.protocol} onChange={(e) => setForm((f) => ({ ...f, protocol: e.target.value }))}>
                <option value="tcp">TCP</option>
                <option value="udp">UDP</option>
              </select>
            </div>
          </div>
          <div className="flex justify-end gap-2">
            <Button variant="secondary" size="sm" onClick={() => setShowCreate(false)}>キャンセル</Button>
            <Button size="sm" onClick={handleCreate} disabled={creating || !form.name.trim()}>
              {creating ? '作成中...' : '作成'}
            </Button>
          </div>
        </div>
      )}

      {loading ? (
        <p className="text-sm text-[var(--color-text-secondary)]">読み込み中...</p>
      ) : endpoints.length === 0 ? (
        <div className="rounded-lg border border-[var(--color-border)] bg-white p-8 text-center">
          <p className="text-sm text-[var(--color-text-secondary)]">Ingress エンドポイントがありません</p>
        </div>
      ) : (
        <div className="rounded-lg border border-[var(--color-border)] bg-white overflow-hidden">
          <table className="w-full text-sm">
            <thead className="bg-[var(--color-bg-secondary)]">
              <tr>
                <th className="text-left px-4 py-2.5 font-medium text-[var(--color-text-secondary)]">名前</th>
                <th className="text-left px-4 py-2.5 font-medium text-[var(--color-text-secondary)]">ポート/プロトコル</th>
                <th className="text-left px-4 py-2.5 font-medium text-[var(--color-text-secondary)]">パブリック</th>
                <th className="text-left px-4 py-2.5 font-medium text-[var(--color-text-secondary)]">状態</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {endpoints.map((ep) => (
                <tr key={ep.id} className="border-t border-[var(--color-border)]">
                  <td className="px-4 py-3 font-medium">{ep.name}</td>
                  <td className="px-4 py-3 text-[var(--color-text-secondary)]">{ep.port}/{ep.protocol}</td>
                  <td className="px-4 py-3 text-[var(--color-text-secondary)]">
                    {ep.public_ip ? `${ep.public_ip}:${ep.public_port}` : '—'}
                  </td>
                  <td className="px-4 py-3"><StatusBadge status={ep.status} /></td>
                  <td className="px-4 py-3 text-right">
                    {deleteId === ep.id ? (
                      <span className="flex items-center justify-end gap-2">
                        <span className="text-xs text-[var(--color-text-secondary)]">削除しますか？</span>
                        <Button variant="danger" size="sm" onClick={() => handleDelete(ep.id)}>確認</Button>
                        <Button variant="secondary" size="sm" onClick={() => setDeleteId(null)}>キャンセル</Button>
                      </span>
                    ) : (
                      <Button variant="danger" size="sm" onClick={() => setDeleteId(ep.id)}>削除</Button>
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
