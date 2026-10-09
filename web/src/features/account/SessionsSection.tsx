import { LaptopIcon, MonitorSmartphoneIcon, SmartphoneIcon, TabletIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { formatDateTime, formatRelative } from '@/i18n/format'
import {
  useEndOtherSessionsMutation,
  useEndSessionMutation,
  useListMySessionsQuery,
  type Session,
} from '@/shared/api/generated'
import { useErrorMessage } from '@/shared/api/useErrorMessage'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from '@/shared/ui/alert-dialog'
import { Badge } from '@/shared/ui/badge'
import { Button } from '@/shared/ui/button'
import { Card, CardAction, CardContent, CardDescription, CardHeader, CardTitle } from '@/shared/ui/card'
import { Skeleton } from '@/shared/ui/skeleton'
import { ErrorState } from '@/shared/ui/states'

const deviceIcons = {
  DESKTOP: LaptopIcon,
  MOBILE: SmartphoneIcon,
  TABLET: TabletIcon,
  UNKNOWN: MonitorSmartphoneIcon,
}

export function SessionsSection() {
  const { t } = useTranslation()
  const message = useErrorMessage()
  const { data, error, refetch } = useListMySessionsQuery()
  const [endSession, endState] = useEndSessionMutation()
  const [endOthers, endOthersState] = useEndOtherSessionsMutation()

  const others = data?.items.filter((s) => !s.current).length ?? 0

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('sessions.title')}</CardTitle>
        <CardDescription>{t('sessions.description')}</CardDescription>
        {others > 0 && (
          <CardAction>
            <AlertDialog>
              <AlertDialogTrigger asChild>
                <Button variant="outline" size="sm" loading={endOthersState.isLoading}>
                  {t('sessions.endOthers')}
                </Button>
              </AlertDialogTrigger>
              <AlertDialogContent>
                <AlertDialogHeader>
                  <AlertDialogTitle>{t('sessions.endOthers')}</AlertDialogTitle>
                  <AlertDialogDescription>{t('sessions.endOthersConfirm')}</AlertDialogDescription>
                </AlertDialogHeader>
                <AlertDialogFooter>
                  <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
                  <AlertDialogAction
                    variant="destructive"
                    onClick={() =>
                      void endOthers().then((res) =>
                        res.data
                          ? toast.success(t('sessions.endedOthers', { count: res.data.revoked }))
                          : toast.error(message(res.error)),
                      )
                    }
                  >
                    {t('sessions.endOthers')}
                  </AlertDialogAction>
                </AlertDialogFooter>
              </AlertDialogContent>
            </AlertDialog>
          </CardAction>
        )}
      </CardHeader>
      <CardContent>
        {error ? (
          <ErrorState error={error} onRetry={() => void refetch()} />
        ) : !data ? (
          <Skeleton className="h-32" />
        ) : (
          <ul className="divide-y">
            {data.items.map((s) => (
              <SessionRow
                key={s.id}
                session={s}
                ending={endState.isLoading && endState.originalArgs?.id === s.id}
                onEnd={() =>
                  void endSession({ id: s.id }).then((res) =>
                    res.error ? toast.error(message(res.error)) : toast.success(t('sessions.ended')),
                  )
                }
              />
            ))}
          </ul>
        )}
        {data && others === 0 && <p className="pt-2 text-sm text-muted-foreground">{t('sessions.empty')}</p>}
      </CardContent>
    </Card>
  )
}

function SessionRow({ session: s, ending, onEnd }: { session: Session; ending: boolean; onEnd: () => void }) {
  const { t } = useTranslation()
  const Icon = deviceIcons[s.device]
  const name = [s.browser ?? t('sessions.unknownBrowser'), s.os].filter(Boolean).join(' · ')
  return (
    <li className="flex flex-wrap items-center gap-4 py-3">
      <span className="flex size-10 items-center justify-center rounded-lg bg-muted">
        <Icon className="size-5 text-muted-foreground" aria-hidden />
      </span>
      <div className="min-w-0 flex-1 space-y-0.5">
        <p className="flex flex-wrap items-center gap-2 text-sm font-medium">
          {name}
          <Badge variant="outline">{t(`sessions.client.${s.client}`)}</Badge>
          {s.current && <Badge variant="success">{t('common.current')}</Badge>}
        </p>
        <p className="text-xs text-muted-foreground">
          <span title={formatDateTime(s.last_seen_at)}>
            {t('sessions.lastSeen', { time: formatRelative(s.last_seen_at) })}
          </span>
          {' · '}
          {t('sessions.started', { time: formatDateTime(s.created_at) })}
          {s.ip && <> · {t('sessions.ip', { ip: s.ip })}</>}
        </p>
      </div>
      {!s.current && (
        <Button variant="ghost" size="sm" onClick={onEnd} loading={ending}>
          {t('sessions.end')}
        </Button>
      )}
    </li>
  )
}
