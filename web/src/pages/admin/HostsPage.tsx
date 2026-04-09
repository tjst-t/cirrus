import { useState, useEffect, useCallback } from 'react'
import { hostsApi, type Host, type HostAction, type CreateHostRequest } from '@/api/hosts'
import { Button } from '@/components/Button'
import { Input } from '@/components/Input'
import { Dialog, ConfirmDialog } from '@/components/admin/Dialog'
import { ErrorMessage } from '@/components/admin/ErrorMessage'
import { cn } from '@/lib/utils'

const STATUS_LABELS: Record<Host['operational_state'], string> = {
  active: 'アクティブ',
  draining: 'ドレイン中',
  maintenance: 'メンテナンス',
  retired: '廃止済み',
  provisioning: 'プロビジョニング',
}

const STATUS_COLORS: Record<Host['operational_state'], string> = {
  active: 'bg-green-100 text-green-700',
  draining: 'bg-yellow-100 text-yellow-700',
  maintenance: 'bg-blue-100 text-blue-700',
  retired: 'bg-gray-100 text-gray-500',
  provisioning: 'bg-purple-100 text-purple-700',
}

// Actions available per status
const AVAILABLE_ACTIONS: Record<Host['operational_state'], HostAction[]> = {
  active: ['drain', 'maintenance'],
  draining: ['activate', 'maintenance'],
  maintenance: ['activate', 'retire'],
  retired: [],
  provisioning: ['activate'],
}

const ACTION_LABELS: Record<HostAction, string> = {
  activate: '有効化',
  drain: 'ドレイン',
  maintenance: 'メンテナンス',
  retire: '廃止',
}

const ACTION_VARIANTS: Record<HostAction, 'primary' | 'secondary' | 'danger' | 'ghost'> = {
  activate: 'primary',
  drain: 'secondary',
  maintenance: 'secondary',
  retire: 'danger',
}

function HostRow({ host, onActionComplete }: { host: Host; onActionComplete: () => void }) {
  const [pending, setPending] = useState<HostAction | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [retireConfirmOpen, setRetireConfirmOpen] = useState(false)

  const executeAction = (action: HostAction) => {
    setPending(action)
    setError(null)
    hostsApi
      .action(host.id, action)
      .then(() => onActionComplete())
      .catch((e: Error) => setError(e.message))
      .finally(() => setPending(null))
  }

  const handleAction = (action: HostAction) => {
    if (action === 'retire') {
      setRetireConfirmOpen(true)
      return
    }
    executeAction(action)
  }

  const actions = AVAILABLE_ACTIONS[host.operational_state] ?? []

  return (
    <>
      <tr data-testid={`host-row-${host.id}`} className="border-b border-[var(--color-border)] hover:bg-[var(--color-bg-secondary)] transition-colors">
        <td className="py-3 px-4">
          <p className="font-medium text-sm">{host.name}</p>
          <p className="text-xs font-mono text-[var(--color-text-secondary)]">{host.id}</p>
        </td>
        <td className="py-3 px-4 text-sm text-[var(--color-text-secondary)]">{host.address}</td>
        <td className="py-3 px-4">
          <span
            data-testid={`host-status-${host.id}`}
            className={cn(
              'inline-block px-2 py-0.5 rounded text-xs font-medium',
              STATUS_COLORS[host.operational_state],
            )}
          >
            {STATUS_LABELS[host.operational_state]}
          </span>
        </td>
        <td className="py-3 px-4 text-sm text-[var(--color-text-secondary)]">
          {host.vcpus_used} / {host.vcpus_total}
        </td>
        <td className="py-3 px-4 text-sm text-[var(--color-text-secondary)]">
          {Math.round(host.memory_used_mb / 1024)} / {Math.round(host.memory_total_mb / 1024)} GB
        </td>
        <td className="py-3 px-4">
          <div className="flex items-center gap-1">
            {actions.map((action) => (
              <Button
                key={action}
                data-testid={`host-action-${action}-${host.id}`}
                variant={ACTION_VARIANTS[action]}
                size="sm"
                onClick={() => handleAction(action)}
                disabled={pending !== null}
              >
                {pending === action ? '処理中...' : ACTION_LABELS[action]}
              </Button>
            ))}
          </div>
        </td>
      </tr>
      {error && (
        <tr>
          <td colSpan={6} className="px-4 pb-2">
            <ErrorMessage message={error} />
          </td>
        </tr>
      )}
      <ConfirmDialog
        open={retireConfirmOpen}
        onClose={() => setRetireConfirmOpen(false)}
        onConfirm={() => { setRetireConfirmOpen(false); executeAction('retire') }}
        title="ホストを廃止"
        description={`ホスト "${host.name}" を廃止しますか？この操作は元に戻せません。`}
        confirmLabel="廃止"
        loading={pending === 'retire'}
        data-testid="confirm-retire-dialog"
      />
    </>
  )
}

