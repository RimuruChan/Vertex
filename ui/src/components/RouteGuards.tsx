import { Navigate, Outlet, useLocation } from 'react-router-dom'
import { Spin } from 'antd'
import { useAuth } from '../auth/AuthContext'

export function RequireLogin() {
  const location = useLocation()
  const { user, ready } = useAuth()
  if (!ready) return <Spin fullscreen />
  if (!user) {
    return <Navigate to="/login" replace state={{ from: `${location.pathname}${location.search}` }} />
  }
  return <Outlet />
}

export function RequireAdmin() {
  const { user, ready } = useAuth()
  if (!ready) return <Spin fullscreen />
  if (!user) {
    return <RequireLogin />
  }
  if (user.role !== 'admin') {
    return <Navigate to="/" replace />
  }
  return <Outlet />
}
