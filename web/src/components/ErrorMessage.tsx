import type { ApiError } from '@/lib/errorMessages'
import { getErrorMessage } from '@/lib/errorMessages'

interface ErrorMessageProps {
  // 文字列（後方互換）
  message?: string
  // 構造化エラーオブジェクト
  error?: ApiError
  onDismiss?: () => void
  'data-testid'?: string
}

export function ErrorMessage({ message, error, onDismiss, 'data-testid': testId = 'error-message' }: ErrorMessageProps) {
  const text = error ? getErrorMessage(error) : (message ?? '')
  if (!text) return null
  return (
    <div data-testid={testId} className="flex items-start gap-2 rounded-xl border border-danger/30 bg-danger/5 p-4 text-sm text-danger">
      <span className="flex-1">{text}</span>
      {onDismiss && (
        <button
          onClick={onDismiss}
          aria-label="エラーを閉じる"
          className="shrink-0 text-danger/60 hover:text-danger leading-none"
        >
          ✕
        </button>
      )}
    </div>
  )
}
