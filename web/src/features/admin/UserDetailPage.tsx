import { ArrowLeftIcon, KeyRoundIcon, MailIcon, PlusIcon, ShieldOffIcon, UserCogIcon } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useParams } from 'react-router'
import { toast } from 'sonner'

import { useRoleName, useScopeLabel } from '@/features/account/scope'
import { usePermissions } from '@/features/auth/usePermissions'
import { formatDateTime } from '@/i18n/format'
import { httpStatus, type AnyError } from '@/shared/api/errors'
import {
  useGetUserQuery,
  useListUserRolesQuery,
  useResendActivationEmailMutation,
  useSendPasswordResetEmailMutation,
  type Assignment,
} from '@/shared/api/generated'
import { useErrorMessage } from '@/shared/api/useErrorMessage'
import { Avatar } from '@/shared/ui/avatar'
import { Badge } from '@/shared/ui/badge'
import { Button } from '@/shared/ui/button'
import { Card, CardAction, CardContent, CardDescription, CardHeader, CardTitle } from '@/shared/ui/card'
import { DescriptionList } from '@/shared/ui/description-list'
import { Skeleton } from '@/shared/ui/skeleton'
import { EmptyState, ErrorState } from '@/shared/ui/states'
import { StatusBadge } from '@/shared/ui/status-badge'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/shared/ui/table'

import { useAccountErrors } from './accountErrors'
import { AssignRoleDialog, EndRoleDialog, ResetMfaDialog, StatusDialog } from './userDialogs'

type DialogState =
  { kind: 'status' } | { kind: 'mfa' } | { kind: 'assign' } | { kind: 'end'; assignment: Assignment } | null

/** UserDetailPage, bir hesabın bilgileri, hesap işlemleri ve rol atamalarıdır. */
export function UserDetailPage() {
  const { t } = useTranslation()
  const { id = '' } = useParams()
  const { has } = usePermissions()
  const user = useGetUserQuery({ id })
  const [dialog, setDialog] = useState<DialogState>(null)

  const back = (
    <Button variant="link" asChild className="mb-2 px-0">
      <Link to="/yonetim/kullanicilar">
        <ArrowLeftIcon />
        {t('userDetail.back')}
      </Link>
    </Button>
  )

  if (user.error) {
    return (
      <>
        {back}
        {httpStatus(user.error) === 404 ? (
          <EmptyState>{t('errors.notFound')}</EmptyState>
        ) : (
          <ErrorState error={user.error} onRetry={() => void user.refetch()} />
        )}
      </>
    )
  }
  const u = user.data
  if (!u) {
    return (
      <>
        {back}
        <Skeleton className="h-64" />
      </>
    )
  }

  return (
    <>
      {back}
      <div className="mb-6 flex flex-wrap items-center gap-4">
        <Avatar firstName={u.first_name} lastName={u.last_name} className="size-12 text-base" />
        <div className="space-y-1">
          <h1 className="text-2xl font-semibold tracking-tight">
            {u.first_name} {u.last_name}
          </h1>
          <div className="flex flex-wrap items-center gap-2">
            <span className="font-mono text-sm text-muted-foreground">{u.username}</span>
            <StatusBadge status={u.status} />
            <Badge variant={u.mfa_enabled ? 'success' : 'muted'}>
              {t('profile.mfa')}: {t(u.mfa_enabled ? 'mfa.on' : 'mfa.off')}
            </Badge>
          </div>
        </div>
      </div>

      <div className="grid gap-6 lg:grid-cols-3">
        <Card className="lg:col-span-2">
          <CardHeader>
            <CardTitle>{t('userDetail.account')}</CardTitle>
          </CardHeader>
          <CardContent>
            <DescriptionList
              items={[
                [t('profile.email'), u.email],
                [t('profile.username'), <span className="font-mono">{u.username}</span>],
                [t('profile.status'), <StatusBadge status={u.status} />],
                [t('profile.lastLogin'), formatDateTime(u.last_login_at)],
              ]}
            />
          </CardContent>
        </Card>

        {has('user:manage') && (
          <AccountActions
            userId={u.id}
            status={u.status}
            mfaEnabled={u.mfa_enabled}
            onStatus={() => setDialog({ kind: 'status' })}
            onResetMfa={() => setDialog({ kind: 'mfa' })}
          />
        )}

        <RolesCard
          userId={u.id}
          canAssign={has('role:assign')}
          onAssign={() => setDialog({ kind: 'assign' })}
          onEnd={(assignment) => setDialog({ kind: 'end', assignment })}
        />
      </div>

      {dialog?.kind === 'status' && <StatusDialog user={u} onClose={() => setDialog(null)} />}
      {dialog?.kind === 'mfa' && <ResetMfaDialog userId={u.id} onClose={() => setDialog(null)} />}
      {dialog?.kind === 'assign' && <AssignRoleDialog userId={u.id} onClose={() => setDialog(null)} />}
      {dialog?.kind === 'end' && (
        <EndRoleDialog userId={u.id} assignment={dialog.assignment} onClose={() => setDialog(null)} />
      )}
    </>
  )
}

