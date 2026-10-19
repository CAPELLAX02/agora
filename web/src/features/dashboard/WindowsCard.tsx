import { ArrowRightIcon, CalendarClockIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'

import { formatDateTime } from '@/i18n/format'
import { useGetCalendarWindowsQuery, useListMyProgramsQuery } from '@/shared/api/generated'
import { useLocalized } from '@/shared/lib/localized'
import { Button } from '@/shared/ui/button'
import { Skeleton } from '@/shared/ui/skeleton'

import { DashCard } from './DashCard'

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
    <DashCard
      icon={CalendarClockIcon}
      tone="sky"
      title={t('dashboard.windows.title')}
      description={data ? t('dashboard.windows.term', { term: data.term.code }) : undefined}
    >
      {!data ? (
        <Skeleton className="h-24" />
      ) : (
        <>
          {open.length === 0 ? (
            <p className="text-sm text-muted-foreground">{t('dashboard.windows.none')}</p>
          ) : (
            <ul className="space-y-1.5" aria-label={t('dashboard.windows.open')}>
              {open.map((w) => (
                <li
                  key={w.type.code}
                  className="flex flex-wrap items-center gap-x-3 gap-y-0.5 rounded-lg bg-emerald-500/8 px-3 py-2 dark:bg-emerald-400/10"
                >
                  <span aria-hidden className="relative flex size-2.5 shrink-0">
                    <span className="absolute inline-flex size-full animate-ping rounded-full bg-emerald-400 opacity-60" />
                    <span className="relative inline-flex size-2.5 rounded-full bg-emerald-500" />
                  </span>
                  {/* Yer yetmezse tarih alt satıra geçer: ad kelime kelime bölünmez. */}
                  <span className="min-w-40 flex-1 text-sm font-semibold">{localized(w.type)}</span>
                  <span className="ml-auto text-xs font-medium text-emerald-700 dark:text-emerald-300">
                    {t('dashboard.windows.until', {
                      time: formatDateTime(new Date(new Date(w.current?.ends_at ?? '').getTime() - 60_000)),
                    })}
                  </span>
                </li>
              ))}
            </ul>
          )}
          {upcoming.length > 0 && (
            <ul className="space-y-1.5" aria-label={t('dashboard.windows.upcoming')}>
              {upcoming.map((w) => (
                <li
                  key={w.type.code}
                  className="flex flex-wrap items-center gap-x-3 gap-y-0.5 px-3 text-sm text-muted-foreground"
                >
                  <span aria-hidden className="size-2 shrink-0 rounded-full bg-sky-400/70" />
                  <span className="min-w-40 flex-1">{localized(w.type)}</span>
                  <span className="ml-auto tabular-nums">{formatDateTime(w.next?.starts_at)}</span>
                </li>
              ))}
            </ul>
          )}
          <Button variant="soft" size="sm" className="mt-auto w-fit" asChild>
            <Link to="/takvim">
              {t('dashboard.windows.calendar')}
              <ArrowRightIcon />
            </Link>
          </Button>
        </>
      )}
    </DashCard>
  )
}
