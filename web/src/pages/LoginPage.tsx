import { useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { setToken, clearToken } from '@/lib/auth'
import { Button } from '@/components/Button'
import { Input } from '@/components/Input'

export function LoginPage() {
  const [token, setTokenValue] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)
  const navigate = useNavigate()

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError(null)

    const trimmed = token.trim()
    if (!trimmed) {
      setError('トークンを入力してください')
      return
    }

    setLoading(true)
    try {
      // Verify token before storing: validate by making a real API call.
      const res = await fetch('/api/v1/organizations', {
        headers: { Authorization: `Bearer ${trimmed}` },
      })
      if (res.status === 401) {
        clearToken()
        setError('トークンが無効です')
        return
      }
      setToken(trimmed)
      navigate('/', { replace: true })
    } catch {
      clearToken()
      setError('サーバーに接続できません')
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
            <Input
              id="token"
              type="password"
              placeholder="Bearer トークンを入力"
              value={token}
              onChange={(e) => setTokenValue(e.target.value)}
              autoComplete="current-password"
            />
          </div>

          {error && (
            <p className="text-sm text-danger">{error}</p>
          )}

          <Button type="submit" disabled={loading} className="w-full">
            {loading ? 'ログイン中…' : 'サインイン'}
          </Button>
        </form>
      </div>
    </div>
  )
}
