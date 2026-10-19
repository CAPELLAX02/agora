import {
  CalendarClockIcon,
  CalendarRangeIcon,
  DoorOpenIcon,
  PencilIcon,
  PlusIcon,
  Trash2Icon,
} from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useSearchParams } from 'react-router'
import { toast } from 'sonner'

import { usePermissions } from '@/features/auth/usePermissions'
import { navTheme } from '@/features/shell/nav'
import { formatDate, formatDateTime, formatPeriod } from '@/i18n/format'
import {
  useDeleteCalendarEventMutation,
  useGetCalendarWindowsQuery,
  useListFacultiesQuery,
  useListMyProgramsQuery,
  useListTermEventsQuery,
  useListTermsQuery,
  type CalendarEvent,
  type CalendarWindow,
} from '@/shared/api/generated'
import { useErrorMessage } from '@/shared/api/useErrorMessage'
import { useLocalized } from '@/shared/lib/localized'
import { cn } from '@/shared/lib/utils'
import { ConfirmDialog } from '@/shared/ui/confirm-dialog'
import { Badge } from '@/shared/ui/badge'
import { Button } from '@/shared/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/shared/ui/card'
import { IconChip } from '@/shared/ui/icon-chip'
import { PageHeader } from '@/shared/ui/page-header'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/shared/ui/select'
import { Skeleton } from '@/shared/ui/skeleton'
import { EmptyState, ErrorState } from '@/shared/ui/states'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/shared/ui/table'
import { toneFill } from '@/shared/ui/tone'

import { eventCategory } from './categories'
import { EventDialog } from './EventDialog'
import { TermSelect } from './TermSelect'
import { useTermParam } from './useTermParam'

