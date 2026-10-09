import { CalendarClockIcon, PencilIcon, PlusIcon, Trash2Icon } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useSearchParams } from 'react-router'
import { toast } from 'sonner'

import { usePermissions } from '@/features/auth/usePermissions'
import { formatDate, formatDateTime, formatPeriod } from '@/i18n/format'
import {
  useDeleteCalendarEventMutation,
  useGetCalendarWindowsQuery,
  useGetCurrentTermQuery,
  useListFacultiesQuery,
  useListTermEventsQuery,
  useListTermsQuery,
  type CalendarEvent,
  type CalendarWindow,
} from '@/shared/api/generated'
import { useErrorMessage } from '@/shared/api/useErrorMessage'
import { useLocalized } from '@/shared/lib/localized'
import { ConfirmDialog } from '@/shared/ui/confirm-dialog'
import { Badge } from '@/shared/ui/badge'
import { Button } from '@/shared/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/shared/ui/card'
import { PageHeader } from '@/shared/ui/page-header'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/shared/ui/select'
import { Skeleton } from '@/shared/ui/skeleton'
import { EmptyState, ErrorState } from '@/shared/ui/states'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/shared/ui/table'

import { EventDialog } from './EventDialog'
import { termLabel } from './term'

const UNIVERSITY = 'UNIVERSITY'

/**
 * CalendarPage, bir dönemin işlem pencerelerini (ders seçme açık mı?) ve takvim olaylarını
 * gösterir. Birim seçilince o birimin kendi tanımladığı pencereler üniversitenin
 * pencerelerinin yerine geçer. Takvim yöneticileri olay ekler, düzenler ve siler.
 */
