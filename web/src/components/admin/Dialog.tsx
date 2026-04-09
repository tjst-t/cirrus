import { useEffect, useRef } from 'react'
import { cn } from '@/lib/utils'
import { Button } from '@/components/Button'

interface DialogProps {
  open: boolean
  onClose: () => void
  title: string
  children: React.ReactNode
  className?: string
  'data-testid'?: string
}

export function Dialog({ open, onClose, title, children, className, 'data-testid': testId }: DialogProps) {
  const overlayRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!open) return
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', handler)
    return () => window.removeEventListener('keydown', handler)
  }, [open, onClose])

  if (!open) return null

  return (
    <div
      ref={overlayRef}
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/40"
      onClick={(e) => {
        if (e.target === overlayRef.current) onClose()
      }}
    >
      <div
        data-testid={testId}
        className={cn(
          'bg-white rounded-xl shadow-xl w-full max-w-md mx-4 max-h-[90vh] overflow-auto',
          className,
        )}
      >
        <div className="flex items-center justify-between px-5 py-4 border-b border-[var(--color-border)]">
          <h3 className="text-base font-semibold text-[var(--color-text)]">{title}</h3>
          <button
            onClick={onClose}
            className="text-[var(--color-text-secondary)] hover:text-[var(--color-text)] transition-colors text-lg leading-none"
            aria-label="閉じる"
          >
            ×
          </button>
        </div>
        <div className="px-5 py-4">{children}</div>
      </div>
    </div>
  )
}

interface ConfirmDialogProps {
  open: boolean
  onClose: () => void
  onConfirm: () => void
  title: string
  description: string
  confirmLabel?: string
  loading?: boolean
  'data-testid'?: string
  confirmButtonTestId?: string
}

export function ConfirmDialog({
  open,
  onClose,
  onConfirm,
  title,
  description,
  confirmLabel = '削除',
  loading = false,
  'data-testid': testId,
  confirmButtonTestId,
}: ConfirmDialogProps) {
  return (
    <Dialog open={open} onClose={onClose} title={title} data-testid={testId}>
      <p className="text-sm text-[var(--color-text-secondary)] mb-5">{description}</p>
      <div className="flex justify-end gap-2">
        <Button variant="secondary" size="sm" onClick={onClose} disabled={loading}>
          キャンセル
        </Button>
        <Button variant="danger" size="sm" onClick={onConfirm} disabled={loading} data-testid={confirmButtonTestId}>
          {loading ? '処理中...' : confirmLabel}
        </Button>
      </div>
    </Dialog>
  )
}
