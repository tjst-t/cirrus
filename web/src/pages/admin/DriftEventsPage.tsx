// Drift Events list API (/admin/drift-events) is not yet implemented in the backend.
// This page will be available once the API endpoint is added.

export function DriftEventsPage() {
  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-xl font-semibold text-[var(--color-text)]">Drift Event ビューア</h1>
      </div>
      <div className="rounded-lg border border-[var(--color-border)] bg-white p-8 text-center">
        <p className="text-sm font-medium text-[var(--color-text)]">このページは準備中です</p>
        <p className="text-xs text-[var(--color-text-secondary)] mt-1">
          Drift Event 一覧 API（GET /admin/drift-events）はまだ実装されていません。
        </p>
      </div>
    </div>
  )
}