const UNIVERSITY = 'UNIVERSITY'
const PROGRAM = 'PROGRAM'

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
  const faculties = useListFacultiesQuery({})
  const localized = useLocalized()
  const [editing, setEditing] = useState<CalendarEvent | 'new' | null>(null)

  const [termId, setTermId] = useTermParam()
  const term = terms.data?.items.find((x) => x.id === termId)
  const canManage = has('calendar:manage')
  // Öğrenci varsayılan olarak kendi programının takvimini görür: üniversitenin kapalı
  // penceresi, programa açılmış ders seçmeyi gizlemesin.
  const programs = useListMyProgramsQuery(undefined, { skip: !has('registration:manage_own') })
  const myProgram = programs.data?.items[0]?.program
  const scope =
    params.get('birim') ?? (params.get('kapsam') === 'universite' || !myProgram ? UNIVERSITY : PROGRAM)
  const target =
    scope === PROGRAM && myProgram
      ? { programId: myProgram.id }
      : scope !== UNIVERSITY && scope !== PROGRAM
        ? { facultyId: scope }
        : {}

  const setScope = (value: string) => {
    const next = new URLSearchParams(params)
    next.delete('birim')
    next.delete('kapsam')
    if (value === UNIVERSITY && myProgram) {
      next.set('kapsam', 'universite')
    } else if (value !== UNIVERSITY && value !== PROGRAM) {
      next.set('birim', value)
    }
    setParams(next, { replace: true })
  }

  if (terms.error) {
    return <ErrorState error={terms.error} onRetry={() => void terms.refetch()} />
  }

  return (
    <>
      <PageHeader
        {...navTheme('/takvim')}
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
      <div className="mb-6 flex flex-col gap-3 sm:flex-row sm:flex-wrap">
        <TermSelect value={termId} onChange={setTermId} />
        <Select value={scope} onValueChange={setScope}>
          <SelectTrigger className="sm:min-w-80" aria-label={t('calendar.scope')}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {myProgram && (
              <SelectItem value={PROGRAM}>{t('calendar.myProgram', { name: myProgram.name_tr })}</SelectItem>
            )}
            <SelectItem value={UNIVERSITY}>{t('calendar.wholeUniversity')}</SelectItem>
            {faculties.data?.items.map((f) => (
              <SelectItem key={f.id} value={f.id}>
                {localized(f)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      {!terms.data ? (
        <div className="space-y-4">
          <Skeleton className="h-40" />
          <Skeleton className="h-64" />
        </div>
      ) : !termId ? (
        <EmptyState>{t('calendar.noTerms')}</EmptyState>
      ) : (
        <div className="space-y-6">
          {term && (
            <p className="inline-flex items-center gap-2 rounded-md bg-sky-500/10 px-3 py-1 text-sm font-semibold text-sky-700 dark:text-sky-300">
              <CalendarRangeIcon className="size-4" aria-hidden />
              {t('calendar.termDates', { start: formatDate(term.starts_on), end: formatDate(term.ends_on) })}
            </p>
          )}
          <Windows termId={termId} target={target} />
          <Events termId={termId} target={target} canManage={canManage} onEdit={(e) => setEditing(e)} />
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

type Target = { facultyId?: string; programId?: string }

function Windows({ termId, target }: { termId: string; target: Target }) {
  const { t } = useTranslation()
  const { data, error, refetch } = useGetCalendarWindowsQuery({ termId, ...target })

  return (
    <Card>
      <CardHeader className="flex items-center gap-3">
        <IconChip icon={DoorOpenIcon} tone="emerald" />
        <div className="space-y-0.5">
          <CardTitle>{t('calendar.windows.title')}</CardTitle>
          <CardDescription>{t('calendar.windows.description')}</CardDescription>
        </div>
      </CardHeader>
      <CardContent>
        {error ? (
          <ErrorState error={error} onRetry={() => void refetch()} />
        ) : !data ? (
          <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
            {Array.from({ length: 6 }, (_, i) => (
              <Skeleton key={i} className="h-20" />
            ))}
          </div>
        ) : (
          <ul className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
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
    <li
      className={cn(
        'flex gap-3 rounded-xl border p-3 transition-shadow',
        w.open
          ? 'border-emerald-400/50 bg-emerald-500/[0.05] shadow-xs ring-1 ring-emerald-400/20'
          : 'border-border/70 bg-card',
      )}
      aria-label={localized(w.type)}
    >
      <IconChip icon={eventCategory[w.type.category].icon} tone={w.open ? 'emerald' : 'slate'} size="sm" />
      <div className="min-w-0 flex-1">
        <div className="flex items-start justify-between gap-2">
          <p className="text-sm font-semibold">{localized(w.type)}</p>
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
          <Badge variant="violet" className="mt-2">
            {t(`calendar.scopes.${w.scope_type}`)}
          </Badge>
        )}
      </div>
    </li>
  )
}

function Events({
  termId,
  target,
  canManage,
  onEdit,
}: {
  termId: string
  target: Target
  canManage: boolean
  onEdit: (e: CalendarEvent) => void
}) {
  const { t, i18n } = useTranslation()
  const localized = useLocalized()
  const message = useErrorMessage()
  const { data, error, refetch } = useListTermEventsQuery({ id: termId, ...target })
  const [remove] = useDeleteCalendarEventMutation()
  const [deleting, setDeleting] = useState<CalendarEvent | null>(null)

  const title = (e: CalendarEvent) =>
    (i18n.language === 'en' ? e.title_en : e.title_tr) ?? e.title_tr ?? localized(e.type)
  const events = [...(data?.items ?? [])].sort((a, b) => a.starts_at.localeCompare(b.starts_at))

  return (
    <Card className="gap-0 py-0">
      <CardHeader className="flex items-center gap-3 border-b border-border/60 py-4">
        <IconChip icon={CalendarClockIcon} tone="sky" />
        <CardTitle>{t('calendar.events.title')}</CardTitle>
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
                <TableCell className="min-w-32 text-sm font-medium whitespace-normal tabular-nums">
                  {formatPeriod(e.starts_at, e.ends_at)}
                </TableCell>
                <TableCell className="whitespace-normal">
                  <div className="flex flex-wrap items-center gap-2">
                    <span
                      aria-hidden
                      className={cn(
                        'size-2.5 shrink-0 rounded-full',
                        toneFill[eventCategory[e.type.category].tone],
                      )}
                    />
                    <span className="font-semibold">{title(e)}</span>
                    {!e.is_published && <Badge variant="warning">{t('calendar.events.draft')}</Badge>}
                  </div>
                  {e.note && <p className="mt-0.5 pl-[18px] text-xs text-muted-foreground">{e.note}</p>}
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