export function CalendarPage() {
  const { t } = useTranslation()
  const { has } = usePermissions()
  const [params, setParams] = useSearchParams()
  const terms = useListTermsQuery()
  const current = useGetCurrentTermQuery()
  const faculties = useListFacultiesQuery({})
  const localized = useLocalized()
  const [editing, setEditing] = useState<CalendarEvent | 'new' | null>(null)

  const termId = params.get('donem') ?? current.data?.id ?? terms.data?.items[0]?.id ?? ''
  const facultyId = params.get('birim') ?? ''
  const term = terms.data?.items.find((x) => x.id === termId)
  const canManage = has('calendar:manage')

  const setParam = (key: string, value: string) => {
    const next = new URLSearchParams(params)
    if (value && value !== UNIVERSITY) {
      next.set(key, value)
    } else {
      next.delete(key)
    }
    setParams(next, { replace: true })
  }

  if (terms.error) {
    return <ErrorState error={terms.error} onRetry={() => void terms.refetch()} />
  }

  return (
    <>
      <PageHeader
        title={t('calendar.title')}
        description={t('calendar.description')}
        actions={
          canManage &&
          termId && (
            <Button onClick={() => setEditing('new')}>
              <PlusIcon />
              {t('calendar.newEvent')}
            </Button>
          )
        }
      />
      <div className="mb-6 flex flex-col gap-3 sm:flex-row">
        <Select value={termId} onValueChange={(v) => setParam('donem', v)} disabled={!terms.data}>
          <SelectTrigger className="sm:w-64" aria-label={t('calendar.term')}>
            <SelectValue placeholder={t('common.loading')} />
          </SelectTrigger>
          <SelectContent>
            {terms.data?.items.map((x) => (
              <SelectItem key={x.id} value={x.id}>
                {termLabel(t, x)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Select value={facultyId || UNIVERSITY} onValueChange={(v) => setParam('birim', v)}>
          <SelectTrigger className="sm:w-80" aria-label={t('calendar.scope')}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={UNIVERSITY}>{t('calendar.wholeUniversity')}</SelectItem>
            {faculties.data?.items.map((f) => (
              <SelectItem key={f.id} value={f.id}>
                {localized(f)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      {!terms.data || current.isLoading ? (
        <div className="space-y-4">
          <Skeleton className="h-40" />
          <Skeleton className="h-64" />
        </div>
      ) : !termId ? (
        <EmptyState>{t('calendar.noTerms')}</EmptyState>
      ) : (
        <div className="space-y-6">
          {term && (
            <p className="text-sm text-muted-foreground">
              {t('calendar.termDates', { start: formatDate(term.starts_on), end: formatDate(term.ends_on) })}
            </p>
          )}
          <Windows termId={termId} facultyId={facultyId} />
          <Events termId={termId} facultyId={facultyId} canManage={canManage} onEdit={(e) => setEditing(e)} />
        </div>
      )}
      {editing && (
        <EventDialog
          termId={termId}
          event={editing === 'new' ? null : editing}
          onClose={() => setEditing(null)}
        />
      )}
    </>
  )
}

function Windows({ termId, facultyId }: { termId: string; facultyId: string }) {
  const { t } = useTranslation()
  const { data, error, refetch } = useGetCalendarWindowsQuery({ termId, ...(facultyId && { facultyId }) })

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('calendar.windows.title')}</CardTitle>
        <CardDescription>{t('calendar.windows.description')}</CardDescription>
      </CardHeader>
      <CardContent>
        {error ? (
          <ErrorState error={error} onRetry={() => void refetch()} />
        ) : !data ? (
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {Array.from({ length: 6 }, (_, i) => (
              <Skeleton key={i} className="h-20" />
            ))}
          </div>
        ) : (
          <ul className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {data.items
              .filter((w) => w.type.is_action_window)
              .map((w) => (
                <WindowTile key={w.type.code} window={w} />
              ))}
          </ul>
        )}
      </CardContent>
    </Card>
  )
}

function WindowTile({ window: w }: { window: CalendarWindow }) {
  const { t } = useTranslation()
  const localized = useLocalized()
  return (
    <li className="rounded-md border p-3" aria-label={localized(w.type)}>
      <div className="flex items-start justify-between gap-2">
        <p className="text-sm font-medium">{localized(w.type)}</p>
        <Badge variant={w.open ? 'success' : 'muted'}>
          {t(w.open ? 'calendar.windows.open' : 'calendar.windows.closed')}
        </Badge>
      </div>
      <p className="mt-1 text-xs text-muted-foreground">
        {w.current
          ? t('calendar.windows.until', {
              time: formatDateTime(new Date(new Date(w.current.ends_at).getTime() - 60_000)),
            })
          : w.next
            ? t('calendar.windows.opens', { time: formatDateTime(w.next.starts_at) })
            : w.events.length > 0
              ? t('calendar.windows.ended')
              : t('calendar.windows.undefined')}
      </p>
      {w.scope_type && w.scope_type !== 'UNIVERSITY' && (
        <Badge variant="outline" className="mt-2">
          {t(`calendar.scopes.${w.scope_type}`)}
        </Badge>
      )}
    </li>
  )
}

function Events({
  termId,
  facultyId,
  canManage,
  onEdit,
}: {
  termId: string
  facultyId: string
  canManage: boolean
  onEdit: (e: CalendarEvent) => void
}) {
  const { t, i18n } = useTranslation()
  const localized = useLocalized()
  const message = useErrorMessage()
  const { data, error, refetch } = useListTermEventsQuery({ id: termId, ...(facultyId && { facultyId }) })
  const [remove] = useDeleteCalendarEventMutation()
  const [deleting, setDeleting] = useState<CalendarEvent | null>(null)

  const title = (e: CalendarEvent) =>
    (i18n.language === 'en' ? e.title_en : e.title_tr) ?? e.title_tr ?? localized(e.type)
  const events = [...(data?.items ?? [])].sort((a, b) => a.starts_at.localeCompare(b.starts_at))

  return (
    <Card className="gap-0 py-0">
      <CardHeader className="border-b py-4">
        <CardTitle className="flex items-center gap-2">
          <CalendarClockIcon className="size-4" aria-hidden />
          {t('calendar.events.title')}
        </CardTitle>
      </CardHeader>
      {error ? (
        <CardContent className="py-4">
          <ErrorState error={error} onRetry={() => void refetch()} />
        </CardContent>
      ) : !data ? (
        <CardContent className="space-y-2 py-4">
          <Skeleton className="h-8" />
          <Skeleton className="h-8" />
        </CardContent>
      ) : events.length === 0 ? (
        <EmptyState>{t('calendar.events.empty')}</EmptyState>
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('calendar.events.when')}</TableHead>
              <TableHead>{t('calendar.events.event')}</TableHead>
              <TableHead className="hidden md:table-cell">{t('calendar.events.scope')}</TableHead>
              {canManage && <TableHead className="w-24 text-right">{t('common.actions')}</TableHead>}
            </TableRow>
          </TableHeader>
          <TableBody>
            {events.map((e) => (
              <TableRow key={e.id}>
                <TableCell className="text-sm whitespace-nowrap">
                  {formatPeriod(e.starts_at, e.ends_at)}
                </TableCell>
                <TableCell>
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="font-medium">{title(e)}</span>
                    {!e.is_published && <Badge variant="warning">{t('calendar.events.draft')}</Badge>}
                  </div>
                  {e.note && <p className="text-xs text-muted-foreground">{e.note}</p>}
                </TableCell>
                <TableCell className="hidden text-sm md:table-cell">
                  {e.scope ? e.scope.name : t('calendar.scopes.UNIVERSITY')}
                </TableCell>
                {canManage && (
                  <TableCell className="text-right">
                    <Button
                      variant="ghost"
                      size="icon"
                      onClick={() => onEdit(e)}
                      aria-label={t('calendar.events.edit', { name: title(e) })}
                    >
                      <PencilIcon />
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon"
                      onClick={() => setDeleting(e)}
                      aria-label={t('calendar.events.delete', { name: title(e) })}
                    >
                      <Trash2Icon />
                    </Button>
                  </TableCell>
                )}
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
      {deleting && (
        <ConfirmDialog
          title={t('calendar.events.deleteTitle')}
          description={t('calendar.events.deleteDescription', { name: title(deleting) })}
          confirm={t('calendar.events.deleteConfirm')}
          destructive
          onClose={() => setDeleting(null)}
          onConfirm={async () => {
            const res = await remove({ id: deleting.id })
            if ('error' in res) {
              toast.error(message(res.error))
              return
            }
            toast.success(t('calendar.events.deleted'))
            setDeleting(null)
          }}
        />
      )}
    </Card>
  )
}
