import { useEffect, useState } from 'react'
import { volumesApi, type Volume, type CreateVolumeRequest } from '@/api/volumes'
import { vmsApi, type VolumeType } from '@/api/vms'
import { Button } from '@/components/Button'
import { ErrorMessage } from '@/components/ErrorMessage'
import { cn } from '@/lib/utils'

function VolumeStatusBadge({ status }: { status: Volume['status'] }) {
  const styles: Record<Volume['status'], string> = {
    available: 'bg-success text-white',
    'in-use': 'bg-accent text-white',
    creating: 'bg-warning text-white',
    deleting: 'bg-warning text-white',
    error: 'bg-danger text-white',
  }
  const labels: Record<Volume['status'], string> = {
    available: '利用可能',
    'in-use': '使用中',
    creating: '作成中',
    deleting: '削除中',
    error: 'エラー',
  }
  return (
    <span className={cn('inline-flex items-center px-2 py-0.5 rounded text-xs font-medium', styles[status])}>
      {labels[status]}
    </span>
  )
}

interface CreateVolumeDialogProps {
  onClose: () => void
  onCreated: () => void
}

function CreateVolumeDialog({ onClose, onCreated }: CreateVolumeDialogProps) {
  const [form, setForm] = useState<CreateVolumeRequest>({
    name: '',
    size_gb: 20,
    volume_type_id: '',
  })
  const [volumeTypes, setVolumeTypes] = useState<VolumeType[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    vmsApi.listVolumeTypes()
      .then(setVolumeTypes)
      .catch(() => {})
  }, [])

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setLoading(true)
    try {
      const req: CreateVolumeRequest = {
        name: form.name,
        size_gb: form.size_gb,
      }
      if (form.volume_type_id) req.volume_type_id = form.volume_type_id
      await volumesApi.create(req)
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
        <h3 className="text-base font-semibold text-[var(--color-text)] mb-4">ボリュームを作成</h3>
        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label className="block text-xs font-medium text-[var(--color-text)] mb-1">名前 *</label>
            <input
              type="text"
              value={form.name}
              onChange={(e) => setForm((p) => ({ ...p, name: e.target.value }))}
              required
              className="w-full h-9 px-3 text-sm border border-[var(--color-border)] rounded focus:outline-none focus:ring-2 focus:ring-accent/30"
              placeholder="my-volume"
            />
          </div>
          <div>
            <label className="block text-xs font-medium text-[var(--color-text)] mb-1">サイズ (GB) *</label>
            <input
              type="number"
              value={form.size_gb}
              onChange={(e) => setForm((p) => ({ ...p, size_gb: Number(e.target.value) }))}
              min={1}
              required
              className="w-full h-9 px-3 text-sm border border-[var(--color-border)] rounded focus:outline-none focus:ring-2 focus:ring-accent/30"
            />
          </div>
          {volumeTypes.length > 0 && (
            <div>
              <label className="block text-xs font-medium text-[var(--color-text)] mb-1">ボリュームタイプ</label>
              <select
                value={form.volume_type_id}
                onChange={(e) => setForm((p) => ({ ...p, volume_type_id: e.target.value }))}
                className="w-full h-9 px-3 text-sm border border-[var(--color-border)] rounded focus:outline-none focus:ring-2 focus:ring-accent/30 bg-white"
              >
                <option value="">デフォルト</option>
                {volumeTypes.map((vt) => (
                  <option key={vt.id} value={vt.id}>{vt.name}</option>
                ))}
              </select>
            </div>
          )}
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

interface ResizeDialogProps {
  volume: Volume
  onClose: () => void
  onResized: () => void
}

function ResizeDialog({ volume, onClose, onResized }: ResizeDialogProps) {
  const [sizeGb, setSizeGb] = useState(volume.size_gb)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (sizeGb <= volume.size_gb) {
      setError('現在のサイズより大きい値を指定してください')
      return
    }
    setLoading(true)
    try {
      await volumesApi.resize(volume.id, { size_gb: sizeGb })
      onResized()
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
      <div className="relative bg-white rounded-xl border border-[var(--color-border)] p-6 w-full max-w-sm shadow-lg">
        <h3 className="text-base font-semibold text-[var(--color-text)] mb-2">ボリュームをリサイズ</h3>
        <p className="text-xs text-[var(--color-text-secondary)] mb-4">
          現在: {volume.size_gb} GB（拡張のみ可能）
        </p>
        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label className="block text-xs font-medium text-[var(--color-text)] mb-1">新しいサイズ (GB)</label>
            <input
              type="number"
              value={sizeGb}
              onChange={(e) => setSizeGb(Number(e.target.value))}
              min={volume.size_gb + 1}
              required
              className="w-full h-9 px-3 text-sm border border-[var(--color-border)] rounded focus:outline-none focus:ring-2 focus:ring-accent/30"
            />
          </div>
          {error && <p className="text-xs text-danger">{error}</p>}
          <div className="flex gap-2 justify-end">
            <Button type="button" variant="ghost" size="sm" onClick={onClose}>キャンセル</Button>
            <Button type="submit" variant="primary" size="sm" disabled={loading}>
              {loading ? '処理中...' : 'リサイズ'}
            </Button>
          </div>
        </form>
      </div>
    </div>
  )
}

export function VolumesPage() {
  const [volumes, setVolumes] = useState<Volume[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [showCreate, setShowCreate] = useState(false)
  const [deleteId, setDeleteId] = useState<string | null>(null)
  const [resizeVolume, setResizeVolume] = useState<Volume | null>(null)

  const load = () => {
    setLoading(true)
    volumesApi.list()
      .then(setVolumes)
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false))
  }

  useEffect(() => { load() }, [])

  const handleDelete = async (id: string) => {
    try {
      await volumesApi.delete(id)
      setDeleteId(null)
      load()
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'エラーが発生しました')
    }
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold text-[var(--color-text)]">ボリューム管理</h2>
        <Button variant="primary" size="sm" onClick={() => setShowCreate(true)}>
          + ボリュームを作成
        </Button>
      </div>

      {error && <ErrorMessage message={error} />}

      {loading ? (
        <div className="flex items-center justify-center h-40 text-[var(--color-text-secondary)] text-sm">読み込み中...</div>
      ) : (
        <div className="bg-white rounded-xl border border-[var(--color-border)] overflow-hidden">
          <table className="w-full text-sm">
            <thead className="bg-[var(--color-bg-secondary)]">
              <tr>
                <th className="text-left px-4 py-3 text-xs font-medium text-[var(--color-text-secondary)]">名前</th>
                <th className="text-left px-4 py-3 text-xs font-medium text-[var(--color-text-secondary)]">サイズ</th>
                <th className="text-left px-4 py-3 text-xs font-medium text-[var(--color-text-secondary)]">状態</th>
                <th className="text-left px-4 py-3 text-xs font-medium text-[var(--color-text-secondary)]">アタッチ先</th>
                <th className="text-left px-4 py-3 text-xs font-medium text-[var(--color-text-secondary)]">作成日時</th>
                <th className="text-left px-4 py-3 text-xs font-medium text-[var(--color-text-secondary)]">操作</th>
              </tr>
            </thead>
            <tbody>
              {volumes.length === 0 ? (
                <tr>
                  <td colSpan={6} className="px-4 py-8 text-center text-[var(--color-text-secondary)]">
                    ボリュームがありません
                  </td>
                </tr>
              ) : (
                volumes.map((vol) => (
                  <tr key={vol.id} className="border-t border-[var(--color-border)] hover:bg-[var(--color-bg-secondary)]/50">
                    <td className="px-4 py-3 font-medium">{vol.name}</td>
                    <td className="px-4 py-3 text-[var(--color-text-secondary)]">{vol.size_gb} GB</td>
                    <td className="px-4 py-3">
                      <VolumeStatusBadge status={vol.status} />
                    </td>
                    <td className="px-4 py-3 text-[var(--color-text-secondary)] font-mono text-xs">
                      {vol.attached_vm_id ? vol.attached_vm_id.slice(0, 8) + '...' : '—'}
                    </td>
                    <td className="px-4 py-3 text-[var(--color-text-secondary)]">
                      {new Date(vol.created_at).toLocaleDateString('ja-JP')}
                    </td>
                    <td className="px-4 py-3">
                      <div className="flex gap-1">
                        <Button
                          variant="secondary"
                          size="sm"
                          onClick={() => setResizeVolume(vol)}
                          disabled={vol.status === 'deleting' || vol.status === 'creating'}
                        >
                          リサイズ
                        </Button>
                        <Button
                          variant="danger"
                          size="sm"
                          onClick={() => setDeleteId(vol.id)}
                          disabled={vol.status === 'in-use'}
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
        <CreateVolumeDialog onClose={() => setShowCreate(false)} onCreated={load} />
      )}

      {resizeVolume && (
        <ResizeDialog
          volume={resizeVolume}
          onClose={() => setResizeVolume(null)}
          onResized={load}
        />
      )}

      {deleteId && (
        <div className="fixed inset-0 z-50 flex items-center justify-center">
          <div className="absolute inset-0 bg-black/40" onClick={() => setDeleteId(null)} />
          <div className="relative bg-white rounded-xl border border-[var(--color-border)] p-6 w-full max-w-sm shadow-lg">
            <h3 className="text-base font-semibold text-[var(--color-text)] mb-2">ボリュームを削除</h3>
            <p className="text-sm text-[var(--color-text-secondary)] mb-4">
              このボリュームを削除してもよろしいですか？この操作は取り消せません。
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
