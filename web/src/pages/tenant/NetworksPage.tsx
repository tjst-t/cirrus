import { useCallback, useEffect, useState } from 'react'
import {
  networksApi,
  type Network,
  type NetworkGroup,
  type NetworkPolicy,
} from '@/api/networks'
import { Button } from '@/components/Button'
import { ErrorMessage } from '@/components/ErrorMessage'

// ---- Status Badge ----
function StatusBadge({ status }: { status: string }) {
  const colorMap: Record<string, string> = {
    active: 'bg-emerald-50 text-emerald-700 border-emerald-200',
    pending: 'bg-amber-50 text-amber-700 border-amber-200',
    error: 'bg-red-50 text-red-700 border-red-200',
    deleting: 'bg-slate-50 text-slate-500 border-slate-200',
  }
  const cls = colorMap[status] ?? 'bg-slate-50 text-slate-500 border-slate-200'
  return (
    <span className={`inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium border ${cls}`}>
      {status}
    </span>
  )
}

// ---- Create Network Dialog ----
function CreateNetworkDialog({ onClose, onCreated }: { onClose: () => void; onCreated: (net: Network) => void }) {
  const [name, setName] = useState('')
  const [cidr, setCidr] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setLoading(true)
    try {
      const req: { name: string; cidr?: string } = { name }
      if (cidr.trim()) req.cidr = cidr.trim()
      const created = await networksApi.create(req)
      onCreated(created)
      onClose()
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'エラーが発生しました')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div data-testid="network-create-dialog" className="fixed inset-0 z-50 flex items-center justify-center">
      <div className="absolute inset-0 bg-black/40 backdrop-blur-sm" onClick={onClose} />
      <div className="relative bg-white rounded-2xl border border-[var(--color-border)] p-6 w-full max-w-md shadow-xl">
        <div className="flex items-center gap-3 mb-5">
          <div className="w-8 h-8 rounded-lg bg-accent/10 flex items-center justify-center">
            <svg className="w-4 h-4 text-accent" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4v16m8-8H4" />
            </svg>
          </div>
          <h3 className="text-base font-semibold text-[var(--color-text)]">ネットワークを作成</h3>
        </div>
        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label className="block text-xs font-medium text-[var(--color-text)] mb-1.5">名前 <span className="text-red-500">*</span></label>
            <input
              data-testid="network-create-name"
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
              required
              className="w-full h-9 px-3 text-sm border border-[var(--color-border)] rounded-lg focus:outline-none focus:ring-2 focus:ring-accent/30 focus:border-accent/50 transition-all"
              placeholder="my-network"
            />
          </div>
          <div>
            <label className="block text-xs font-medium text-[var(--color-text)] mb-1.5">CIDR</label>
            <input
              data-testid="network-create-cidr"
              type="text"
              value={cidr}
              onChange={(e) => setCidr(e.target.value)}
              className="w-full h-9 px-3 text-sm border border-[var(--color-border)] rounded-lg focus:outline-none focus:ring-2 focus:ring-accent/30 focus:border-accent/50 transition-all"
              placeholder="省略可（自動割り当て）"
            />
          </div>
          {error && <p className="text-xs text-red-500">{error}</p>}
          <div className="flex gap-2 justify-end pt-1">
            <Button type="button" variant="ghost" size="sm" onClick={onClose}>キャンセル</Button>
            <Button data-testid="network-create-submit" type="submit" variant="primary" size="sm" disabled={loading}>
              {loading ? '作成中...' : '作成'}
            </Button>
          </div>
        </form>
      </div>
    </div>
  )
}

