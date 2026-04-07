export function ErrorMessage({ message }: { message: string }) {
  return (
    <div className="rounded-xl border border-danger/30 bg-danger/5 p-4 text-sm text-danger">
      {message}
    </div>
  )
}
