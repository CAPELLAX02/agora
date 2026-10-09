import { useTranslation } from 'react-i18next'

import { useRoleName, useScopeLabel } from './scope'
import { formatDate, formatDateTime } from '@/i18n/format'
import { useGetMeQuery } from '@/shared/api/generated'
import { Badge } from '@/shared/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/shared/ui/card'
import { DescriptionList } from '@/shared/ui/description-list'
import { PageHeader } from '@/shared/ui/page-header'
import { Skeleton } from '@/shared/ui/skeleton'
import { ErrorState } from '@/shared/ui/states'
import { StatusBadge } from '@/shared/ui/status-badge'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/shared/ui/table'

export function ProfilePage() {
  const { t } = useTranslation()
  const { data: me, error, refetch } = useGetMeQuery()
  const scopeLabel = useScopeLabel()
  const roleName = useRoleName()

  return (
    <>
      <PageHeader title={t('profile.title')} description={t('profile.description')} />
      {error ? (
        <ErrorState error={error} onRetry={() => void refetch()} />
      ) : !me ? (
        <Skeleton className="h-64" />
      ) : (
        <div className="grid gap-6">
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
