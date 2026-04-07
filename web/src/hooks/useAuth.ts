import { useState, useCallback } from 'react'
import { getToken, setToken, isAuthenticated, logout } from '@/lib/auth'

export function useAuth() {
  const [authenticated, setAuthenticated] = useState(() => isAuthenticated())

  const login = useCallback((token: string) => {
    setToken(token)
    setAuthenticated(true)
  }, [])

  const handleLogout = useCallback(() => {
    logout()
    setAuthenticated(false)
  }, [])

  return {
    authenticated,
    token: getToken(),
    login,
    logout: handleLogout,
  }
}
