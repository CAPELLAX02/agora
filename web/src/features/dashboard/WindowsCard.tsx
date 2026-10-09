import { ArrowRightIcon, CalendarClockIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'

import { formatDateTime } from '@/i18n/format'
import { useGetCalendarWindowsQuery, useListMyProgramsQuery } from '@/shared/api/generated'
import { useLocalized } from '@/shared/lib/localized'
import { Badge } from '@/shared/ui/badge'
import { Button } from '@/shared/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/shared/ui/card'
import { Skeleton } from '@/shared/ui/skeleton'

/**
 * WindowsCard, şu an açık olan ve yaklaşan işlem pencereleridir (ders seçme, not girişi ...).
 * Öğrencinin ilk program kaydının (anadal) takvimi, personel için üniversite takvimi uygulanır.
 */
export function WindowsCard() {
  const { t } = useTranslation()
  const localized = useLocalized()
  const programs = useListMyProgramsQuery()
  const programId = programs.data?.items[0]?.program.id
  const { data, isError } = useGetCalendarWindowsQuery(programId ? { programId } : {}, {
    skip: programs.isLoading,
  })

  if (isError) {
    return null // aktif dönem tanımlı değilse kart gösterilmez
  }
  const actions = data?.items.filter((w) => w.type.is_action_window) ?? []
  const open = actions.filter((w) => w.open)
  const upcoming = actions
    .filter((w) => !w.open && w.next)
    .sort((a, b) => (a.next?.starts_at ?? '').localeCompare(b.next?.starts_at ?? ''))
    .slice(0, 3)

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <CalendarClockIcon className="size-4 text-primary" aria-hidden />
          {t('dashboard.windows.title')}
        </CardTitle>
        {data && <CardDescription>{t('dashboard.windows.term', { term: data.term.code })}</CardDescription>}
      </CardHeader>
      <CardContent className="space-y-4">
        {!data ? (
          <Skeleton className="h-24" />
        ) : (
          <>
            {open.length === 0 ? (
              <p className="text-sm text-muted-foreground">{t('dashboard.windows.none')}</p>
            ) : (
              <ul className="space-y-2" aria-label={t('dashboard.windows.open')}>
                {open.map((w) => (
                  <li key={w.type.code} className="flex items-start justify-between gap-3">
                    <span className="text-sm font-medium">{localized(w.type)}</span>
                    <Badge variant="success">
                      {t('dashboard.windows.until', {
                        time: formatDateTime(new Date(new Date(w.current?.ends_at ?? '').getTime() - 60_000)),
                      })}
                    </Badge>
                  </li>
                ))}
              </ul>
            )}
            {upcoming.length > 0 && (
              <ul className="space-y-1 border-t pt-3" aria-label={t('dashboard.windows.upcoming')}>
                {upcoming.map((w) => (
                  <li key={w.type.code} className="flex justify-between gap-3 text-sm text-muted-foreground">
                    <span>{localized(w.type)}</span>
                    <span className="tabular-nums">{formatDateTime(w.next?.starts_at)}</span>
                  </li>
                ))}
              </ul>
            )}
            <Button variant="outline" size="sm" asChild>
              <Link to="/takvim">
                {t('dashboard.windows.calendar')}
                <ArrowRightIcon />
              </Link>
            </Button>
          </>
        )}
      </CardContent>
    </Card>
  )
}
