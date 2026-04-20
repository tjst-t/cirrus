import { useState, useEffect, useCallback } from 'react'
import {
  storageApi,
  type AdminFlavor,
  type CreateFlavorRequest,
} from '@/api/storage'
import { Button } from '@/components/Button'
import { Input } from '@/components/Input'
import { Dialog, ConfirmDialog } from '@/components/admin/Dialog'
import { ErrorMessage } from '@/components/admin/ErrorMessage'
import { Section } from '@/components/admin/Section'

function FlavorsSection() {
  const [flavors, setFlavors] = useState<AdminFlavor[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [showCreate, setShowCreate] = useState(false)
  const [creating, setCreating] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<AdminFlavor | null>(null)
  const [deleting, setDeleting] = useState(false)
  const [form, setForm] = useState<CreateFlavorRequest>({ name: '', vcpus: 1, ram_mb: 1024, disk_gb: 20 })

  const load = useCallback(() => {
    setLoading(true)
    setError(null)
    storageApi.listFlavors().then(setFlavors).catch((e: Error) => setError(e.message)).finally(() => setLoading(false))
  }, [])

  useEffect(() => { load() }, [load])

  const handleCreate = () => {
    if (!form.name.trim()) return
    setCreating(true)
    storageApi.createFlavor(form).then(() => { setShowCreate(false); setForm({ name: '', vcpus: 1, ram_mb: 1024, disk_gb: 20 }); load() })
      .catch((e: Error) => setError(e.message)).finally(() => setCreating(false))
  }

  const handleDelete = () => {
    if (!deleteTarget) return
    setError(null)
    setDeleting(true)
    storageApi.deleteFlavor(deleteTarget.id).then(() => load())
      .catch((e: Error) => setError(e.message))
      .finally(() => { setDeleteTarget(null); setDeleting(false) })
  }

  return (
    <Section
      title="Flavor"
      action={<Button data-testid="create-flavor-button" size="sm" onClick={() => setShowCreate(true)}>+ 追加</Button>}
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
              <tr key={f.id} data-testid={`flavor-row-${f.id}`} className="border-b border-[var(--color-border)] last:border-0">
                <td className="py-2.5 font-medium">{f.name}</td>
                <td className="py-2.5 text-[var(--color-text-secondary)]">{f.vcpus}</td>
                <td className="py-2.5 text-[var(--color-text-secondary)]">{(f.ram_mb / 1024).toFixed(1)}</td>
                <td className="py-2.5 text-[var(--color-text-secondary)]">{f.disk_gb}</td>
                <td className="py-2.5 text-right">
                  <Button data-testid={`delete-flavor-button-${f.id}`} variant="danger" size="sm" onClick={() => setDeleteTarget(f)}>削除</Button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      <Dialog open={showCreate} onClose={() => setShowCreate(false)} title="Flavor 作成" data-testid="create-flavor-dialog">
        <div className="flex flex-col gap-3">
          <div>
            <label htmlFor="flavor-name" className="block text-sm font-medium mb-1">名前</label>
            <Input id="flavor-name" data-testid="flavor-name-input" value={form.name} onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))} placeholder="standard.small" />
          </div>
          <div className="flex gap-3">
            <div className="flex-1">
              <label htmlFor="flavor-vcpus" className="block text-sm font-medium mb-1">vCPU</label>
              <Input id="flavor-vcpus" data-testid="flavor-vcpus-input" type="number" min={1} value={form.vcpus} onChange={(e) => setForm((f) => ({ ...f, vcpus: parseInt(e.target.value, 10) || 1 }))} />
            </div>
            <div className="flex-1">
              <label htmlFor="flavor-memory" className="block text-sm font-medium mb-1">メモリ (MB)</label>
              <Input id="flavor-memory" data-testid="flavor-memory-input" type="number" min={128} value={form.ram_mb} onChange={(e) => setForm((f) => ({ ...f, ram_mb: parseInt(e.target.value, 10) || 1024 }))} />
            </div>
            <div className="flex-1">
              <label htmlFor="flavor-disk" className="block text-sm font-medium mb-1">ディスク (GB)</label>
              <Input id="flavor-disk" data-testid="flavor-disk-input" type="number" min={1} value={form.disk_gb} onChange={(e) => setForm((f) => ({ ...f, disk_gb: parseInt(e.target.value, 10) || 20 }))} />
            </div>
          </div>
          <div className="flex justify-end gap-2 mt-1">
            <Button variant="secondary" size="sm" onClick={() => setShowCreate(false)} disabled={creating}>キャンセル</Button>
            <Button data-testid="create-flavor-submit" size="sm" onClick={handleCreate} disabled={creating}>{creating ? '作成中...' : '作成'}</Button>
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
        data-testid="confirm-delete-dialog"
        confirmButtonTestId="confirm-delete-button"
      />
    </Section>
  )
}

export function ComputePage() {
  return (
    <div>
      <h1 className="text-xl font-semibold text-[var(--color-text)] mb-6">コンピュート管理</h1>
      <FlavorsSection />
    </div>
  )
}
