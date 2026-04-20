import { useCallback, useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { vmsApi, type Vm, type Flavor, type VolumeType, type CreateVmRequest } from '@/api/vms'
import { networksApi, type Network } from '@/api/networks'
import { azApi, type AvailabilityZone } from '@/api/az'
import { Button } from '@/components/Button'
import { ErrorMessage } from '@/components/ErrorMessage'
import { VmStatusBadge } from '@/components/tenant/VmStatusBadge'
import { ErrorTooltip } from '@/components/tenant/ErrorTooltip'

interface CreateVmDialogProps {
  onClose: () => void
  onCreated: () => void
}

function CreateVmDialog({ onClose, onCreated }: CreateVmDialogProps) {
  const [form, setForm] = useState<CreateVmRequest>({
    name: '',
    flavor_id: '',
    network_id: '',
    az_id: '',
    volume_type_id: '',
    volume_size_gb: 20,
  })
  const [flavors, setFlavors] = useState<Flavor[]>([])
  const [networks, setNetworks] = useState<Network[]>([])
  const [volumeTypes, setVolumeTypes] = useState<VolumeType[]>([])
  const [azList, setAzList] = useState<AvailabilityZone[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    Promise.all([
      vmsApi.listFlavors(),
      networksApi.list(),
      vmsApi.listVolumeTypes(),
      azApi.list(),
    ])
      .then(([f, n, vt, az]) => {
        setFlavors(f)
        setNetworks(n)
        setVolumeTypes(vt)
        setAzList(az)
        if (f.length > 0) setForm((prev) => ({ ...prev, flavor_id: f[0].id }))
        if (n.length > 0) setForm((prev) => ({ ...prev, network_id: n[0].id }))
      })
      .catch((e: Error) => setError(e.message))
  }, [])

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!form.name || !form.flavor_id || !form.network_id) return
    setLoading(true)
    setError(null)
    try {
      const req: CreateVmRequest = {
        name: form.name,
        flavor_id: form.flavor_id,
        network_id: form.network_id,
      }
      if (form.az_id) {
        req.az_id = form.az_id
      }
      if (form.volume_type_id) {
        req.volume_type_id = form.volume_type_id
        req.volume_size_gb = form.volume_size_gb
      }
      await vmsApi.create(req)
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
        <h3 className="text-base font-semibold text-[var(--color-text)] mb-4">VM を作成</h3>
        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label className="block text-xs font-medium text-[var(--color-text)] mb-1">名前 *</label>
            <input
              data-testid="vm-create-name"
              type="text"
              value={form.name}
              onChange={(e) => setForm((p) => ({ ...p, name: e.target.value }))}
              required
              className="w-full h-9 px-3 text-sm border border-[var(--color-border)] rounded focus:outline-none focus:ring-2 focus:ring-accent/30"
              placeholder="my-vm"
            />
          </div>
          <div>
            <label className="block text-xs font-medium text-[var(--color-text)] mb-1">フレーバー *</label>
            <select
              data-testid="vm-create-flavor"
              value={form.flavor_id}
              onChange={(e) => setForm((p) => ({ ...p, flavor_id: e.target.value }))}
              required
              className="w-full h-9 px-3 text-sm border border-[var(--color-border)] rounded focus:outline-none focus:ring-2 focus:ring-accent/30 bg-white"
            >
              {flavors.map((f) => (
                <option key={f.id} value={f.id}>
                  {f.name} ({f.vcpus}vCPU / {Math.round(f.ram_mb / 1024)}GB RAM / {f.disk_gb}GB)
                </option>
              ))}
            </select>
          </div>
          <div>
            <label className="block text-xs font-medium text-[var(--color-text)] mb-1">ネットワーク *</label>
            <select
              data-testid="vm-create-network"
              value={form.network_id}
              onChange={(e) => setForm((p) => ({ ...p, network_id: e.target.value }))}
              required
              className="w-full h-9 px-3 text-sm border border-[var(--color-border)] rounded focus:outline-none focus:ring-2 focus:ring-accent/30 bg-white"
            >
              {networks.map((n) => (
                <option key={n.id} value={n.id}>
                  {n.name} ({n.cidr})
                </option>
              ))}
            </select>
          </div>
          <div>
            <label className="block text-xs font-medium text-[var(--color-text)] mb-1">アベイラビリティゾーン</label>
            <select
              data-testid="vm-create-az"
              value={form.az_id}
              onChange={(e) => setForm((p) => ({ ...p, az_id: e.target.value }))}
              className="w-full h-9 px-3 text-sm border border-[var(--color-border)] rounded focus:outline-none focus:ring-2 focus:ring-accent/30 bg-white"
            >
              <option value="">自動選択</option>
              {azList.map((az) => (
                <option key={az.id} value={az.id}>
                  {az.name}
                </option>
              ))}
            </select>
          </div>
          {volumeTypes.length > 0 && (
            <>
              <div>
                <label className="block text-xs font-medium text-[var(--color-text)] mb-1">ボリュームタイプ</label>
                <select
                  value={form.volume_type_id}
                  onChange={(e) => setForm((p) => ({ ...p, volume_type_id: e.target.value }))}
                  className="w-full h-9 px-3 text-sm border border-[var(--color-border)] rounded focus:outline-none focus:ring-2 focus:ring-accent/30 bg-white"
                >
                  <option value="">なし</option>
                  {volumeTypes.map((vt) => (
                    <option key={vt.id} value={vt.id}>{vt.name}</option>
                  ))}
                </select>
              </div>
              {form.volume_type_id && (
                <div>
                  <label className="block text-xs font-medium text-[var(--color-text)] mb-1">ボリュームサイズ (GB)</label>
                  <input
                    type="number"
                    value={form.volume_size_gb}
                    onChange={(e) => setForm((p) => ({ ...p, volume_size_gb: Number(e.target.value) }))}
                    min={1}
                    className="w-full h-9 px-3 text-sm border border-[var(--color-border)] rounded focus:outline-none focus:ring-2 focus:ring-accent/30"
                  />
                </div>
              )}
            </>
          )}

          {error && (
            <ErrorMessage data-testid="form-error" message={error} />
          )}

          <div className="flex gap-2 justify-end pt-2">
            <Button type="button" variant="ghost" size="sm" onClick={onClose}>
              キャンセル
            </Button>
            <Button data-testid="vm-create-submit" type="submit" variant="primary" size="sm" disabled={loading}>
              {loading ? '作成中...' : '作成'}
            </Button>
          </div>
        </form>
      </div>
    </div>
  )
}

