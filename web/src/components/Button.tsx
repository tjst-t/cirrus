import { forwardRef } from 'react'
import { cn } from '@/lib/utils'

export interface ButtonProps
  extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: 'primary' | 'secondary' | 'danger' | 'ghost'
  size?: 'sm' | 'md' | 'lg'
}

const Button = forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant = 'primary', size = 'md', ...props }, ref) => {
    return (
      <button
        ref={ref}
        className={cn(
          'inline-flex items-center justify-center font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-offset-2 disabled:pointer-events-none disabled:opacity-50',
          {
            'bg-accent text-white hover:bg-accent-hover focus-visible:ring-accent':
              variant === 'primary',
            'border border-[var(--color-border)] bg-white text-[var(--color-text)] hover:bg-[var(--color-bg-secondary)]':
              variant === 'secondary',
            'bg-danger text-white hover:opacity-90 focus-visible:ring-danger':
              variant === 'danger',
            'text-[var(--color-text)] hover:bg-[var(--color-bg-secondary)]':
              variant === 'ghost',
          },
          {
            'h-7 px-2 text-xs rounded': size === 'sm',
            'h-9 px-4 text-sm rounded': size === 'md',
            'h-11 px-6 text-base rounded-lg': size === 'lg',
          },
          className,
        )}
        {...props}
      />
    )
  },
)

Button.displayName = 'Button'

export { Button }
