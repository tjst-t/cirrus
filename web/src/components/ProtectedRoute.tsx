import { Navigate, Outlet, useLocation } from 'react-router-dom'
import { isAuthenticated } from '@/lib/auth'

export function ProtectedRoute() {
  const location = useLocation()
  if (!isAuthenticated()) {
    const redirect = location.pathname + location.search
    return <Navigate to={`/login?redirect=${encodeURIComponent(redirect)}`} replace />
  }
  return <Outlet />
}
