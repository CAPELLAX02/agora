import { AtSignIcon, ClockIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { navTheme } from '@/features/shell/nav'
import { formatDate, formatDateTime } from '@/i18n/format'
import { useGetMeQuery } from '@/shared/api/generated'
import { Avatar } from '@/shared/ui/avatar'
import { Badge } from '@/shared/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/shared/ui/card'
import { DescriptionList } from '@/shared/ui/description-list'
import { PageHeader } from '@/shared/ui/page-header'
import { Skeleton } from '@/shared/ui/skeleton'
import { ErrorState } from '@/shared/ui/states'
import { StatusBadge } from '@/shared/ui/status-badge'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/shared/ui/table'

import { useRoleName, useScopeLabel } from './scope'

export function ProfilePage() {
  const { t } = useTranslation()
  const { data: me, error, refetch } = useGetMeQuery()
  const scopeLabel = useScopeLabel()
  const roleName = useRoleName()

  return (
    <>
      <PageHeader
        {...navTheme('/profil')}
        title={t('profile.title')}
        description={t('profile.description')}
      />
      {error ? (
        <ErrorState error={error} onRetry={() => void refetch()} />
      ) : !me ? (
        <Skeleton className="h-64" />
      ) : (
        <div className="grid gap-6">
          <Card className="overflow-hidden pt-0">
            <div aria-hidden className="relative h-24 bg-brand-gradient">
              <div className="absolute -top-10 right-16 size-40 rounded-full bg-amber-300/30 blur-2xl" />
              <div className="absolute -bottom-12 left-1/3 size-40 rounded-full bg-sky-300/30 blur-2xl" />
            </div>
            {/* İçerik konumlandırılır: avatar geçişin üstünde çizilsin (altında kalıp kırpılmasın). */}
            <CardContent className="relative z-10 -mt-12 flex flex-col gap-4 sm:flex-row sm:items-end">
              <Avatar
                firstName={me.first_name}
                lastName={me.last_name}
                className="size-24 text-3xl ring-4 ring-card"
              />
              <div className="min-w-0 flex-1 space-y-1.5 pb-1">
                <p className="text-2xl font-bold tracking-tight">
                  {me.first_name} {me.last_name}
                </p>
                <p className="flex flex-wrap items-center gap-x-4 gap-y-1 text-sm text-muted-foreground">
                  <span className="font-mono font-semibold text-foreground">{me.username}</span>
                  <span className="inline-flex items-center gap-1.5">
                    <AtSignIcon className="size-3.5" aria-hidden />
                    {me.email}
                  </span>
                  <span className="inline-flex items-center gap-1.5">
                    <ClockIcon className="size-3.5" aria-hidden />
                    {formatDateTime(me.last_login_at)}
                  </span>
                </p>
              </div>
              <div className="flex flex-wrap gap-2 pb-1">
                {me.roles.map((r) => (
                  <Badge key={`${r.role}-${r.scope_id ?? ''}`} variant="violet">
                    {roleName(r)}
                  </Badge>
                ))}
              </div>
            </CardContent>
          </Card>
          <Card>
            <CardHeader>
              <CardTitle>{t('profile.info')}</CardTitle>
            </CardHeader>
            <CardContent>
              <DescriptionList
                items={[
                  [t('profile.firstName'), me.first_name],
                  [t('profile.lastName'), me.last_name],
                  [t('profile.username'), <span className="font-mono">{me.username}</span>],
                  [t('profile.email'), me.email],
                  [t('profile.status'), <StatusBadge status={me.status} />],
                  [t('profile.lastLogin'), formatDateTime(me.last_login_at)],
                  [
                    t('profile.mfa'),
                    <Badge variant={me.mfa_enabled ? 'success' : 'muted'}>
                      {t(me.mfa_enabled ? 'mfa.on' : 'mfa.off')}
                    </Badge>,
                  ],
                ]}
              />
            </CardContent>
          </Card>
          <Card>
            <CardHeader>
              <CardTitle>{t('profile.roles')}</CardTitle>
            </CardHeader>
            <CardContent>
              {me.roles.length === 0 ? (
                <p className="text-sm text-muted-foreground">{t('dashboard.roles.empty')}</p>
              ) : (
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>{t('profile.role')}</TableHead>
                      <TableHead>{t('profile.scope')}</TableHead>
                      <TableHead>{t('profile.validUntil')}</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {me.roles.map((r) => (
                      <TableRow key={`${r.role}-${r.scope_id ?? ''}`}>
                        <TableCell className="font-medium">{roleName(r)}</TableCell>
                        <TableCell>{scopeLabel(r)}</TableCell>
                        <TableCell>
                          {r.valid_until ? formatDate(r.valid_until) : t('profile.noEnd')}
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              )}
            </CardContent>
          </Card>
        </div>
      )}
    </>
  )
}
