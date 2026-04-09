import { useState, useEffect } from 'react'
import {
  driftEventsApi,
  type DriftEvent,
  type DriftEventResourceType,
  type DriftEventStatus,
  type ListDriftEventsParams,
} from '@/api/driftEvents'
import { Button } from '@/components/Button'
import { ErrorMessage } from '@/components/admin/ErrorMessage'

export function DriftEventsPage() {
  const [events, setEvents] = useState<DriftEvent[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [resourceTypeFilter, setResourceTypeFilter] = useState<DriftEventResourceType | ''>('')
  const [statusFilter, setStatusFilter] = useState<DriftEventStatus | ''>('')

  useEffect(() => {
    const ac = new AbortController()
    setLoading(true)
    setError(null)
    const params: ListDriftEventsParams = {}
    if (resourceTypeFilter) params.resource_type = resourceTypeFilter
    if (statusFilter) params.status = statusFilter
    driftEventsApi
      .list(params)
      .then((data) => { if (!ac.signal.aborted) setEvents(data) })
      .catch((e: Error) => { if (!ac.signal.aborted) setError(e.message) })
      .finally(() => { if (!ac.signal.aborted) setLoading(false) })
    return () => ac.abort()
  }, [resourceTypeFilter, statusFilter])

  const handleResolve = (id: string) => {
    driftEventsApi
      .resolve(id)
      .then((updated) => {
        setEvents((prev) => prev.map((e) => (e.id === id ? updated : e)))
      })
      .catch((e: Error) => setError(e.message))
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-xl font-semibold text-[var(--color-text)]">Drift Event ビューア</h1>
      </div>

      {/* Filters */}
      <div className="flex gap-3 mb-4">
        <div>
          <label className="block text-xs font-medium mb-1 text-[var(--color-text-secondary)]">
            リソース種別
          </label>
          <select
            data-testid="drift-filter-resource-type"
            value={resourceTypeFilter}
            onChange={(e) => setResourceTypeFilter(e.target.value as DriftEventResourceType | '')}
            className="rounded border border-[var(--color-border)] bg-white px-3 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-[var(--color-primary)]"
          >
            <option value="">すべて</option>
            <option value="vm">VM</option>
            <option value="host">ホスト</option>
          </select>
        </div>
        <div>
          <label className="block text-xs font-medium mb-1 text-[var(--color-text-secondary)]">
            ステータス
          </label>
          <select
            data-testid="drift-filter-status"
            value={statusFilter}
            onChange={(e) => setStatusFilter(e.target.value as DriftEventStatus | '')}
            className="rounded border border-[var(--color-border)] bg-white px-3 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-[var(--color-primary)]"
          >
            <option value="">すべて</option>
            <option value="open">未解決</option>
            <option value="resolved">解決済み</option>
          </select>
        </div>
      </div>

      {error && (
        <div className="mb-4">
          <ErrorMessage message={error} />
        </div>
      )}

      {loading ? (
        <p className="text-sm text-[var(--color-text-secondary)]">読み込み中...</p>
      ) : events.length === 0 ? (
        <div
          data-testid="empty-drift-events"
          className="rounded-lg border border-[var(--color-border)] bg-white p-8 text-center"
        >
          <p className="text-sm text-[var(--color-text-secondary)]">Drift Event はありません</p>
        </div>
      ) : (
        <div className="rounded-lg border border-[var(--color-border)] bg-white overflow-hidden">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-[var(--color-border)] bg-[var(--color-bg-secondary)]">
                <th className="px-4 py-3 text-left font-medium text-[var(--color-text-secondary)]">
                  リソース種別
                </th>
                <th className="px-4 py-3 text-left font-medium text-[var(--color-text-secondary)]">
                  リソース ID
                </th>
                <th className="px-4 py-3 text-left font-medium text-[var(--color-text-secondary)]">
                  説明
                </th>
                <th className="px-4 py-3 text-left font-medium text-[var(--color-text-secondary)]">
                  ステータス
                </th>
                <th className="px-4 py-3 text-left font-medium text-[var(--color-text-secondary)]">
                  検知日時
                </th>
                <th className="px-4 py-3" />
              </tr>
            </thead>
            <tbody>
              {events.map((ev) => (
                <tr
                  key={ev.id}
                  data-testid={`drift-row-${ev.id}`}
                  className="border-b border-[var(--color-border)] last:border-0 hover:bg-[var(--color-bg-secondary)]"
                >
                  <td className="px-4 py-3 font-mono text-xs">{ev.resource_type}</td>
                  <td className="px-4 py-3 font-mono text-xs truncate max-w-[12rem]">
                    {ev.resource_id}
                  </td>
                  <td className="px-4 py-3 text-[var(--color-text)]">{ev.description}</td>
                  <td className="px-4 py-3">
                    <span
                      data-testid={`drift-status-${ev.id}`}
                      className={`inline-block rounded-full px-2 py-0.5 text-xs font-medium ${
                        ev.status === 'open'
                          ? 'bg-red-100 text-red-700'
                          : 'bg-green-100 text-green-700'
                      }`}
                    >
                      {ev.status}
                    </span>
                  </td>
                  <td className="px-4 py-3 text-xs text-[var(--color-text-secondary)]">
                    {new Date(ev.detected_at).toLocaleString('ja-JP')}
                  </td>
                  <td className="px-4 py-3">
                    {ev.status === 'open' && (
                      <Button
                        data-testid={`resolve-drift-button-${ev.id}`}
                        variant="secondary"
                        size="sm"
                        onClick={() => handleResolve(ev.id)}
                      >
                        解決済みにする
                      </Button>
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
