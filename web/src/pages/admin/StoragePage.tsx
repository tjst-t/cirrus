import { useState, useEffect, useCallback } from 'react'
import {
  storageApi,
  type StorageBackend,
  type StorageDomain,
  type AdminVolumeType,
  type CreateStorageBackendRequest,
  type CreateVolumeTypeRequest,
} from '@/api/storage'
import { Button } from '@/components/Button'
import { Input } from '@/components/Input'
import { Dialog, ConfirmDialog } from '@/components/admin/Dialog'
import { ErrorMessage } from '@/components/admin/ErrorMessage'
import { Section } from '@/components/admin/Section'

// ---- Storage Backends -------------------------------------------------------

function StorageBackendsSection() {
  const [backends, setBackends] = useState<StorageBackend[]>([])
  const [domains, setDomains] = useState<StorageDomain[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [showCreate, setShowCreate] = useState(false)
  const [creating, setCreating] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<StorageBackend | null>(null)
  const [deleting, setDeleting] = useState(false)
  const [form, setForm] = useState<CreateStorageBackendRequest>({
    storage_domain_id: '',
    name: '',
    driver: 'ceph',
    endpoint: '',
  })
  const [driverConfigRaw, setDriverConfigRaw] = useState('{}')
  const [configError, setConfigError] = useState<string | null>(null)

  const load = useCallback(() => {
    setLoading(true)
    setError(null)
    Promise.all([storageApi.listBackends(), storageApi.listDomains()])
      .then(([b, d]) => { setBackends(b); setDomains(d) })
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => { load() }, [load])

  const handleCreate = () => {
    if (!form.name.trim() || !form.storage_domain_id || !form.endpoint.trim()) return
    let driver_config: Record<string, unknown>
    try {
      driver_config = JSON.parse(driverConfigRaw) as Record<string, unknown>
      setConfigError(null)
    } catch {
      setConfigError('Driver Config は JSON 形式で入力してください')
      return
    }
    setCreating(true)
    storageApi
      .createBackend({ ...form, driver_config })
      .then(() => {
        setShowCreate(false)
        setForm({ storage_domain_id: '', name: '', driver: 'ceph', endpoint: '' })
        setDriverConfigRaw('{}')
        load()
      })
      .catch((e: Error) => setError(e.message))
      .finally(() => setCreating(false))
  }

  const handleDelete = () => {
    if (!deleteTarget) return
    setError(null)
    setDeleting(true)
    storageApi
      .deleteBackend(deleteTarget.id)
      .then(() => load())
      .catch((e: Error) => setError(e.message))
      .finally(() => { setDeleteTarget(null); setDeleting(false) })
  }

  return (
    <Section
      title="Storage Backend"
      action={<Button data-testid="create-backend-button" size="sm" onClick={() => setShowCreate(true)}>+ 追加</Button>}
    >
      {error && <div className="mb-3"><ErrorMessage message={error} /></div>}
      {loading ? (
        <p className="text-sm text-[var(--color-text-secondary)]">読み込み中...</p>
      ) : backends.length === 0 ? (
        <p className="text-sm text-[var(--color-text-secondary)]">バックエンドなし</p>
      ) : (
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-[var(--color-border)]">
              <th className="text-left py-2 font-medium text-[var(--color-text-secondary)]">名前</th>
              <th className="text-left py-2 font-medium text-[var(--color-text-secondary)]">タイプ</th>
              <th className="text-left py-2 font-medium text-[var(--color-text-secondary)]">ID</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {backends.map((b) => (
              <tr key={b.id} data-testid={`backend-row-${b.id}`} className="border-b border-[var(--color-border)] last:border-0">
                <td className="py-2.5 font-medium">{b.name}</td>
                <td className="py-2.5 text-[var(--color-text-secondary)]">{b.driver}</td>
                <td className="py-2.5 font-mono text-xs text-[var(--color-text-secondary)]">{b.id}</td>
                <td className="py-2.5 text-right">
                  <Button data-testid={`delete-backend-button-${b.id}`} variant="danger" size="sm" onClick={() => setDeleteTarget(b)}>削除</Button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      <Dialog open={showCreate} onClose={() => setShowCreate(false)} title="Storage Backend 作成" data-testid="create-backend-dialog">
        <div className="flex flex-col gap-3">
          <div>
            <label htmlFor="backend-domain" className="block text-sm font-medium mb-1">Storage Domain</label>
            <select
              id="backend-domain"
              value={form.storage_domain_id}
              onChange={(e) => setForm((f) => ({ ...f, storage_domain_id: e.target.value }))}
              className="flex h-9 w-full rounded border border-[var(--color-border)] bg-white px-3 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent"
            >
              <option value="">選択してください</option>
              {domains.map((d) => <option key={d.id} value={d.id}>{d.name}</option>)}
            </select>
          </div>
          <div>
            <label htmlFor="backend-name" className="block text-sm font-medium mb-1">名前</label>
            <Input id="backend-name" data-testid="backend-name-input" value={form.name} onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))} placeholder="ceph-01" />
          </div>
          <div>
            <label htmlFor="backend-driver" className="block text-sm font-medium mb-1">Driver</label>
            <select
              id="backend-driver"
              value={form.driver}
              onChange={(e) => setForm((f) => ({ ...f, driver: e.target.value }))}
              className="flex h-9 w-full rounded border border-[var(--color-border)] bg-white px-3 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent"
            >
              <option value="ceph">ceph</option>
              <option value="sim">sim</option>
              <option value="nfs">nfs</option>
              <option value="lvm">lvm</option>
            </select>
          </div>
          <div>
            <label htmlFor="backend-endpoint" className="block text-sm font-medium mb-1">Endpoint</label>
            <Input id="backend-endpoint" value={form.endpoint} onChange={(e) => setForm((f) => ({ ...f, endpoint: e.target.value }))} placeholder="ceph://mon1:6789/pool" />
          </div>
          <div>
            <label htmlFor="backend-driver-config" className="block text-sm font-medium mb-1">Driver Config (JSON)</label>
            <textarea
              id="backend-driver-config"
              value={driverConfigRaw}
              onChange={(e) => setDriverConfigRaw(e.target.value)}
              rows={3}
              className="w-full rounded border border-[var(--color-border)] px-3 py-2 text-sm font-mono focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent"
              placeholder='{}'
            />
            {configError && <p className="text-xs text-[var(--color-danger)] mt-1">{configError}</p>}
          </div>
          <div className="flex justify-end gap-2 mt-1">
            <Button variant="secondary" size="sm" onClick={() => setShowCreate(false)} disabled={creating}>キャンセル</Button>
            <Button size="sm" onClick={handleCreate} disabled={creating}>{creating ? '作成中...' : '作成'}</Button>
          </div>
        </div>
      </Dialog>

      <ConfirmDialog
        open={deleteTarget !== null}
        onClose={() => setDeleteTarget(null)}
        onConfirm={handleDelete}
        title="Storage Backend 削除"
        description={`"${deleteTarget?.name}" を削除しますか？`}
        loading={deleting}
        data-testid="confirm-delete-dialog"
        confirmButtonTestId="confirm-delete-button"
      />
    </Section>
  )
}

// ---- Volume Types -----------------------------------------------------------

function VolumeTypesSection() {
  const [types, setTypes] = useState<AdminVolumeType[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [showCreate, setShowCreate] = useState(false)
  const [creating, setCreating] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<AdminVolumeType | null>(null)
  const [deleting, setDeleting] = useState(false)
  const [form, setForm] = useState<CreateVolumeTypeRequest>({ name: '' })

  const load = useCallback(() => {
    setLoading(true)
    setError(null)
    storageApi.listVolumeTypes().then(setTypes).catch((e: Error) => setError(e.message)).finally(() => setLoading(false))
  }, [])

  useEffect(() => { load() }, [load])

  const handleCreate = () => {
    if (!form.name.trim()) return
    setCreating(true)
    storageApi.createVolumeType(form).then(() => { setShowCreate(false); setForm({ name: '' }); load() })
      .catch((e: Error) => setError(e.message)).finally(() => setCreating(false))
  }

  const handleDelete = () => {
    if (!deleteTarget) return
    setError(null)
    setDeleting(true)
    storageApi.deleteVolumeType(deleteTarget.id).then(() => load())
      .catch((e: Error) => setError(e.message))
      .finally(() => { setDeleteTarget(null); setDeleting(false) })
  }

  return (
    <Section
      title="Volume Type"
      action={<Button size="sm" onClick={() => setShowCreate(true)}>+ 追加</Button>}
    >
      {error && <div className="mb-3"><ErrorMessage message={error} /></div>}
      {loading ? (
        <p className="text-sm text-[var(--color-text-secondary)]">読み込み中...</p>
      ) : types.length === 0 ? (
        <p className="text-sm text-[var(--color-text-secondary)]">Volume Type なし</p>
      ) : (
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-[var(--color-border)]">
              <th className="text-left py-2 font-medium text-[var(--color-text-secondary)]">名前</th>
              <th className="text-left py-2 font-medium text-[var(--color-text-secondary)]">説明</th>
              <th className="text-left py-2 font-medium text-[var(--color-text-secondary)]">公開</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {types.map((t) => (
              <tr key={t.id} data-testid={`volume-type-row-${t.id}`} className="border-b border-[var(--color-border)] last:border-0">
                <td className="py-2.5 font-medium">{t.name}</td>
                <td className="py-2.5 text-[var(--color-text-secondary)]">{t.description || '-'}</td>
                <td className="py-2.5 text-[var(--color-text-secondary)]">{t.is_public ? 'はい' : 'いいえ'}</td>
                <td className="py-2.5 text-right">
                  <Button data-testid={`delete-volume-type-button-${t.id}`} variant="danger" size="sm" onClick={() => setDeleteTarget(t)}>削除</Button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      <Dialog open={showCreate} onClose={() => setShowCreate(false)} title="Volume Type 作成">
        <div className="flex flex-col gap-3">
          <div>
            <label htmlFor="volume-type-name" className="block text-sm font-medium mb-1">名前</label>
            <Input id="volume-type-name" value={form.name} onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))} placeholder="ssd" />
          </div>
          <div>
            <label htmlFor="volume-type-description" className="block text-sm font-medium mb-1">説明（任意）</label>
            <Input id="volume-type-description" value={form.description ?? ''} onChange={(e) => setForm((f) => ({ ...f, description: e.target.value }))} placeholder="SSD ボリュームタイプ" />
          </div>
          <div className="flex justify-end gap-2 mt-1">
            <Button variant="secondary" size="sm" onClick={() => setShowCreate(false)} disabled={creating}>キャンセル</Button>
            <Button size="sm" onClick={handleCreate} disabled={creating}>{creating ? '作成中...' : '作成'}</Button>
          </div>
        </div>
      </Dialog>

      <ConfirmDialog
        open={deleteTarget !== null}
        onClose={() => setDeleteTarget(null)}
        onConfirm={handleDelete}
        title="Volume Type 削除"
        description={`"${deleteTarget?.name}" を削除しますか？`}
        loading={deleting}
        data-testid="confirm-delete-dialog"
        confirmButtonTestId="confirm-delete-button"
      />
    </Section>
  )
}

// ---- Main page --------------------------------------------------------------

export function StoragePage() {
  return (
    <div>
      <h1 className="text-xl font-semibold text-[var(--color-text)] mb-6">ストレージ管理</h1>
      <StorageBackendsSection />
      <VolumeTypesSection />
    </div>
  )
}
