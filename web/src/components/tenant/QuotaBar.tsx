import { cn } from '@/lib/utils'

interface QuotaBarProps {
  label: string
  used: number
  limit: number
  unit?: string
  className?: string
}

export function QuotaBar({ label, used, limit, unit = '', className }: QuotaBarProps) {
  const unlimited = limit === 0
  const pct = unlimited ? 0 : Math.min((used / limit) * 100, 100)
  const isWarning = !unlimited && pct >= 80
  const isDanger = !unlimited && pct >= 95

  return (
    <div className={cn('space-y-1', className)}>
      <div className="flex items-center justify-between text-xs">
        <span className="font-medium text-[var(--color-text)]">{label}</span>
        <span className="text-[var(--color-text-secondary)]">
          {used}{unit} / {unlimited ? '無制限' : `${limit}${unit}`}
        </span>
      </div>
      {!unlimited && (
        <>
          <div className="h-2 bg-[var(--color-bg-secondary)] rounded-full overflow-hidden">
            <div
              className={cn(
                'h-full rounded-full transition-all duration-300',
                isDanger
                  ? 'bg-danger'
                  : isWarning
                    ? 'bg-warning'
                    : 'bg-accent',
              )}
              style={{ width: `${pct}%` }}
            />
          </div>
          <p className="text-xs text-[var(--color-text-secondary)]">
            残り: {limit - used}{unit} ({(100 - pct).toFixed(0)}%)
          </p>
        </>
      )}
    </div>
  )
}
