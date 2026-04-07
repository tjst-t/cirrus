import { useEffect, useState } from 'react'
import {
  networksApi,
  type Network,
  type NetworkGroup,
  type NetworkPolicy,
  type CreateNetworkRequest,
  type CreateNetworkGroupRequest,
  type CreateNetworkPolicyRequest,
} from '@/api/networks'
import { Button } from '@/components/Button'
import { ErrorMessage } from '@/components/ErrorMessage'

// ---- Create Network Dialog ----
function CreateNetworkDialog({ onClose, onCreated }: { onClose: () => void; onCreated: () => void }) {
  const [form, setForm] = useState<CreateNetworkRequest>({ name: '', cidr: '' })
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setLoading(true)
    try {
      await networksApi.create(form)
      onCreated()
      onClose()
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'エラーが発生しました')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      <div className="absolute inset-0 bg-black/40" onClick={onClose} />
      <div className="relative bg-white rounded-xl border border-[var(--color-border)] p-6 w-full max-w-md shadow-lg">
        <h3 className="text-base font-semibold text-[var(--color-text)] mb-4">ネットワークを作成</h3>
        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label className="block text-xs font-medium text-[var(--color-text)] mb-1">名前 *</label>
            <input
              type="text"
              value={form.name}
              onChange={(e) => setForm((p) => ({ ...p, name: e.target.value }))}
              required
              className="w-full h-9 px-3 text-sm border border-[var(--color-border)] rounded focus:outline-none focus:ring-2 focus:ring-accent/30"
              placeholder="my-network"
            />
          </div>
          <div>
            <label className="block text-xs font-medium text-[var(--color-text)] mb-1">CIDR *</label>
            <input
              type="text"
              value={form.cidr}
              onChange={(e) => setForm((p) => ({ ...p, cidr: e.target.value }))}
              required
              className="w-full h-9 px-3 text-sm border border-[var(--color-border)] rounded focus:outline-none focus:ring-2 focus:ring-accent/30"
              placeholder="10.0.0.0/24"
            />
          </div>
          {error && <p className="text-xs text-danger">{error}</p>}
          <div className="flex gap-2 justify-end pt-2">
            <Button type="button" variant="ghost" size="sm" onClick={onClose}>キャンセル</Button>
            <Button type="submit" variant="primary" size="sm" disabled={loading}>
              {loading ? '作成中...' : '作成'}
            </Button>
          </div>
        </form>
      </div>
    </div>
  )
}

