import { ArrowRightIcon, BookOpenIcon, ShieldCheckIcon, ShieldOffIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'

import { useRoleName, useScopeLabel } from '@/features/account/scope'
import { usePermissions } from '@/features/auth/usePermissions'
import { navGroups } from '@/features/shell/nav'
import { formatDate, formatDateTime } from '@/i18n/format'
import { useGetMeQuery } from '@/shared/api/generated'
import { Badge } from '@/shared/ui/badge'
import { Button } from '@/shared/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/shared/ui/card'
import { Skeleton } from '@/shared/ui/skeleton'
import { ErrorState } from '@/shared/ui/states'

import { WindowsCard } from './WindowsCard'

/** DashboardPage, oturum açan kullanıcının ana sayfasıdır: rolleri, hesap güvenliği, açık işlem pencereleri ve kısayollar. */
export function DashboardPage() {
  const { t } = useTranslation()
  const { data: me, error, refetch } = useGetMeQuery()
  const { has } = usePermissions()
  const scopeLabel = useScopeLabel()
  const roleName = useRoleName()

  if (error) {
    return <ErrorState error={error} onRetry={() => void refetch()} />
  }
  if (!me) {
    return (
      <div className="grid gap-4 md:grid-cols-2">
        <Skeleton className="h-10 w-64 md:col-span-2" />
        <Skeleton className="h-40" />
        <Skeleton className="h-40" />
      </div>
    )
  }

  const shortcuts = navGroups
    .flatMap((g) => g.items)
    .filter((i) => i.to !== '/' && (!i.permission || has(i.permission)))

  return (
    <div className="space-y-6">
      <div className="space-y-1">
        <h1 className="text-2xl font-semibold tracking-tight">
          {t('dashboard.greeting', { name: me.first_name })}
        </h1>
        <p className="text-sm text-muted-foreground">
          {new Intl.DateTimeFormat(document.documentElement.lang, { dateStyle: 'full' }).format(new Date())}
        </p>
      </div>

      <div className="grid gap-4 md:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle>{t('dashboard.roles.title')}</CardTitle>
          </CardHeader>
          <CardContent>
            {me.roles.length === 0 ? (
              <p className="text-sm text-muted-foreground">{t('dashboard.roles.empty')}</p>
            ) : (
              <ul className="space-y-3">
                {me.roles.map((r) => (
                  <li
                    key={`${r.role}-${r.scope_id ?? ''}`}
                    className="flex items-start justify-between gap-3"
                  >
                    <div>
                      <p className="text-sm font-medium">{roleName(r)}</p>
                      <p className="text-xs text-muted-foreground">{scopeLabel(r)}</p>
                    </div>
                    {r.valid_until && (
                      <Badge variant="muted">
                        {t('dashboard.roles.until', { date: formatDate(r.valid_until) })}
                      </Badge>
                    )}
                  </li>
                ))}
              </ul>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>{t('dashboard.security.title')}</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="flex items-center gap-3">
              {me.mfa_enabled ? (
                <ShieldCheckIcon className="size-5 text-success" aria-hidden />
              ) : (
                <ShieldOffIcon className="size-5 text-warning-foreground dark:text-warning" aria-hidden />
              )}
              <p className="text-sm font-medium">
                {t(me.mfa_enabled ? 'dashboard.security.mfaOn' : 'dashboard.security.mfaOff')}
              </p>
            </div>
            <p className="text-sm text-muted-foreground">
              {t('dashboard.security.lastLogin', { time: formatDateTime(me.last_login_at) })}
            </p>
            <Button variant="outline" size="sm" asChild>
              <Link to="/guvenlik">{t('dashboard.security.manage')}</Link>
            </Button>
          </CardContent>
        </Card>

        {has('calendar:read') && <WindowsCard />}

        <Card>
          <CardHeader>
            <CardTitle>{t('dashboard.shortcuts.title')}</CardTitle>
          </CardHeader>
          <CardContent>
            <ul className="divide-y">
              {shortcuts.map(({ to, label, icon: Icon }) => (
                <li key={to}>
                  <Link
                    to={to}
                    className="flex items-center gap-3 py-2.5 text-sm font-medium transition-colors hover:text-primary"
                  >
                    <Icon className="size-4 text-muted-foreground" aria-hidden />
                    <span className="flex-1">{t(label)}</span>
                    <ArrowRightIcon className="size-4 text-muted-foreground" aria-hidden />
                  </Link>
                </li>
              ))}
            </ul>
          </CardContent>
        </Card>

        <Card className="border-dashed">
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <BookOpenIcon className="size-4 text-primary" aria-hidden />
              {t('dashboard.upcoming.title')}
              <Badge variant="secondary">{t('common.comingSoon')}</Badge>
            </CardTitle>
            <CardDescription>{t('dashboard.upcoming.description')}</CardDescription>
          </CardHeader>
        </Card>
      </div>
    </div>
  )
}
