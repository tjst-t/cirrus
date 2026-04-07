import { useState, useEffect, useCallback } from 'react'
import {
  storageApi,
  type StorageBackend,
  type AdminVolumeType,
  type AdminFlavor,
  type CreateStorageBackendRequest,
  type CreateVolumeTypeRequest,
  type CreateFlavorRequest,
} from '@/api/storage'
import { Button } from '@/components/Button'
import { Input } from '@/components/Input'
import { Dialog, ConfirmDialog } from '@/components/admin/Dialog'
import { ErrorMessage } from '@/components/admin/ErrorMessage'

// ---- Section wrapper --------------------------------------------------------

function Section({
  title,
  action,
  children,
}: {
  title: string
  action?: React.ReactNode
  children: React.ReactNode
}) {
  return (
    <div className="rounded-lg border border-[var(--color-border)] bg-white overflow-hidden mb-6">
      <div className="flex items-center justify-between px-5 py-3 border-b border-[var(--color-border)] bg-[var(--color-bg-secondary)]">
        <h2 className="font-semibold text-[var(--color-text)]">{title}</h2>
        {action}
      </div>
      <div className="p-5">{children}</div>
    </div>
  )
}

// ---- Storage Backends -------------------------------------------------------

function StorageBackendsSection() {
  const [backends, setBackends] = useState<StorageBackend[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [showCreate, setShowCreate] = useState(false)
  const [creating, setCreating] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<StorageBackend | null>(null)
  const [deleting, setDeleting] = useState(false)
  const [form, setForm] = useState<CreateStorageBackendRequest>({
    name: '',
    type: 'ceph',
    config: {},
  })
  const [configRaw, setConfigRaw] = useState('{}')
  const [configError, setConfigError] = useState<string | null>(null)

  const load = useCallback(() => {
    setLoading(true)
    setError(null)
    storageApi
      .listBackends()
      .then(setBackends)
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => { load() }, [load])

  const handleCreate = () => {
    let config: Record<string, unknown>
    try {
      config = JSON.parse(configRaw) as Record<string, unknown>
      setConfigError(null)
    } catch {
      setConfigError('設定は JSON 形式で入力してください')
      return
    }
    setCreating(true)
    storageApi
      .createBackend({ ...form, config })
      .then(() => {
        setShowCreate(false)
        setForm({ name: '', type: 'ceph', config: {} })
        setConfigRaw('{}')
        load()
      })
      .catch((e: Error) => setError(e.message))
      .finally(() => setCreating(false))
  }

  const handleDelete = () => {
    if (!deleteTarget) return
    setDeleting(true)
    storageApi
      .deleteBackend(deleteTarget.id)
      .then(() => { setDeleteTarget(null); load() })
      .catch((e: Error) => setError(e.message))
      .finally(() => setDeleting(false))
  }

  return (
    <Section
      title="Storage Backend"
      action={<Button size="sm" onClick={() => setShowCreate(true)}>+ 追加</Button>}
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
              <tr key={b.id} className="border-b border-[var(--color-border)] last:border-0">
                <td className="py-2.5 font-medium">{b.name}</td>
                <td className="py-2.5 text-[var(--color-text-secondary)]">{b.type}</td>
                <td className="py-2.5 font-mono text-xs text-[var(--color-text-secondary)]">{b.id}</td>
                <td className="py-2.5 text-right">
                  <Button variant="danger" size="sm" onClick={() => setDeleteTarget(b)}>削除</Button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      <Dialog open={showCreate} onClose={() => setShowCreate(false)} title="Storage Backend 作成">
        <div className="flex flex-col gap-3">
          <div>
            <label htmlFor="backend-name" className="block text-sm font-medium mb-1">名前</label>
            <Input id="backend-name" value={form.name} onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))} placeholder="ceph-01" />
          </div>
          <div>
            <label htmlFor="backend-type" className="block text-sm font-medium mb-1">タイプ</label>
            <select
              id="backend-type"
              value={form.type}
              onChange={(e) => setForm((f) => ({ ...f, type: e.target.value }))}
              className="flex h-9 w-full rounded border border-[var(--color-border)] bg-white px-3 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent"
            >
              <option value="ceph">ceph</option>
              <option value="nfs">nfs</option>
              <option value="local">local</option>
              <option value="lvm">lvm</option>
            </select>
          </div>
          <div>
            <label htmlFor="backend-config" className="block text-sm font-medium mb-1">設定 (JSON)</label>
            <textarea
              id="backend-config"
              value={configRaw}
              onChange={(e) => setConfigRaw(e.target.value)}
              rows={4}
              className="w-full rounded border border-[var(--color-border)] px-3 py-2 text-sm font-mono focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent"
              placeholder='{"monitors": "10.0.0.1"}'
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
      />
    </Section>
  )
}