export function VmsPage() {
  const [vms, setVms] = useState<Vm[]>([])
  const [flavors, setFlavors] = useState<Flavor[]>([])
  const [azList, setAzList] = useState<AvailabilityZone[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [showCreate, setShowCreate] = useState(false)
  const [deleteId, setDeleteId] = useState<string | null>(null)
  const [actionLoading, setActionLoading] = useState<string | null>(null)
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null)

  const TRANSITIONAL = new Set(['pending', 'starting', 'stopping'])

  const load = useCallback(() => {
    setLoading(true)
    Promise.all([
      vmsApi.list(),
      vmsApi.listFlavors().catch(() => [] as Flavor[]),
      azApi.list().catch(() => [] as AvailabilityZone[]),
    ])
      .then(([v, f, az]) => {
        setVms(v)
        setFlavors(f)
        setAzList(az)
      })
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false))
  }, [])

  // Poll every 3s while any VM is in a transitional state.
  // Uses functional setState so React skips re-render when nothing changed.
  const poll = useCallback(() => {
    vmsApi.list().then((next) => {
      setVms((prev) => {
        if (prev.length !== next.length) return next
        const changed = next.some((vm, i) =>
          prev[i]?.id !== vm.id ||
          prev[i]?.status !== vm.status ||
          prev[i]?.updated_at !== vm.updated_at
        )
        return changed ? next : prev
      })
    }).catch(() => { /* silent poll failure */ })
  }, [])

  useEffect(() => { load() }, [load])

  useEffect(() => {
    const hasTransitional = vms.some((v) => TRANSITIONAL.has(v.status))
    if (hasTransitional) {
      if (!pollRef.current) {
        pollRef.current = setInterval(poll, 3000)
      }
    } else {
      if (pollRef.current) {
        clearInterval(pollRef.current)
        pollRef.current = null
      }
    }
    return () => {
      if (pollRef.current) {
        clearInterval(pollRef.current)
        pollRef.current = null
      }
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [vms, poll])

  const handleDelete = async (id: string) => {
    setError(null)
    try {
      await vmsApi.delete(id)
      load()
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'エラーが発生しました')
    } finally {
      setDeleteId(null)
    }
  }

  const handleAction = async (id: string, action: 'start' | 'stop' | 'reboot') => {
    setError(null)
    setActionLoading(`${id}-${action}`)
    try {
      await vmsApi.action(id, action)
      load()
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'エラーが発生しました')
    } finally {
      setActionLoading(null)
    }
  }

  const flavorName = (flavorId?: string) => {
    if (!flavorId) return '—'
    const f = flavors.find((f) => f.id === flavorId)
    return f ? f.name : flavorId
  }

  const azName = (azId?: string) => {
    if (!azId) return '—'
    const az = azList.find((a) => a.id === azId)
    return az ? az.name : azId
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold text-[var(--color-text)]">VM 管理</h2>
        <Button data-testid="vm-create-button" variant="primary" size="sm" onClick={() => setShowCreate(true)}>
          + VM を作成
        </Button>
      </div>

      {error && <ErrorMessage message={error} onDismiss={() => setError(null)} />}

      {loading ? (
        <div className="flex items-center justify-center h-40 text-[var(--color-text-secondary)] text-sm">
          読み込み中...
        </div>
      ) : (
        <div className="bg-white rounded-xl border border-[var(--color-border)] overflow-hidden">
          <table className="w-full text-sm">
            <thead className="bg-[var(--color-bg-secondary)]">
              <tr>
                <th className="text-left px-4 py-3 text-xs font-medium text-[var(--color-text-secondary)]">名前</th>
                <th className="text-left px-4 py-3 text-xs font-medium text-[var(--color-text-secondary)]">状態</th>
                <th className="text-left px-4 py-3 text-xs font-medium text-[var(--color-text-secondary)]">フレーバー</th>
                <th className="text-left px-4 py-3 text-xs font-medium text-[var(--color-text-secondary)]">AZ</th>
                <th className="text-left px-4 py-3 text-xs font-medium text-[var(--color-text-secondary)]">IPアドレス</th>
                <th className="text-left px-4 py-3 text-xs font-medium text-[var(--color-text-secondary)]">作成日時</th>
                <th className="text-left px-4 py-3 text-xs font-medium text-[var(--color-text-secondary)]">操作</th>
              </tr>
            </thead>
            <tbody>
              {vms.length === 0 ? (
                <tr>
                  <td colSpan={7} className="px-4 py-8 text-center text-[var(--color-text-secondary)]">
                    VM がありません
                  </td>
                </tr>
              ) : (
                vms.map((vm) => (
                  <tr key={vm.id} data-testid={`vm-row-${vm.id}`} className="border-t border-[var(--color-border)] hover:bg-[var(--color-bg-secondary)]/50">
                    <td className="px-4 py-3">
                      <Link to={`/vms/${vm.id}`} className="text-accent hover:underline font-medium">
                        {vm.name}
                      </Link>
                    </td>
                    <td className="px-4 py-3">
                      <div className="flex items-center gap-1">
                        <VmStatusBadge status={vm.status} data-testid={`vm-status-badge-${vm.id}`} />
                        {vm.status === 'error' && vm.error_message && (
                          <ErrorTooltip
                            id={vm.id}
                            message={vm.error_message}
                          />
                        )}
                      </div>
                    </td>
                    <td className="px-4 py-3 text-[var(--color-text-secondary)] text-xs">
                      {flavorName(vm.flavor_id)}
                    </td>
                    <td className="px-4 py-3 text-[var(--color-text-secondary)] text-xs">
                      {azName(vm.az_id)}
                    </td>
                    <td className="px-4 py-3 text-[var(--color-text-secondary)] font-mono text-xs">
                      {vm.ip_address ?? '—'}
                    </td>
                    <td className="px-4 py-3 text-[var(--color-text-secondary)]">
                      {new Date(vm.created_at).toLocaleDateString('ja-JP')}
                    </td>
                    <td className="px-4 py-3">
                      <div className="flex gap-1 flex-wrap">
                        <Button
                          data-testid={`vm-start-${vm.id}`}
                          variant="secondary"
                          size="sm"
                          onClick={() => handleAction(vm.id, 'start')}
                          disabled={vm.status !== 'stopped' || actionLoading !== null}
                        >
                          起動
                        </Button>
                        <Button
                          data-testid={`vm-stop-${vm.id}`}
                          variant="secondary"
                          size="sm"
                          onClick={() => handleAction(vm.id, 'stop')}
                          disabled={vm.status !== 'running' || actionLoading !== null}
                        >
                          停止
                        </Button>
                        <Button
                          data-testid={`vm-reboot-${vm.id}`}
                          variant="secondary"
                          size="sm"
                          onClick={() => handleAction(vm.id, 'reboot')}
                          disabled={vm.status !== 'running' || actionLoading !== null}
                        >
                          再起動
                        </Button>
                        <Button
                          data-testid={`vm-delete-${vm.id}`}
                          variant="danger"
                          size="sm"
                          onClick={() => setDeleteId(vm.id)}
                        >
                          削除
                        </Button>
                      </div>
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      )}

      {showCreate && (
        <CreateVmDialog
          onClose={() => setShowCreate(false)}
          onCreated={load}
        />
      )}

      {deleteId && (
        <div className="fixed inset-0 z-50 flex items-center justify-center">
          <div className="absolute inset-0 bg-black/40" onClick={() => setDeleteId(null)} />
          <div className="relative bg-white rounded-xl border border-[var(--color-border)] p-6 w-full max-w-sm shadow-lg">
            <h3 className="text-base font-semibold text-[var(--color-text)] mb-2">VM を削除</h3>
            <p className="text-sm text-[var(--color-text-secondary)] mb-4">
              この VM を削除してもよろしいですか？この操作は取り消せません。
            </p>
            <div className="flex gap-2 justify-end">
              <Button variant="ghost" size="sm" onClick={() => setDeleteId(null)}>
                キャンセル
              </Button>
              <Button data-testid="vm-list-delete-confirm-button" variant="danger" size="sm" onClick={() => handleDelete(deleteId)}>
                削除する
              </Button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
