import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { formatDateTime } from '@/i18n/format'
import { useListMySecurityEventsQuery, type SecurityEvent } from '@/shared/api/generated'
import { Badge } from '@/shared/ui/badge'
import { Button } from '@/shared/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/shared/ui/card'
import { Skeleton } from '@/shared/ui/skeleton'
import { EmptyState, ErrorState } from '@/shared/ui/states'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/shared/ui/table'

import { useEventDetail } from './events'

const warning = new Set(['LOGIN_FAILED', 'ACCOUNT_LOCKED', 'REFRESH_TOKEN_REUSED', 'MFA_DISABLED'])

export function HistorySection() {
  const { t } = useTranslation()
  const [cursors, setCursors] = useState<string[]>([''])
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('events.title')}</CardTitle>
        <CardDescription>{t('events.description')}</CardDescription>
      </CardHeader>
      <CardContent>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('events.when')}</TableHead>
              <TableHead>{t('events.event')}</TableHead>
              <TableHead>{t('events.ip')}</TableHead>
              <TableHead>{t('events.details')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {cursors.map((cursor, i) => (
              <EventPage
                key={cursor}
                cursor={cursor}
                last={i === cursors.length - 1}
                onMore={(next) => setCursors((c) => [...c, next])}
              />
            ))}
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  )
}

/** EventPage, imleçli listenin bir sayfasıdır: "daha fazla" yeni bir sayfa ekler. */
function EventPage({
  cursor,
  last,
  onMore,
}: {
  cursor: string
  last: boolean
  onMore: (next: string) => void
}) {
  const { t } = useTranslation()
  const detail = useEventDetail()
  const { data, error, refetch, isFetching } = useListMySecurityEventsQuery({
    limit: 20,
    ...(cursor && { cursor }),
  })

  if (error) {
    return (
      <TableRow>
        <TableCell colSpan={4}>
          <ErrorState error={error} onRetry={() => void refetch()} />
        </TableCell>
      </TableRow>
    )
  }
  if (!data) {
    return (
      <TableRow>
        <TableCell colSpan={4}>
          <Skeleton className="h-24" />
        </TableCell>
      </TableRow>
    )
  }
  if (data.items.length === 0 && cursor === '') {
    return (
      <TableRow>
        <TableCell colSpan={4}>
          <EmptyState>{t('events.empty')}</EmptyState>
        </TableCell>
      </TableRow>
    )
  }
  return (
    <>
      {data.items.map((e: SecurityEvent) => (
        <TableRow key={e.id}>
          <TableCell className="text-muted-foreground">{formatDateTime(e.occurred_at)}</TableCell>
          <TableCell>
            {warning.has(e.type) ? (
              <Badge variant="warning">{t(`events.types.${e.type}`)}</Badge>
            ) : (
              <span className="font-medium">{t(`events.types.${e.type}`)}</span>
            )}
          </TableCell>
          <TableCell className="font-mono text-xs">{e.ip ?? '—'}</TableCell>
          <TableCell className="text-muted-foreground">{detail(e)}</TableCell>
        </TableRow>
      ))}
      {last && data.next_cursor && (
        <TableRow className="hover:bg-transparent">
          <TableCell colSpan={4} className="text-center">
            <Button
              variant="ghost"
              size="sm"
              loading={isFetching}
              onClick={() => onMore(data.next_cursor ?? '')}
            >
              {t('common.loadMore')}
            </Button>
          </TableCell>
        </TableRow>
      )}
    </>
  )
}