// ---- Groups Panel ----
function GroupsPanel({ networkId, onGroupsChanged }: { networkId: string; onGroupsChanged: (groups: NetworkGroup[]) => void }) {
  const [groups, setGroups] = useState<NetworkGroup[]>([])
  const [loading, setLoading] = useState(true)
  const [showCreate, setShowCreate] = useState(false)
  const [groupName, setGroupName] = useState('')
  const [creating, setCreating] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [deleteGroupId, setDeleteGroupId] = useState<string | null>(null)

  const load = useCallback(() => {
    setLoading(true)
    networksApi.listGroups(networkId)
      .then((g) => { setGroups(g); onGroupsChanged(g) })
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false))
  }, [networkId, onGroupsChanged])

  useEffect(() => { load() }, [load])

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault()
    setCreating(true)
    try {
      await networksApi.createGroup(networkId, { name: groupName })
      setGroupName('')
      setShowCreate(false)
      load()
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'エラーが発生しました')
    } finally {
      setCreating(false)
    }
  }

  const handleDelete = async (groupId: string) => {
    setDeleting(true)
    try {
      await networksApi.deleteGroup(networkId, groupId)
      setDeleteGroupId(null)
      load()
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'エラーが発生しました')
    } finally {
      setDeleting(false)
    }
  }

  const deleteTarget = groups.find((g) => g.id === deleteGroupId)

  return (
    <div data-testid={`groups-panel-${networkId}`} className="space-y-3">
      <div className="flex items-center justify-between">
        <span className="text-xs font-semibold text-[var(--color-text-secondary)] uppercase tracking-wider">グループ</span>
        <button
          data-testid={`add-group-button-${networkId}`}
          onClick={() => setShowCreate((s) => !s)}
          className="inline-flex items-center gap-1 text-xs text-accent hover:text-accent/80 font-medium transition-colors"
        >
          <svg className="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2.5} d="M12 4v16m8-8H4" />
          </svg>
          追加
        </button>
      </div>
      {error && <p className="text-xs text-red-500">{error}</p>}
      {showCreate && (
        <form onSubmit={handleCreate} className="flex gap-2 items-center">
          <input
            data-testid={`group-name-input-${networkId}`}
            type="text"
            value={groupName}
            onChange={(e) => setGroupName(e.target.value)}
            required
            autoFocus
            className="flex-1 h-7 px-2 text-xs border border-[var(--color-border)] rounded-md focus:outline-none focus:ring-2 focus:ring-accent/30 focus:border-accent/50 transition-all"
            placeholder="グループ名"
          />
          <Button data-testid={`group-create-submit-${networkId}`} type="submit" size="sm" variant="primary" disabled={creating}>追加</Button>
          <Button type="button" size="sm" variant="ghost" onClick={() => setShowCreate(false)}>✕</Button>
        </form>
      )}
      {loading ? (
        <p className="text-xs text-[var(--color-text-secondary)]">読み込み中...</p>
      ) : groups.length === 0 ? (
        <p data-testid={`group-empty-${networkId}`} className="text-xs text-[var(--color-text-secondary)] italic py-1">グループなし</p>
      ) : (
        <ul className="space-y-1">
          {groups.map((g) => (
            <li
              key={g.id}
              data-testid={`group-item-${g.id}`}
              className="flex items-center justify-between px-2.5 py-1.5 bg-[var(--color-bg-secondary)] rounded-lg text-xs group"
            >
              <div className="flex items-center gap-2">
                <div className="w-1.5 h-1.5 rounded-full bg-accent/60" />
                <span className="font-medium text-[var(--color-text)]">{g.name}</span>
              </div>
              <button
                data-testid={`group-delete-${g.id}`}
                onClick={() => setDeleteGroupId(g.id)}
                className="text-red-400 hover:text-red-600 text-xs opacity-0 group-hover:opacity-100 transition-opacity"
              >
                削除
              </button>
            </li>
          ))}
        </ul>
      )}

      {deleteGroupId && (
        <div data-testid="group-delete-dialog" className="fixed inset-0 z-50 flex items-center justify-center">
          <div className="absolute inset-0 bg-black/40 backdrop-blur-sm" onClick={() => setDeleteGroupId(null)} />
          <div className="relative bg-white rounded-2xl border border-[var(--color-border)] p-6 w-full max-w-sm shadow-xl">
            <h3 className="text-base font-semibold text-[var(--color-text)] mb-2">グループを削除</h3>
            <p className="text-sm text-[var(--color-text-secondary)] mb-4">
              {deleteTarget ? `「${deleteTarget.name}」` : 'このグループ'}を削除してもよろしいですか？この操作は取り消せません。
            </p>
            <div className="flex gap-2 justify-end">
              <Button variant="ghost" size="sm" onClick={() => setDeleteGroupId(null)}>キャンセル</Button>
              <Button data-testid="group-delete-confirm" variant="danger" size="sm" disabled={deleting} onClick={() => handleDelete(deleteGroupId)}>削除する</Button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

// ---- Policies Panel ----
function PoliciesPanel({ networkId, groups }: { networkId: string; groups: NetworkGroup[] }) {
  const [policies, setPolicies] = useState<NetworkPolicy[]>([])
  const [loading, setLoading] = useState(true)
  const [showCreate, setShowCreate] = useState(false)
  const [srcGroupId, setSrcGroupId] = useState('')
  const [dstGroupId, setDstGroupId] = useState('')
  const [protocol, setProtocol] = useState('tcp')
  const [dstPort, setDstPort] = useState('')
  const [priority, setPriority] = useState('1000')
  const [action, setAction] = useState('allow')
  const [creating, setCreating] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [deletePolicyId, setDeletePolicyId] = useState<string | null>(null)

  const groupMap = Object.fromEntries(groups.map((g) => [g.id, g.name]))

  const load = useCallback(() => {
    setLoading(true)
    networksApi.listPolicies(networkId)
      .then(setPolicies)
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false))
  }, [networkId])

  useEffect(() => { load() }, [load])

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault()
    setCreating(true)
    try {
      const req: {
        src_group_id: string
        dst_group_id: string
        protocol: string
        dst_port?: number
        priority?: number
        action?: string
      } = {
        src_group_id: srcGroupId,
        dst_group_id: dstGroupId,
        protocol,
        action,
      }
      if (dstPort.trim()) req.dst_port = Number(dstPort)
      if (priority.trim()) req.priority = Number(priority)
      await networksApi.createPolicy(networkId, req)
      setSrcGroupId('')
      setDstGroupId('')
      setProtocol('tcp')
      setDstPort('')
      setPriority('1000')
      setAction('allow')
      setShowCreate(false)
      load()
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'エラーが発生しました')
    } finally {
      setCreating(false)
    }
  }

  const handleDelete = async (policyId: string) => {
    setDeleting(true)
    try {
      await networksApi.deletePolicy(networkId, policyId)
      setDeletePolicyId(null)
      load()
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'エラーが発生しました')
    } finally {
      setDeleting(false)
    }
  }

  const hasGroups = groups.length >= 2

  return (
    <div data-testid={`policies-panel-${networkId}`} className="space-y-3">
      <div className="flex items-center justify-between">
        <span className="text-xs font-semibold text-[var(--color-text-secondary)] uppercase tracking-wider">ポリシー</span>
        <button
          data-testid={`add-policy-button-${networkId}`}
          onClick={() => setShowCreate((s) => !s)}
          disabled={!hasGroups}
          className="inline-flex items-center gap-1 text-xs text-accent hover:text-accent/80 font-medium transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
          title={!hasGroups ? 'グループが2つ以上必要です' : undefined}
        >
          <svg className="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2.5} d="M12 4v16m8-8H4" />
          </svg>
          追加
        </button>
      </div>
      {error && <p className="text-xs text-red-500">{error}</p>}

      {showCreate && (
        <form
          data-testid={`policy-form-${networkId}`}
          onSubmit={handleCreate}
          className="space-y-2 p-3 bg-[var(--color-bg-secondary)] rounded-xl border border-[var(--color-border)]"
        >
          <div className="grid grid-cols-2 gap-2">
            <div>
              <label className="text-xs text-[var(--color-text-secondary)] mb-1 block">送信元グループ</label>
              <select
                data-testid={`policy-src-group-${networkId}`}
                value={srcGroupId}
                onChange={(e) => setSrcGroupId(e.target.value)}
                required
                className="w-full h-7 px-2 text-xs border border-[var(--color-border)] rounded-md focus:outline-none bg-white"
              >
                <option value="">選択...</option>
                {groups.map((g) => (
                  <option key={g.id} value={g.id}>{g.name}</option>
                ))}
              </select>
            </div>
            <div>
              <label className="text-xs text-[var(--color-text-secondary)] mb-1 block">宛先グループ</label>
              <select
                data-testid={`policy-dst-group-${networkId}`}
                value={dstGroupId}
                onChange={(e) => setDstGroupId(e.target.value)}
                required
                className="w-full h-7 px-2 text-xs border border-[var(--color-border)] rounded-md focus:outline-none bg-white"
              >
                <option value="">選択...</option>
                {groups.map((g) => (
                  <option key={g.id} value={g.id}>{g.name}</option>
                ))}
              </select>
            </div>
            <div>
              <label className="text-xs text-[var(--color-text-secondary)] mb-1 block">プロトコル</label>
              <select
                data-testid={`policy-protocol-${networkId}`}
                value={protocol}
                onChange={(e) => setProtocol(e.target.value)}
                className="w-full h-7 px-2 text-xs border border-[var(--color-border)] rounded-md focus:outline-none bg-white"
              >
                <option value="tcp">tcp</option>
                <option value="udp">udp</option>
                <option value="icmp">icmp</option>
                <option value="any">any</option>
              </select>
            </div>
            <div>
              <label className="text-xs text-[var(--color-text-secondary)] mb-1 block">宛先ポート</label>
              <input
                data-testid={`policy-dst-port-${networkId}`}
                type="number"
                value={dstPort}
                onChange={(e) => setDstPort(e.target.value)}
                className="w-full h-7 px-2 text-xs border border-[var(--color-border)] rounded-md focus:outline-none focus:ring-1 focus:ring-accent/30 bg-white"
                placeholder="例: 80"
                min={1}
                max={65535}
              />
            </div>
            <div>
              <label className="text-xs text-[var(--color-text-secondary)] mb-1 block">優先度</label>
              <input
                data-testid={`policy-priority-${networkId}`}
                type="number"
                value={priority}
                onChange={(e) => setPriority(e.target.value)}
                className="w-full h-7 px-2 text-xs border border-[var(--color-border)] rounded-md focus:outline-none focus:ring-1 focus:ring-accent/30 bg-white"
                placeholder="1000"
              />
            </div>
            <div>
              <label className="text-xs text-[var(--color-text-secondary)] mb-1 block">アクション</label>
              <select
                data-testid={`policy-action-${networkId}`}
                value={action}
                onChange={(e) => setAction(e.target.value)}
                className="w-full h-7 px-2 text-xs border border-[var(--color-border)] rounded-md focus:outline-none bg-white"
              >
                <option value="allow">allow</option>
                <option value="deny">deny</option>
              </select>
            </div>
          </div>
          <div className="flex gap-2 justify-end pt-1">
            <Button type="button" size="sm" variant="ghost" onClick={() => setShowCreate(false)}>キャンセル</Button>
            <Button data-testid={`policy-create-submit-${networkId}`} type="submit" size="sm" variant="primary" disabled={creating}>追加</Button>
          </div>
        </form>
      )}

      {loading ? (
        <p className="text-xs text-[var(--color-text-secondary)]">読み込み中...</p>
      ) : policies.length === 0 ? (
        <p className="text-xs text-[var(--color-text-secondary)] italic py-1">ポリシーなし</p>
      ) : (
        <ul className="space-y-1">
          {policies.map((p) => (
            <li
              key={p.id}
              data-testid={`policy-item-${p.id}`}
              className="flex items-center justify-between px-2.5 py-1.5 bg-[var(--color-bg-secondary)] rounded-lg text-xs gap-2 group"
            >
              <div className="flex items-center gap-2 min-w-0 flex-1">
                <span className="font-medium text-[var(--color-text)] truncate">
                  {groupMap[p.src_group_id] ?? p.src_group_id}
                </span>
                <svg className="w-3 h-3 text-[var(--color-text-secondary)] shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M14 5l7 7m0 0l-7 7m7-7H3" />
                </svg>
                <span className="font-medium text-[var(--color-text)] truncate">
                  {groupMap[p.dst_group_id] ?? p.dst_group_id}
                </span>
                <span className="text-[var(--color-text-secondary)] shrink-0">{p.protocol}</span>
                {p.dst_port != null && (
                  <span className="text-[var(--color-text-secondary)] shrink-0">:{p.dst_port}</span>
                )}
              </div>
              <div className="flex items-center gap-2 shrink-0">
                <span className={`font-medium ${p.action === 'allow' ? 'text-emerald-600' : 'text-red-500'}`}>
                  {p.action}
                </span>
                <button
                  data-testid={`policy-delete-${p.id}`}
                  onClick={() => setDeletePolicyId(p.id)}
                  className="text-red-400 hover:text-red-600 text-xs opacity-0 group-hover:opacity-100 transition-opacity"
                >
                  削除
                </button>
              </div>
            </li>
          ))}
        </ul>
      )}

      {deletePolicyId && (
        <div data-testid="policy-delete-dialog" className="fixed inset-0 z-50 flex items-center justify-center">
          <div className="absolute inset-0 bg-black/40 backdrop-blur-sm" onClick={() => setDeletePolicyId(null)} />
          <div className="relative bg-white rounded-2xl border border-[var(--color-border)] p-6 w-full max-w-sm shadow-xl">
            <h3 className="text-base font-semibold text-[var(--color-text)] mb-2">ポリシーを削除</h3>
            <p className="text-sm text-[var(--color-text-secondary)] mb-4">
              このポリシーを削除してもよろしいですか？この操作は取り消せません。
            </p>
            <div className="flex gap-2 justify-end">
              <Button variant="ghost" size="sm" onClick={() => setDeletePolicyId(null)}>キャンセル</Button>
              <Button data-testid="policy-delete-confirm" variant="danger" size="sm" disabled={deleting} onClick={() => handleDelete(deletePolicyId)}>削除する</Button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

// ---- Expanded Network Panel ----
function NetworkExpandedPanel({ networkId }: { networkId: string }) {
  const [groups, setGroups] = useState<NetworkGroup[]>([])

  // GroupsPanel is the single source of truth for group data.
  // onGroupsChanged keeps this parent state in sync so PoliciesPanel
  // always reflects the latest group list (e.g. after add/delete).
  const handleGroupsChanged = useCallback((g: NetworkGroup[]) => {
    setGroups(g)
  }, [])

  return (
    <div className="border-t border-[var(--color-border)] px-5 py-4 grid grid-cols-2 gap-6">
      <GroupsPanel networkId={networkId} onGroupsChanged={handleGroupsChanged} />
      <PoliciesPanel networkId={networkId} groups={groups} />
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

  const load = useCallback(() => {
    setLoading(true)
    setError(null)
    networksApi.list()
      .then(setNetworks)
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => { load() }, [load])

  const handleDelete = async (id: string) => {
    try {
      await networksApi.delete(id)
      setDeleteId(null)
      setExpandedId(null)
      load()
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'エラーが発生しました')
    }
  }

  const deleteTarget = networks.find((n) => n.id === deleteId)

  return (
    <div className="space-y-5">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-lg font-semibold text-[var(--color-text)]">ネットワーク管理</h2>
          <p className="text-xs text-[var(--color-text-secondary)] mt-0.5">テナントの仮想ネットワーク、グループ、ポリシーを管理します</p>
        </div>
        <Button
          data-testid="create-network-button"
          variant="primary"
          size="sm"
          onClick={() => setShowCreate(true)}
        >
          <svg className="w-3.5 h-3.5 mr-1.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2.5} d="M12 4v16m8-8H4" />
          </svg>
          ネットワークを作成
        </Button>
      </div>

      {error && <ErrorMessage message={error} />}

      {loading ? (
        <div className="flex items-center justify-center h-40 text-[var(--color-text-secondary)] text-sm">
          読み込み中...
        </div>
      ) : (
        <div className="space-y-2">
          {networks.length === 0 ? (
            <div
              data-testid="network-empty-state"
              className="bg-white rounded-2xl border border-dashed border-[var(--color-border)] p-12 text-center"
            >
              <div className="w-12 h-12 rounded-xl bg-[var(--color-bg-secondary)] flex items-center justify-center mx-auto mb-3">
                <svg className="w-6 h-6 text-[var(--color-text-secondary)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M9 3H5a2 2 0 00-2 2v4m6-6h10a2 2 0 012 2v4M9 3v18m0 0h10a2 2 0 002-2V9M9 21H5a2 2 0 01-2-2V9m0 0h18" />
                </svg>
              </div>
              <p className="text-sm font-medium text-[var(--color-text)] mb-1">ネットワークがありません</p>
              <p className="text-xs text-[var(--color-text-secondary)]">「ネットワークを作成」ボタンから作成してください</p>
            </div>
          ) : (
            networks.map((net) => (
              <div
                key={net.id}
                data-testid={`network-row-${net.id}`}
                className="bg-white rounded-2xl border border-[var(--color-border)] overflow-hidden transition-shadow hover:shadow-sm"
              >
                {/* Network Row Header */}
                <div className="flex items-center justify-between px-5 py-3.5">
                  <div className="flex items-center gap-3 min-w-0">
                    <button
                      data-testid={`network-expand-${net.id}`}
                      onClick={() => setExpandedId(expandedId === net.id ? null : net.id)}
                      className="w-6 h-6 rounded-md flex items-center justify-center text-[var(--color-text-secondary)] hover:bg-[var(--color-bg-secondary)] transition-colors shrink-0"
                      aria-label={expandedId === net.id ? '折り畳む' : '展開する'}
                    >
                      <svg
                        className={`w-3.5 h-3.5 transition-transform duration-200 ${expandedId === net.id ? 'rotate-90' : ''}`}
                        fill="none"
                        stroke="currentColor"
                        viewBox="0 0 24 24"
                      >
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 5l7 7-7 7" />
                      </svg>
                    </button>
                    <span className="text-sm font-semibold text-[var(--color-text)]">{net.name}</span>
                    {net.cidr && (
                      <span className="text-xs text-[var(--color-text-secondary)] font-mono bg-[var(--color-bg-secondary)] px-2 py-0.5 rounded">{net.cidr}</span>
                    )}
                    <StatusBadge status={net.status} />
                  </div>
                  <div className="flex items-center gap-2 shrink-0">
                    <Button
                      data-testid={`network-delete-${net.id}`}
                      variant="ghost"
                      size="sm"
                      onClick={(e) => { e.stopPropagation(); setDeleteId(net.id) }}
                      className="text-red-500 hover:text-red-600 hover:bg-red-50"
                    >
                      削除
                    </Button>
                  </div>
                </div>

                {/* Expanded Panel */}
                {expandedId === net.id && (
                  <NetworkExpandedPanel networkId={net.id} />
                )}
              </div>
            ))
          )}
        </div>
      )}

      {/* Create Dialog */}
      {showCreate && (
        <CreateNetworkDialog
          onClose={() => setShowCreate(false)}
          onCreated={(net) => {
            setNetworks((prev) => [...prev, net])
          }}
        />
      )}

      {/* Delete Confirmation */}
      {deleteId && (
        <div data-testid="network-delete-dialog" className="fixed inset-0 z-50 flex items-center justify-center">
          <div className="absolute inset-0 bg-black/40 backdrop-blur-sm" onClick={() => setDeleteId(null)} />
          <div className="relative bg-white rounded-2xl border border-[var(--color-border)] p-6 w-full max-w-sm shadow-xl">
            <div className="flex items-center gap-3 mb-3">
              <div className="w-8 h-8 rounded-lg bg-red-50 flex items-center justify-center">
                <svg className="w-4 h-4 text-red-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                </svg>
              </div>
              <h3 className="text-base font-semibold text-[var(--color-text)]">ネットワークを削除</h3>
            </div>
            <p className="text-sm text-[var(--color-text-secondary)] mb-4">
              {deleteTarget ? `「${deleteTarget.name}」` : 'このネットワーク'}を削除してもよろしいですか？この操作は取り消せません。
            </p>
            <div className="flex gap-2 justify-end">
              <Button variant="ghost" size="sm" onClick={() => setDeleteId(null)}>キャンセル</Button>
              <Button data-testid="network-delete-confirm" variant="danger" size="sm" onClick={() => handleDelete(deleteId)}>削除する</Button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
