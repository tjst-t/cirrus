export function ErrorMessage({ message, 'data-testid': testId }: { message: string; 'data-testid'?: string }) {
  return (
    <div
      data-testid={testId}
      className="rounded border border-[var(--color-danger)] bg-[var(--color-danger,#fef2f2)] px-3 py-2 text-sm text-[var(--color-danger)]"
    >
      {message}
    </div>
  )
}