// ---- Groups Panel ----
function GroupsPanel({ networkId }: { networkId: string }) {
  const [groups, setGroups] = useState<NetworkGroup[]>([])
  const [loading, setLoading] = useState(true)
  const [showCreate, setShowCreate] = useState(false)
  const [form, setForm] = useState<CreateNetworkGroupRequest>({ name: '' })
  const [creating, setCreating] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [deleteGroupId, setDeleteGroupId] = useState<string | null>(null)

  const load = () => {
    setLoading(true)
    networksApi.listGroups(networkId)
      .then(setGroups)
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false))
  }

  useEffect(() => { load() }, [networkId])

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault()
    setCreating(true)
    try {
      await networksApi.createGroup(networkId, form)
      setForm({ name: '' })
      setShowCreate(false)
      load()
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'エラーが発生しました')
    } finally {
      setCreating(false)
    }
  }

  const handleDelete = async (groupId: string) => {
    try {
      await networksApi.deleteGroup(networkId, groupId)
      setDeleteGroupId(null)
      load()
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'エラーが発生しました')
    }
  }

  const deleteTarget = groups.find((g) => g.id === deleteGroupId)

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        <span className="text-xs font-medium text-[var(--color-text-secondary)]">グループ</span>
        <button
          onClick={() => setShowCreate((s) => !s)}
          className="text-xs text-accent hover:underline"
        >
          + 追加
        </button>
      </div>
      {error && <p className="text-xs text-danger">{error}</p>}
      {showCreate && (
        <form onSubmit={handleCreate} className="flex gap-2">
          <input
            type="text"
            value={form.name}
            onChange={(e) => setForm({ name: e.target.value })}
            required
            className="flex-1 h-7 px-2 text-xs border border-[var(--color-border)] rounded focus:outline-none focus:ring-2 focus:ring-accent/30"
            placeholder="グループ名"
          />
          <Button type="submit" size="sm" variant="primary" disabled={creating}>追加</Button>
          <Button type="button" size="sm" variant="ghost" onClick={() => setShowCreate(false)}>✕</Button>
        </form>
      )}
      {loading ? (
        <p className="text-xs text-[var(--color-text-secondary)]">読み込み中...</p>
      ) : groups.length === 0 ? (
        <p className="text-xs text-[var(--color-text-secondary)]">グループなし</p>
      ) : (
        <ul className="space-y-1">
          {groups.map((g) => (
            <li key={g.id} className="flex items-center justify-between px-2 py-1 bg-[var(--color-bg-secondary)] rounded text-xs">
              <span>{g.name}</span>
              <button
                onClick={() => setDeleteGroupId(g.id)}
                className="text-danger hover:underline text-xs"
              >
                削除
              </button>
            </li>
          ))}
        </ul>
      )}

      {deleteGroupId && (
        <div className="fixed inset-0 z-50 flex items-center justify-center">
          <div className="absolute inset-0 bg-black/40" onClick={() => setDeleteGroupId(null)} />
          <div className="relative bg-white rounded-xl border border-[var(--color-border)] p-6 w-full max-w-sm shadow-lg">
            <h3 className="text-base font-semibold text-[var(--color-text)] mb-2">グループを削除</h3>
            <p className="text-sm text-[var(--color-text-secondary)] mb-4">
              {deleteTarget ? `「${deleteTarget.name}」` : 'このグループ'}を削除してもよろしいですか？この操作は取り消せません。
            </p>
            <div className="flex gap-2 justify-end">
              <Button variant="ghost" size="sm" onClick={() => setDeleteGroupId(null)}>キャンセル</Button>
              <Button variant="danger" size="sm" onClick={() => handleDelete(deleteGroupId)}>削除する</Button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

// ---- Policies Panel ----
function PoliciesPanel({ networkId }: { networkId: string }) {
  const [policies, setPolicies] = useState<NetworkPolicy[]>([])
  const [loading, setLoading] = useState(true)
  const [showCreate, setShowCreate] = useState(false)
  const [form, setForm] = useState<CreateNetworkPolicyRequest>({
    name: '',
    direction: 'ingress',
    protocol: 'tcp',
    action: 'allow',
  })
  const [creating, setCreating] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [deletePolicyId, setDeletePolicyId] = useState<string | null>(null)

  const load = () => {
    setLoading(true)
    networksApi.listPolicies(networkId)
      .then(setPolicies)
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false))
  }

  useEffect(() => { load() }, [networkId])

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault()
    setCreating(true)
    try {
      await networksApi.createPolicy(networkId, form)
      setForm({ name: '', direction: 'ingress', protocol: 'tcp', action: 'allow' })
      setShowCreate(false)
      load()
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'エラーが発生しました')
    } finally {
      setCreating(false)
    }
  }

  const handleDelete = async (policyId: string) => {
    try {
      await networksApi.deletePolicy(networkId, policyId)
      setDeletePolicyId(null)
      load()
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'エラーが発生しました')
    }
  }

  const deleteTarget = policies.find((p) => p.id === deletePolicyId)

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        <span className="text-xs font-medium text-[var(--color-text-secondary)]">ポリシー</span>
        <button
          onClick={() => setShowCreate((s) => !s)}
          className="text-xs text-accent hover:underline"
        >
          + 追加
        </button>
      </div>
      {error && <p className="text-xs text-danger">{error}</p>}
      {showCreate && (
        <form onSubmit={handleCreate} className="space-y-2 p-3 bg-[var(--color-bg-secondary)] rounded-lg">
          <div className="grid grid-cols-2 gap-2">
            <div>
              <label className="text-xs text-[var(--color-text-secondary)]">名前</label>
              <input
                type="text"
                value={form.name}
                onChange={(e) => setForm((p) => ({ ...p, name: e.target.value }))}
                required
                className="w-full h-7 px-2 text-xs border border-[var(--color-border)] rounded focus:outline-none focus:ring-1 focus:ring-accent/30 bg-white"
              />
            </div>
            <div>
              <label className="text-xs text-[var(--color-text-secondary)]">方向</label>
              <select
                value={form.direction}
                onChange={(e) => setForm((p) => ({ ...p, direction: e.target.value as 'ingress' | 'egress' }))}
                className="w-full h-7 px-2 text-xs border border-[var(--color-border)] rounded focus:outline-none bg-white"
              >
                <option value="ingress">ingress</option>
                <option value="egress">egress</option>
              </select>
            </div>
            <div>
              <label className="text-xs text-[var(--color-text-secondary)]">プロトコル</label>
              <input
                type="text"
                value={form.protocol}
                onChange={(e) => setForm((p) => ({ ...p, protocol: e.target.value }))}
                className="w-full h-7 px-2 text-xs border border-[var(--color-border)] rounded focus:outline-none focus:ring-1 focus:ring-accent/30 bg-white"
                placeholder="tcp"
              />
            </div>
            <div>
              <label className="text-xs text-[var(--color-text-secondary)]">アクション</label>
              <select
                value={form.action}
                onChange={(e) => setForm((p) => ({ ...p, action: e.target.value as 'allow' | 'deny' }))}
                className="w-full h-7 px-2 text-xs border border-[var(--color-border)] rounded focus:outline-none bg-white"
              >
                <option value="allow">allow</option>
                <option value="deny">deny</option>
              </select>
            </div>
            <div>
              <label className="text-xs text-[var(--color-text-secondary)]">ポート（最小）</label>
              <input
                type="number"
                value={form.port_range_min ?? ''}
                onChange={(e) => setForm((p) => ({ ...p, port_range_min: e.target.value ? Number(e.target.value) : undefined }))}
                className="w-full h-7 px-2 text-xs border border-[var(--color-border)] rounded focus:outline-none focus:ring-1 focus:ring-accent/30 bg-white"
                placeholder="80"
              />
            </div>
            <div>
              <label className="text-xs text-[var(--color-text-secondary)]">ポート（最大）</label>
              <input
                type="number"
                value={form.port_range_max ?? ''}
                onChange={(e) => setForm((p) => ({ ...p, port_range_max: e.target.value ? Number(e.target.value) : undefined }))}
                className="w-full h-7 px-2 text-xs border border-[var(--color-border)] rounded focus:outline-none focus:ring-1 focus:ring-accent/30 bg-white"
                placeholder="80"
              />
            </div>
          </div>
          <div>
            <label className="text-xs text-[var(--color-text-secondary)]">リモート CIDR</label>
            <input
              type="text"
              value={form.remote_cidr ?? ''}
              onChange={(e) => setForm((p) => ({ ...p, remote_cidr: e.target.value || undefined }))}
              className="w-full h-7 px-2 text-xs border border-[var(--color-border)] rounded focus:outline-none focus:ring-1 focus:ring-accent/30 bg-white"
              placeholder="0.0.0.0/0"
            />
          </div>
          <div className="flex gap-2 justify-end">
            <Button type="button" size="sm" variant="ghost" onClick={() => setShowCreate(false)}>キャンセル</Button>
            <Button type="submit" size="sm" variant="primary" disabled={creating}>追加</Button>
          </div>
        </form>
      )}
      {loading ? (
        <p className="text-xs text-[var(--color-text-secondary)]">読み込み中...</p>
      ) : policies.length === 0 ? (
        <p className="text-xs text-[var(--color-text-secondary)]">ポリシーなし</p>
      ) : (
        <ul className="space-y-1">
          {policies.map((p) => (
            <li key={p.id} className="flex items-center justify-between px-2 py-1 bg-[var(--color-bg-secondary)] rounded text-xs gap-2">
              <span className="font-medium">{p.name}</span>
              <span className="text-[var(--color-text-secondary)]">
                {p.direction} / {p.protocol}
                {p.port_range_min != null ? ` : ${p.port_range_min}` : ''}
                {p.port_range_max != null && p.port_range_max !== p.port_range_min ? `-${p.port_range_max}` : ''}
              </span>
              <span className={p.action === 'allow' ? 'text-success' : 'text-danger'}>{p.action}</span>
              <button
                onClick={() => setDeletePolicyId(p.id)}
                className="text-danger hover:underline text-xs shrink-0"
              >
                削除
              </button>
            </li>
          ))}
        </ul>
      )}

      {deletePolicyId && (
        <div className="fixed inset-0 z-50 flex items-center justify-center">
          <div className="absolute inset-0 bg-black/40" onClick={() => setDeletePolicyId(null)} />
          <div className="relative bg-white rounded-xl border border-[var(--color-border)] p-6 w-full max-w-sm shadow-lg">
            <h3 className="text-base font-semibold text-[var(--color-text)] mb-2">ポリシーを削除</h3>
            <p className="text-sm text-[var(--color-text-secondary)] mb-4">
              {deleteTarget ? `「${deleteTarget.name}」` : 'このポリシー'}を削除してもよろしいですか？この操作は取り消せません。
            </p>
            <div className="flex gap-2 justify-end">
              <Button variant="ghost" size="sm" onClick={() => setDeletePolicyId(null)}>キャンセル</Button>
              <Button variant="danger" size="sm" onClick={() => handleDelete(deletePolicyId)}>削除する</Button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

// ---- Main Page ----
export function NetworksPage() {
  const [networks, setNetworks] = useState<Network[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [showCreate, setShowCreate] = useState(false)
  const [expandedId, setExpandedId] = useState<string | null>(null)
  const [deleteId, setDeleteId] = useState<string | null>(null)

  const load = () => {
    setLoading(true)
    networksApi.list()
      .then(setNetworks)
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false))
  }

  useEffect(() => { load() }, [])

  const handleDelete = async (id: string) => {
    try {
      await networksApi.delete(id)
      setDeleteId(null)
      load()
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'エラーが発生しました')
    }
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold text-[var(--color-text)]">ネットワーク管理</h2>
        <Button variant="primary" size="sm" onClick={() => setShowCreate(true)}>
          + ネットワークを作成
        </Button>
      </div>

      {error && <ErrorMessage message={error} />}

      {loading ? (
        <div className="flex items-center justify-center h-40 text-[var(--color-text-secondary)] text-sm">読み込み中...</div>
      ) : (
        <div className="space-y-2">
          {networks.length === 0 ? (
            <div className="bg-white rounded-xl border border-[var(--color-border)] p-8 text-center text-sm text-[var(--color-text-secondary)]">
              ネットワークがありません
            </div>
          ) : (
            networks.map((net) => (
              <div key={net.id} className="bg-white rounded-xl border border-[var(--color-border)]">
                <div
                  className="flex items-center justify-between px-4 py-3 cursor-pointer"
                  onClick={() => setExpandedId(expandedId === net.id ? null : net.id)}
                >
                  <div className="flex items-center gap-3">
                    <span className="text-sm font-medium text-[var(--color-text)]">{net.name}</span>
                    <span className="text-xs text-[var(--color-text-secondary)] font-mono">{net.cidr}</span>
                    <span className="text-xs px-2 py-0.5 rounded bg-[var(--color-bg-secondary)] text-[var(--color-text-secondary)]">
                      {net.status}
                    </span>
                  </div>
                  <div className="flex items-center gap-2">
                    <Button
                      variant="danger"
                      size="sm"
                      onClick={(e) => { e.stopPropagation(); setDeleteId(net.id) }}
                    >
                      削除
                    </Button>
                    <span className="text-xs text-[var(--color-text-secondary)]">
                      {expandedId === net.id ? '▲' : '▼'}
                    </span>
                  </div>
                </div>
                {expandedId === net.id && (
                  <div className="border-t border-[var(--color-border)] px-4 py-4 grid grid-cols-2 gap-6">
                    <GroupsPanel networkId={net.id} />
                    <PoliciesPanel networkId={net.id} />
                  </div>
                )}
              </div>
            ))
          )}
        </div>
      )}

      {showCreate && (
        <CreateNetworkDialog onClose={() => setShowCreate(false)} onCreated={load} />
      )}

      {deleteId && (
        <div className="fixed inset-0 z-50 flex items-center justify-center">
          <div className="absolute inset-0 bg-black/40" onClick={() => setDeleteId(null)} />
          <div className="relative bg-white rounded-xl border border-[var(--color-border)] p-6 w-full max-w-sm shadow-lg">
            <h3 className="text-base font-semibold text-[var(--color-text)] mb-2">ネットワークを削除</h3>
            <p className="text-sm text-[var(--color-text-secondary)] mb-4">
              このネットワークを削除してもよろしいですか？この操作は取り消せません。
            </p>
            <div className="flex gap-2 justify-end">
              <Button variant="ghost" size="sm" onClick={() => setDeleteId(null)}>キャンセル</Button>
              <Button variant="danger" size="sm" onClick={() => handleDelete(deleteId)}>削除する</Button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
