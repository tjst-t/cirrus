import { useState, useEffect, useCallback } from 'react'
import {
  networkInfraApi,
  type GatewayNode,
  type IPPool,
  type CreateGatewayNodeRequest,
  type CreateIPPoolRequest,
} from '@/api/network-infra'
import { Button } from '@/components/Button'
import { Input } from '@/components/Input'
import { Dialog, ConfirmDialog } from '@/components/admin/Dialog'
import { ErrorMessage } from '@/components/admin/ErrorMessage'
import { Section } from '@/components/admin/Section'

// ---- Gateway Nodes ----------------------------------------------------------

function GatewayNodesSection() {
  const [nodes, setNodes] = useState<GatewayNode[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [showCreate, setShowCreate] = useState(false)
  const [creating, setCreating] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<GatewayNode | null>(null)
  const [deleting, setDeleting] = useState(false)
  const [form, setForm] = useState<CreateGatewayNodeRequest>({
    host_id: '',
    external_ip: '',
    internal_ip: '',
  })

  const load = useCallback(() => {
    setLoading(true)
    setError(null)
    networkInfraApi
      .listGatewayNodes()
      .then(setNodes)
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => { load() }, [load])

  const handleCreate = () => {
    if (!form.host_id.trim() || !form.external_ip.trim() || !form.internal_ip.trim()) return
    setCreating(true)
    networkInfraApi
      .createGatewayNode(form)
      .then(() => {
        setShowCreate(false)
        setForm({ host_id: '', external_ip: '', internal_ip: '' })
        load()
      })
      .catch((e: Error) => setError(e.message))
      .finally(() => setCreating(false))
  }

  const handleDelete = () => {
    if (!deleteTarget) return
    setDeleting(true)
    networkInfraApi
      .deleteGatewayNode(deleteTarget.id)
      .then(() => { setDeleteTarget(null); load() })
      .catch((e: Error) => setError(e.message))
      .finally(() => setDeleting(false))
  }

  return (
    <Section
      title="ゲートウェイノード"
      action={<Button data-testid="create-gateway-node-button" size="sm" onClick={() => setShowCreate(true)}>+ 追加</Button>}
    >
      {error && <div className="mb-3"><ErrorMessage message={error} /></div>}
      {loading ? (
        <p className="text-sm text-[var(--color-text-secondary)]">読み込み中...</p>
      ) : nodes.length === 0 ? (
        <p data-testid="empty-gateway-nodes" className="text-sm text-[var(--color-text-secondary)]">ゲートウェイノードなし</p>
      ) : (
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-[var(--color-border)]">
              <th className="text-left py-2 font-medium text-[var(--color-text-secondary)]">Host ID</th>
              <th className="text-left py-2 font-medium text-[var(--color-text-secondary)]">外部 IP</th>
              <th className="text-left py-2 font-medium text-[var(--color-text-secondary)]">内部 IP</th>
              <th className="text-left py-2 font-medium text-[var(--color-text-secondary)]">ID</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {nodes.map((n) => (
              <tr key={n.id} data-testid={`gateway-node-row-${n.id}`} className="border-b border-[var(--color-border)] last:border-0">
                <td className="py-2.5 font-mono text-xs text-[var(--color-text-secondary)]">{n.host_id}</td>
                <td className="py-2.5 text-[var(--color-text-secondary)]">{n.external_ip}</td>
                <td className="py-2.5 text-[var(--color-text-secondary)]">{n.internal_ip}</td>
                <td className="py-2.5 font-mono text-xs text-[var(--color-text-secondary)]">{n.id}</td>
                <td className="py-2.5 text-right">
                  <Button data-testid={`delete-gateway-node-button-${n.id}`} variant="danger" size="sm" onClick={() => setDeleteTarget(n)}>削除</Button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      <Dialog open={showCreate} onClose={() => setShowCreate(false)} title="ゲートウェイノード登録" data-testid="create-gateway-node-dialog">
        <div className="flex flex-col gap-3">
          <div>
            <label htmlFor="gw-host-id" className="block text-sm font-medium mb-1">Host ID</label>
            <Input
              id="gw-host-id"
              data-testid="gateway-node-host-id-input"
              value={form.host_id}
              onChange={(e) => setForm((f) => ({ ...f, host_id: e.target.value }))}
              placeholder="xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
            />
          </div>
          <div>
            <label htmlFor="gw-external-ip" className="block text-sm font-medium mb-1">外部 IP</label>
            <Input
              id="gw-external-ip"
              data-testid="gateway-node-external-ip-input"
              value={form.external_ip}
              onChange={(e) => setForm((f) => ({ ...f, external_ip: e.target.value }))}
              placeholder="203.0.113.10"
            />
          </div>
          <div>
            <label htmlFor="gw-internal-ip" className="block text-sm font-medium mb-1">内部 IP</label>
            <Input
              id="gw-internal-ip"
              data-testid="gateway-node-internal-ip-input"
              value={form.internal_ip}
              onChange={(e) => setForm((f) => ({ ...f, internal_ip: e.target.value }))}
              placeholder="10.0.0.1"
            />
          </div>
          <div className="flex justify-end gap-2 mt-1">
            <Button variant="secondary" size="sm" onClick={() => setShowCreate(false)} disabled={creating}>キャンセル</Button>
            <Button data-testid="create-gateway-node-submit" size="sm" onClick={handleCreate} disabled={creating}>{creating ? '登録中...' : '登録'}</Button>
          </div>
        </div>
      </Dialog>

      <ConfirmDialog
        open={deleteTarget !== null}
        onClose={() => setDeleteTarget(null)}
        onConfirm={handleDelete}
        title="ゲートウェイノード削除"
        description={`ゲートウェイノード "${deleteTarget?.id}" を削除しますか？`}
        loading={deleting}
        data-testid="confirm-delete-dialog"
        confirmButtonTestId="confirm-delete-button"
      />
    </Section>
  )
}

// ---- IP Pools ---------------------------------------------------------------

function IPPoolsSection() {
  const [pools, setPools] = useState<IPPool[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [showCreate, setShowCreate] = useState(false)
  const [creating, setCreating] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<IPPool | null>(null)
  const [deleting, setDeleting] = useState(false)
  const [form, setForm] = useState<CreateIPPoolRequest>({ name: '', cidr: '' })

  const load = useCallback(() => {
    setLoading(true)
    setError(null)
    networkInfraApi
      .listIPPools()
      .then(setPools)
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => { load() }, [load])

  const handleCreate = () => {
    if (!form.name.trim() || !form.cidr.trim()) return
    setCreating(true)
    networkInfraApi
      .createIPPool(form)
      .then(() => {
        setShowCreate(false)
        setForm({ name: '', cidr: '' })
        load()
      })
      .catch((e: Error) => setError(e.message))
      .finally(() => setCreating(false))
  }

  const handleDelete = () => {
    if (!deleteTarget) return
    setDeleting(true)
    networkInfraApi
      .deleteIPPool(deleteTarget.id)
      .then(() => { setDeleteTarget(null); load() })
      .catch((e: Error) => setError(e.message))
      .finally(() => setDeleting(false))
  }

  return (
    <Section
      title="IP プール"
      action={<Button data-testid="create-ip-pool-button" size="sm" onClick={() => setShowCreate(true)}>+ 追加</Button>}
    >
      {error && <div className="mb-3"><ErrorMessage message={error} /></div>}
      {loading ? (
        <p className="text-sm text-[var(--color-text-secondary)]">読み込み中...</p>
      ) : pools.length === 0 ? (
        <p className="text-sm text-[var(--color-text-secondary)]">IP プールなし</p>
      ) : (
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-[var(--color-border)]">
              <th className="text-left py-2 font-medium text-[var(--color-text-secondary)]">名前</th>
              <th className="text-left py-2 font-medium text-[var(--color-text-secondary)]">CIDR</th>
              <th className="text-left py-2 font-medium text-[var(--color-text-secondary)]">ID</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {pools.map((p) => (
              <tr key={p.id} data-testid={`ip-pool-row-${p.id}`} className="border-b border-[var(--color-border)] last:border-0">
                <td className="py-2.5 font-medium">{p.name}</td>
                <td className="py-2.5 font-mono text-[var(--color-text-secondary)]">{p.cidr}</td>
                <td className="py-2.5 font-mono text-xs text-[var(--color-text-secondary)]">{p.id}</td>
                <td className="py-2.5 text-right">
                  <Button data-testid={`delete-ip-pool-button-${p.id}`} variant="danger" size="sm" onClick={() => setDeleteTarget(p)}>削除</Button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      <Dialog open={showCreate} onClose={() => setShowCreate(false)} title="IP プール作成" data-testid="create-ip-pool-dialog">
        <div className="flex flex-col gap-3">
          <div>
            <label htmlFor="ip-pool-name" className="block text-sm font-medium mb-1">名前</label>
            <Input
              id="ip-pool-name"
              data-testid="ip-pool-name-input"
              value={form.name}
              onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
              placeholder="public-pool-01"
            />
          </div>
          <div>
            <label htmlFor="ip-pool-cidr" className="block text-sm font-medium mb-1">CIDR</label>
            <Input
              id="ip-pool-cidr"
              data-testid="ip-pool-cidr-input"
              value={form.cidr}
              onChange={(e) => setForm((f) => ({ ...f, cidr: e.target.value }))}
              placeholder="203.0.113.0/24"
            />
          </div>
          <div className="flex justify-end gap-2 mt-1">
            <Button variant="secondary" size="sm" onClick={() => setShowCreate(false)} disabled={creating}>キャンセル</Button>
            <Button data-testid="create-ip-pool-submit" size="sm" onClick={handleCreate} disabled={creating}>{creating ? '作成中...' : '作成'}</Button>
          </div>
        </div>
      </Dialog>

      <ConfirmDialog
        open={deleteTarget !== null}
        onClose={() => setDeleteTarget(null)}
        onConfirm={handleDelete}
        title="IP プール削除"
        description={`"${deleteTarget?.name}" を削除しますか？`}
        loading={deleting}
        data-testid="confirm-delete-dialog"
        confirmButtonTestId="confirm-delete-button"
      />
    </Section>
  )
}

// ---- Main page --------------------------------------------------------------

export function NetworkInfraPage() {
  return (
    <div>
      <h1 className="text-xl font-semibold text-[var(--color-text)] mb-6">ネットワーク管理</h1>
      <GatewayNodesSection />
      <IPPoolsSection />
    </div>
  )
}