export function HostsPage() {
  const [hosts, setHosts] = useState<Host[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [showCreate, setShowCreate] = useState(false)
  const [creating, setCreating] = useState(false)
  const [createError, setCreateError] = useState<string | null>(null)
  const [form, setForm] = useState<CreateHostRequest>({
    name: '',
    address: '',
    vcpus_total: 0,
    memory_total_mb: 0,
  })

  const load = useCallback(() => {
    setLoading(true)
    setError(null)
    hostsApi
      .list()
      .then(setHosts)
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => {
    load()
  }, [load])

  const handleCreate = () => {
    if (!form.name.trim() || !form.address.trim()) return
    setCreating(true)
    setCreateError(null)
    hostsApi
      .create(form)
      .then(() => {
        setShowCreate(false)
        setForm({ name: '', address: '', vcpus_total: 0, memory_total_mb: 0 })
        load()
      })
      .catch((e: Error) => setCreateError(e.message))
      .finally(() => setCreating(false))
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-xl font-semibold text-[var(--color-text)]">ホスト管理</h1>
        <Button data-testid="create-host-button" size="sm" onClick={() => setShowCreate(true)}>
          + ホストを追加
        </Button>
      </div>

      {error && (
        <div className="mb-4">
          <ErrorMessage message={error} />
        </div>
      )}

      {loading ? (
        <p className="text-sm text-[var(--color-text-secondary)]">読み込み中...</p>
      ) : hosts.length === 0 ? (
        <div data-testid="empty-hosts" className="rounded-lg border border-[var(--color-border)] bg-white p-8 text-center">
          <p className="text-sm text-[var(--color-text-secondary)]">ホストがありません</p>
        </div>
      ) : (
        <div className="rounded-lg border border-[var(--color-border)] bg-white overflow-hidden">
          <table className="w-full">
            <thead>
              <tr className="border-b border-[var(--color-border)] bg-[var(--color-bg-secondary)]">
                <th className="text-left py-2.5 px-4 text-xs font-medium text-[var(--color-text-secondary)] uppercase tracking-wide">ホスト</th>
                <th className="text-left py-2.5 px-4 text-xs font-medium text-[var(--color-text-secondary)] uppercase tracking-wide">アドレス</th>
                <th className="text-left py-2.5 px-4 text-xs font-medium text-[var(--color-text-secondary)] uppercase tracking-wide">ステータス</th>
                <th className="text-left py-2.5 px-4 text-xs font-medium text-[var(--color-text-secondary)] uppercase tracking-wide">vCPU (使用/総数)</th>
                <th className="text-left py-2.5 px-4 text-xs font-medium text-[var(--color-text-secondary)] uppercase tracking-wide">メモリ (使用/総数)</th>
                <th className="text-left py-2.5 px-4 text-xs font-medium text-[var(--color-text-secondary)] uppercase tracking-wide">アクション</th>
              </tr>
            </thead>
            <tbody>
              {hosts.map((host) => (
                <HostRow key={host.id} host={host} onActionComplete={load} />
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* Create host dialog */}
      <Dialog open={showCreate} onClose={() => setShowCreate(false)} title="ホストを追加" data-testid="create-host-dialog">
        <div className="flex flex-col gap-3">
          <div>
            <label htmlFor="host-name" className="block text-sm font-medium mb-1">ホスト名</label>
            <Input
              id="host-name"
              data-testid="host-name-input"
              value={form.name}
              onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
              placeholder="host-01"
            />
          </div>
          <div>
            <label htmlFor="host-address" className="block text-sm font-medium mb-1">アドレス</label>
            <Input
              id="host-address"
              data-testid="host-address-input"
              value={form.address}
              onChange={(e) => setForm((f) => ({ ...f, address: e.target.value }))}
              placeholder="192.168.1.10"
            />
          </div>
          <div className="flex gap-3">
            <div className="flex-1">
              <label htmlFor="host-vcpus" className="block text-sm font-medium mb-1">vCPU 数</label>
              <Input
                id="host-vcpus"
                data-testid="host-vcpus-input"
                type="number"
                min={1}
                value={form.vcpus_total || ''}
                onChange={(e) =>
                  setForm((f) => ({ ...f, vcpus_total: parseInt(e.target.value, 10) || 0 }))
                }
                placeholder="32"
              />
            </div>
            <div className="flex-1">
              <label htmlFor="host-memory" className="block text-sm font-medium mb-1">メモリ (MB)</label>
              <Input
                id="host-memory"
                data-testid="host-memory-input"
                type="number"
                min={1}
                value={form.memory_total_mb || ''}
                onChange={(e) =>
                  setForm((f) => ({ ...f, memory_total_mb: parseInt(e.target.value, 10) || 0 }))
                }
                placeholder="65536"
              />
            </div>
          </div>
          {createError && <ErrorMessage message={createError} />}
          <div className="flex justify-end gap-2 mt-1">
            <Button
              variant="secondary"
              size="sm"
              onClick={() => setShowCreate(false)}
              disabled={creating}
            >
              キャンセル
            </Button>
            <Button data-testid="create-host-submit" size="sm" onClick={handleCreate} disabled={creating}>
              {creating ? '追加中...' : '追加'}
            </Button>
          </div>
        </div>
      </Dialog>
    </div>
  )
}
