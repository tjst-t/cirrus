import { useState, type FormEvent } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { setToken, clearToken } from '@/lib/auth'
import { toast } from 'sonner'

export function LoginPage() {
  const [token, setTokenValue] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)
  const navigate = useNavigate()
  const [params] = useSearchParams()

  const isDisabled = token.trim() === '' || token.length > 256 || loading

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    if (isDisabled) return
    setError(null)
    setLoading(true)
    try {
      const res = await fetch('/api/v1/auth/verify', {
        method: 'POST',
        headers: { Authorization: `Bearer ${token.trim()}` },
      })
      if (res.status === 401) {
        clearToken()
        setError('トークンが無効です')
        return
      }
      if (!res.ok) {
        toast.error(<span data-testid="toast-error">サーバーエラーが発生しました</span>)
        return
      }
      setToken(token.trim())
      const raw = params.get('redirect') ?? '/'
      const dest = raw.startsWith('/') && !raw.startsWith('//') ? raw : '/'
      navigate(dest, { replace: true })
    } catch {
      clearToken()
      toast.error(<span data-testid="toast-error">サーバーエラーが発生しました</span>)
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-[var(--color-bg-secondary)]">
      <div
        className="w-full max-w-sm bg-white rounded-lg p-6"
        style={{ boxShadow: '0 2px 8px rgba(0,0,0,0.08)' }}
      >
        <h1 className="text-xl font-semibold mb-1 text-[var(--color-text)]">
          Cirrus IaaS
        </h1>
        <p className="text-sm text-[var(--color-text-secondary)] mb-5">
          APIトークンでサインイン
        </p>

        <form onSubmit={handleSubmit} className="flex flex-col gap-4">
          <div className="flex flex-col gap-1">
            <label
              htmlFor="token"
              className="text-sm font-medium text-[var(--color-text)]"
            >
              APIトークン
            </label>
            <input
              id="token"
              data-testid="token-input"
              type="password"
              placeholder="Bearer トークンを入力"
              value={token}
              onChange={(e) => setTokenValue(e.target.value)}
              maxLength={257}
              autoComplete="current-password"
              className="h-9 w-full rounded-md border border-[var(--color-border)] px-3 text-sm outline-none focus:ring-1 focus:ring-[var(--color-accent)]"
            />
          </div>

          {error && (
            <p data-testid="login-error-message" className="text-sm text-danger">
              {error}
            </p>
          )}

          <button
            type="submit"
            data-testid="login-button"
            disabled={isDisabled}
            className="h-9 w-full rounded-md bg-[var(--color-accent)] px-4 text-sm font-medium text-white transition-colors hover:opacity-90 disabled:opacity-40 disabled:cursor-not-allowed"
          >
            {loading ? (
              <span data-testid="login-spinner" className="inline-flex items-center gap-2">
                <svg className="h-4 w-4 animate-spin" viewBox="0 0 24 24" fill="none">
                  <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
                  <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z" />
                </svg>
                ログイン中…
              </span>
            ) : 'サインイン'}
          </button>
        </form>
      </div>
    </div>
  )
}