function AccountActions({
  userId,
  status,
  mfaEnabled,
  onStatus,
  onResetMfa,
}: {
  userId: string
  status: string
  mfaEnabled: boolean
  onStatus: () => void
  onResetMfa: () => void
}) {
  const { t } = useTranslation()
  const message = useErrorMessage()
  const accountErrors = useAccountErrors()
  const [resend, resendState] = useResendActivationEmailMutation()
  const [sendReset, resetState] = useSendPasswordResetEmailMutation()

  const notify = (error: AnyError, ok: string) => {
    if (error) {
      toast.error(message(error, accountErrors))
    } else {
      toast.success(ok)
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('userDetail.actions.title')}</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col items-stretch gap-2">
        <Button
          variant="outline"
          className="h-auto min-h-9 justify-start text-left whitespace-normal"
          onClick={onStatus}
        >
          <UserCogIcon />
          {t('userDetail.actions.changeStatus')}
        </Button>
        {status === 'PENDING' && (
          <Button
            variant="outline"
            className="h-auto min-h-9 justify-start text-left whitespace-normal"
            loading={resendState.isLoading}
            onClick={() =>
              void resend({ id: userId }).then((r) => notify(r.error, t('userDetail.actions.activationSent')))
            }
          >
            <MailIcon />
            {t('userDetail.actions.resendActivation')}
          </Button>
        )}
        {status === 'ACTIVE' && (
          <Button
            variant="outline"
            className="h-auto min-h-9 justify-start text-left whitespace-normal"
            loading={resetState.isLoading}
            onClick={() =>
              void sendReset({ id: userId }).then((r) => notify(r.error, t('userDetail.actions.resetSent')))
            }
          >
            <KeyRoundIcon />
            {t('userDetail.actions.sendReset')}
          </Button>
        )}
        {mfaEnabled && (
          <Button
            variant="outline"
            className="h-auto min-h-9 justify-start text-left whitespace-normal text-destructive"
            onClick={onResetMfa}
          >
            <ShieldOffIcon />
            {t('userDetail.actions.resetMfa')}
          </Button>
        )}
      </CardContent>
    </Card>
  )
}

const stateVariants = { ACTIVE: 'success', UPCOMING: 'warning', ENDED: 'muted' } as const

function RolesCard({
  userId,
  canAssign,
  onAssign,
  onEnd,
}: {
  userId: string
  canAssign: boolean
  onAssign: () => void
  onEnd: (a: Assignment) => void
}) {
  const { t } = useTranslation()
  const scopeLabel = useScopeLabel()
  const roleName = useRoleName()
  const { data, error, refetch } = useListUserRolesQuery({ id: userId })

  return (
    <Card className="lg:col-span-3">
      <CardHeader>
        <CardTitle>{t('userDetail.roles.title')}</CardTitle>
        <CardDescription>{t('userDetail.roles.description')}</CardDescription>
        {canAssign && (
          <CardAction>
            <Button size="sm" onClick={onAssign}>
              <PlusIcon />
              {t('userDetail.roles.assign')}
            </Button>
          </CardAction>
        )}
      </CardHeader>
      <CardContent>
        {error ? (
          <ErrorState error={error} onRetry={() => void refetch()} />
        ) : !data ? (
          <Skeleton className="h-24" />
        ) : data.items.length === 0 ? (
          <EmptyState>{t('userDetail.roles.empty')}</EmptyState>
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('userDetail.roles.role')}</TableHead>
                <TableHead>{t('userDetail.roles.scope')}</TableHead>
                <TableHead>{t('userDetail.roles.period')}</TableHead>
                <TableHead>{t('userDetail.roles.state')}</TableHead>
                <TableHead>{t('common.reason')}</TableHead>
                {canAssign && <TableHead className="sr-only">{t('common.actions')}</TableHead>}
              </TableRow>
            </TableHeader>
            <TableBody>
              {data.items.map((a) => (
                <TableRow key={a.id}>
                  <TableCell className="font-medium">{roleName(a)}</TableCell>
                  <TableCell>{scopeLabel(a)}</TableCell>
                  <TableCell className="text-xs text-muted-foreground">
                    {formatDateTime(a.valid_from)} →{' '}
                    {a.valid_until ? formatDateTime(a.valid_until) : t('userDetail.roles.openEnded')}
                  </TableCell>
                  <TableCell>
                    <Badge variant={stateVariants[a.state]}>{t(`userDetail.roles.states.${a.state}`)}</Badge>
                  </TableCell>
                  <TableCell className="max-w-56 truncate text-muted-foreground" title={a.reason ?? ''}>
                    {a.reason ?? '—'}
                  </TableCell>
                  {canAssign && (
                    <TableCell className="text-right">
                      {a.state !== 'ENDED' && (
                        <Button variant="ghost" size="sm" onClick={() => onEnd(a)}>
                          {t('userDetail.roles.end')}
                        </Button>
                      )}
                    </TableCell>
                  )}
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </CardContent>
    </Card>
  )
}
