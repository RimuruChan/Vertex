import { Navigate, Outlet, useLocation } from 'react-router-dom'
import { useAuth } from '@/auth/AuthContext'
import { PageSpinner } from '@/components/ui/misc'

export function RequireLogin() {
  const location = useLocation()
  const { user, ready } = useAuth()
  if (!ready) return <PageSpinner />
  if (!user) {
    return <Navigate to="/login" replace state={{ from: `${location.pathname}${location.search}` }} />
  }
  return <Outlet />
}

export function RequireAdmin() {
  const { user, ready } = useAuth()
  if (!ready) return <PageSpinner />
  if (!user) return <RequireLogin />
  if (user.role !== 'admin') return <Navigate to="/" replace />
  return <Outlet />
}
