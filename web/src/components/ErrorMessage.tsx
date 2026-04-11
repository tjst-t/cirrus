export function ErrorMessage({ message, 'data-testid': testId = 'error-message' }: { message: string; 'data-testid'?: string }) {
  return (
    <div data-testid={testId} className="rounded-xl border border-danger/30 bg-danger/5 p-4 text-sm text-danger">
      {message}
    </div>
  )
}