// ---- Volume Types -----------------------------------------------------------

function VolumeTypesSection({ backends }: { backends: StorageBackend[] }) {
  const [types, setTypes] = useState<AdminVolumeType[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [showCreate, setShowCreate] = useState(false)
  const [creating, setCreating] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<AdminVolumeType | null>(null)
  const [deleting, setDeleting] = useState(false)
  const [form, setForm] = useState<CreateVolumeTypeRequest>({ name: '', backend_id: '' })

  const load = useCallback(() => {
    setLoading(true)
    setError(null)
    storageApi.listVolumeTypes().then(setTypes).catch((e: Error) => setError(e.message)).finally(() => setLoading(false))
  }, [])

  useEffect(() => { load() }, [load])

  const handleCreate = () => {
    if (!form.name.trim() || !form.backend_id) return
    setCreating(true)
    storageApi.createVolumeType(form).then(() => { setShowCreate(false); setForm({ name: '', backend_id: '' }); load() })
      .catch((e: Error) => setError(e.message)).finally(() => setCreating(false))
  }

  const handleDelete = () => {
    if (!deleteTarget) return
    setDeleting(true)
    storageApi.deleteVolumeType(deleteTarget.id).then(() => { setDeleteTarget(null); load() })
      .catch((e: Error) => setError(e.message)).finally(() => setDeleting(false))
  }

  const backendName = (id: string) => backends.find((b) => b.id === id)?.name ?? id

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
              <th className="text-left py-2 font-medium text-[var(--color-text-secondary)]">Backend</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {types.map((t) => (
              <tr key={t.id} className="border-b border-[var(--color-border)] last:border-0">
                <td className="py-2.5 font-medium">{t.name}</td>
                <td className="py-2.5 text-[var(--color-text-secondary)]">{backendName(t.backend_id)}</td>
                <td className="py-2.5 text-right">
                  <Button variant="danger" size="sm" onClick={() => setDeleteTarget(t)}>削除</Button>
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
            <label htmlFor="volume-type-backend" className="block text-sm font-medium mb-1">Storage Backend</label>
            <select
              id="volume-type-backend"
              value={form.backend_id}
              onChange={(e) => setForm((f) => ({ ...f, backend_id: e.target.value }))}
              className="flex h-9 w-full rounded border border-[var(--color-border)] bg-white px-3 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent"
            >
              <option value="">選択してください</option>
              {backends.map((b) => <option key={b.id} value={b.id}>{b.name}</option>)}
            </select>
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
      />
    </Section>
  )
}

// ---- Flavors ----------------------------------------------------------------

function FlavorsSection() {
  const [flavors, setFlavors] = useState<AdminFlavor[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [showCreate, setShowCreate] = useState(false)
  const [creating, setCreating] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<AdminFlavor | null>(null)
  const [deleting, setDeleting] = useState(false)
  const [form, setForm] = useState<CreateFlavorRequest>({ name: '', vcpus: 1, memory_mb: 1024, disk_gb: 20 })

  const load = useCallback(() => {
    setLoading(true)
    setError(null)
    storageApi.listFlavors().then(setFlavors).catch((e: Error) => setError(e.message)).finally(() => setLoading(false))
  }, [])

  useEffect(() => { load() }, [load])

  const handleCreate = () => {
    if (!form.name.trim()) return
    setCreating(true)
    storageApi.createFlavor(form).then(() => { setShowCreate(false); setForm({ name: '', vcpus: 1, memory_mb: 1024, disk_gb: 20 }); load() })
      .catch((e: Error) => setError(e.message)).finally(() => setCreating(false))
  }

  const handleDelete = () => {
    if (!deleteTarget) return
    setDeleting(true)
    storageApi.deleteFlavor(deleteTarget.id).then(() => { setDeleteTarget(null); load() })
      .catch((e: Error) => setError(e.message)).finally(() => setDeleting(false))
  }

  return (
    <Section
      title="Flavor"
      action={<Button size="sm" onClick={() => setShowCreate(true)}>+ 追加</Button>}
    >
      {error && <div className="mb-3"><ErrorMessage message={error} /></div>}
      {loading ? (
        <p className="text-sm text-[var(--color-text-secondary)]">読み込み中...</p>
      ) : flavors.length === 0 ? (
        <p className="text-sm text-[var(--color-text-secondary)]">Flavor なし</p>
      ) : (
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-[var(--color-border)]">
              <th className="text-left py-2 font-medium text-[var(--color-text-secondary)]">名前</th>
              <th className="text-left py-2 font-medium text-[var(--color-text-secondary)]">vCPU</th>
              <th className="text-left py-2 font-medium text-[var(--color-text-secondary)]">メモリ (GB)</th>
              <th className="text-left py-2 font-medium text-[var(--color-text-secondary)]">ディスク (GB)</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {flavors.map((f) => (
              <tr key={f.id} className="border-b border-[var(--color-border)] last:border-0">
                <td className="py-2.5 font-medium">{f.name}</td>
                <td className="py-2.5 text-[var(--color-text-secondary)]">{f.vcpus}</td>
                <td className="py-2.5 text-[var(--color-text-secondary)]">{(f.memory_mb / 1024).toFixed(1)}</td>
                <td className="py-2.5 text-[var(--color-text-secondary)]">{f.disk_gb}</td>
                <td className="py-2.5 text-right">
                  <Button variant="danger" size="sm" onClick={() => setDeleteTarget(f)}>削除</Button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      <Dialog open={showCreate} onClose={() => setShowCreate(false)} title="Flavor 作成">
        <div className="flex flex-col gap-3">
          <div>
            <label htmlFor="flavor-name" className="block text-sm font-medium mb-1">名前</label>
            <Input id="flavor-name" value={form.name} onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))} placeholder="standard.small" />
          </div>
          <div className="flex gap-3">
            <div className="flex-1">
              <label htmlFor="flavor-vcpus" className="block text-sm font-medium mb-1">vCPU</label>
              <Input id="flavor-vcpus" type="number" min={1} value={form.vcpus} onChange={(e) => setForm((f) => ({ ...f, vcpus: parseInt(e.target.value, 10) || 1 }))} />
            </div>
            <div className="flex-1">
              <label htmlFor="flavor-memory" className="block text-sm font-medium mb-1">メモリ (MB)</label>
              <Input id="flavor-memory" type="number" min={128} value={form.memory_mb} onChange={(e) => setForm((f) => ({ ...f, memory_mb: parseInt(e.target.value, 10) || 1024 }))} />
            </div>
            <div className="flex-1">
              <label htmlFor="flavor-disk" className="block text-sm font-medium mb-1">ディスク (GB)</label>
              <Input id="flavor-disk" type="number" min={1} value={form.disk_gb} onChange={(e) => setForm((f) => ({ ...f, disk_gb: parseInt(e.target.value, 10) || 20 }))} />
            </div>
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
        title="Flavor 削除"
        description={`"${deleteTarget?.name}" を削除しますか？`}
        loading={deleting}
      />
    </Section>
  )
}

// ---- Main page --------------------------------------------------------------

export function StoragePage() {
  const [backends, setBackends] = useState<StorageBackend[]>([])

  // We lift backend list here so VolumeTypes section can show backend names
  const loadBackends = useCallback(() => {
    storageApi.listBackends().then(setBackends).catch((e: Error) => {
      console.error('Failed to load storage backends:', e)
    })
  }, [])

  useEffect(() => { loadBackends() }, [loadBackends])

  return (
    <div>
      <h1 className="text-xl font-semibold text-[var(--color-text)] mb-6">ストレージ管理</h1>
      <StorageBackendsSection />
      <VolumeTypesSection backends={backends} />
      <FlavorsSection />
    </div>
  )
}
