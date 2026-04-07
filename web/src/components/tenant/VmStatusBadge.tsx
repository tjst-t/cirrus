import { cn } from '@/lib/utils'
import type { Vm } from '@/api/vms'

const styles: Record<Vm['status'], string> = {
  running: 'bg-success text-white',
  stopped: 'bg-[var(--color-bg-secondary)] text-[var(--color-text-secondary)] border border-[var(--color-border)]',
  starting: 'bg-warning text-white',
  stopping: 'bg-warning text-white',
  error: 'bg-danger text-white',
  pending: 'bg-[var(--color-bg-secondary)] text-[var(--color-text-secondary)] border border-[var(--color-border)]',
}

const labels: Record<Vm['status'], string> = {
  running: '実行中',
  stopped: '停止',
  starting: '起動中',
  stopping: '停止中',
  error: 'エラー',
  pending: '保留',
}

export function VmStatusBadge({ status }: { status: Vm['status'] }) {
  return (
    <span className={cn('inline-flex items-center px-2 py-0.5 rounded text-xs font-medium', styles[status])}>
      {labels[status]}
    </span>
  )
}
