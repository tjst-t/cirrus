import { useState } from 'react'

interface ErrorTooltipProps {
  id: string
  message: string
}

export function ErrorTooltip({ id, message }: ErrorTooltipProps) {
  const [visible, setVisible] = useState(false)
  return (
    <div className="relative inline-flex">
      <button
        data-testid={`vm-error-tooltip-trigger-${id}`}
        onMouseEnter={() => setVisible(true)}
        onMouseLeave={() => setVisible(false)}
        className="w-4 h-4 rounded-full bg-danger text-white text-xs flex items-center justify-center"
        aria-label="エラー詳細"
        type="button"
      >
        !
      </button>
      {visible && (
        <div
          data-testid={`vm-error-tooltip-content-${id}`}
          className="absolute left-6 top-0 z-10 w-64 rounded-lg border border-danger/30 bg-white p-2 text-xs text-danger shadow-md"
        >
          {message}
        </div>
      )}
    </div>
  )
}
