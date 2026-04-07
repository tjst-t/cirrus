import { cn } from '@/lib/utils'

export function StatusBadge({ status }: { status: 'active' | 'running' | 'deleting' | 'error' | string }) {
  const isActive = status === 'active' || status === 'running'
  return (
    <span className={cn(
      'inline-flex items-center px-2 py-0.5 rounded text-xs font-medium',
      isActive
        ? 'bg-success text-white'
        : 'bg-[var(--color-bg-secondary)] text-[var(--color-text-secondary)] border border-[var(--color-border)]',
    )}>
      {status}
    </span>
  )
}
