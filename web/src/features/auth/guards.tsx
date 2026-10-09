import type { ReactNode } from 'react'
import { Navigate, Outlet, useLocation, type Location } from 'react-router'

import { useAppSelector } from '@/app/hooks'
import { Skeleton } from '@/shared/ui/skeleton'
import { ForbiddenState } from '@/shared/ui/states'

import { usePermissions } from './usePermissions'

type FromState = { from?: Location }

/** GuestOnly, oturum açmış kullanıcıyı giriş ekranlarından geldiği yere yönlendirir. */
export function GuestOnly() {
  const status = useAppSelector((s) => s.auth.status)
  const location = useLocation()
  if (status === 'authenticated') {
    const from = (location.state as FromState | null)?.from
    return <Navigate to={from ? `${from.pathname}${from.search}` : '/'} replace />
  }
  return <Outlet />
}

/** RequireAuth, oturumu olmayan kullanıcıyı giriş ekranına yönlendirir ve geri dönülecek yeri hatırlar. */
export function RequireAuth() {
  const status = useAppSelector((s) => s.auth.status)
  const location = useLocation()
  if (status !== 'authenticated') {
    return <Navigate to="/giris" replace state={{ from: location } satisfies FromState} />
  }
  return <Outlet />
}

/** RequirePasswordChanged, parolasını değiştirmesi gereken kullanıcıyı parola ekranına gönderir. */
export function RequirePasswordChanged() {
  const mustChange = useAppSelector((s) => s.auth.mustChangePassword)
  if (mustChange) {
    return <Navigate to="/parola-degistir" replace />
  }
  return <Outlet />
}

/** RequirePermission, yetkisi olmayan kullanıcıya sayfa yerine bir uyarı gösterir. */
export function RequirePermission({ permission, children }: { permission: string; children: ReactNode }) {
  const { has, isLoading } = usePermissions()
  if (isLoading) {
    return <Skeleton className="h-40 w-full" />
  }
  if (!has(permission)) {
    return <ForbiddenState />
  }
  return children
}
