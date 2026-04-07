export function ErrorMessage({ message }: { message: string }) {
  return (
    <div className="rounded border border-[var(--color-danger)] bg-[var(--color-danger,#fef2f2)] px-3 py-2 text-sm text-[var(--color-danger)]">
      {message}
    </div>
  )
}
