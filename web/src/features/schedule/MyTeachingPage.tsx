import { CalendarRangeIcon, ClipboardListIcon, PresentationIcon, TableIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'

import { TermSelect } from '@/features/calendar/TermSelect'
import { useTermParam } from '@/features/calendar/useTermParam'
import { navTheme } from '@/features/shell/nav'
import { useMySectionsQuery, useMyTeachingQuery } from '@/shared/api/generated'
import { useLocalized } from '@/shared/lib/localized'
import { cn } from '@/shared/lib/utils'
import { Badge } from '@/shared/ui/badge'
import { Button } from '@/shared/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/shared/ui/card'
import { IconChip } from '@/shared/ui/icon-chip'
import { PageHeader } from '@/shared/ui/page-header'
import { Skeleton } from '@/shared/ui/skeleton'
import { EmptyState, ErrorState } from '@/shared/ui/states'
import { toneFill, toneFor, toneSoft } from '@/shared/ui/tone'

import { day, roomLabel } from './layout'
import { WeeklyGrid } from './WeeklyGrid'

/**
 * MyTeachingPage, öğretim elemanının dönemdeki haftalık programı ve verdiği şubelerdir.
 * Şubeden değerlendirme planına ve not tablosuna geçilir.
 */
export function MyTeachingPage() {
  const { t } = useTranslation()
  const localized = useLocalized()
  const [termId, setTermId] = useTermParam()
  const { data, error, refetch } = useMyTeachingQuery(termId ? { termId } : {}, { skip: !termId })
  // Şubeler programdan bağımsız gelir: oturumu olmayan dersler (bitirme projesi ...) de listelenir.
  const mine = useMySectionsQuery(termId ? { termId } : {}, { skip: !termId })
  const sections = mine.data?.items ?? []

  return (
    <>
      <PageHeader
        {...navTheme('/verdigim-dersler')}
        title={t('teaching.title')}
        description={t('teaching.description')}
        actions={<TermSelect value={termId} onChange={setTermId} />}
      />
      {error || mine.error ? (
        <ErrorState error={error ?? mine.error} onRetry={() => void refetch()} />
      ) : !data || !mine.data ? (
        <Skeleton className="h-96" />
      ) : (
        <div className="space-y-6">
          <section aria-labelledby="my-sections" className="space-y-3">
            <h2 id="my-sections" className="text-lg font-bold">
              {t('teaching.sections')}
            </h2>
            {sections.length === 0 ? (
              <Card>
                <EmptyState icon={PresentationIcon} tone="fuchsia">
                  {t('teaching.empty')}
                </EmptyState>
              </Card>
            ) : (
              <ul className="grid gap-3 md:grid-cols-2">
                {sections.map((e) => {
                  const slots = data.items.filter((x) => x.section_id === e.section_id)
                  const tone = toneFor(e.course.code)
                  return (
                    <li
                      key={e.section_id}
                      className="relative flex flex-col gap-3 overflow-hidden rounded-xl border border-border/70 bg-card p-4 pl-5 shadow-soft"
                    >
                      <span aria-hidden className={cn('absolute inset-y-0 left-0 w-1.5', toneFill[tone])} />
                      <div className="space-y-1.5">
                        <p className="flex flex-wrap items-center gap-2 text-sm">
                          <span
                            className={cn(
                              'rounded-md px-1.5 py-0.5 font-mono text-xs font-bold',
                              toneSoft[tone],
                            )}
                          >
                            {e.course.code}-{e.section_code}
                          </span>
                          <span className="font-bold">{localized(e.course)}</span>
                        </p>
                        <p className="text-xs text-muted-foreground">
                          {slots.length === 0
                            ? t('teaching.noSchedule')
                            : slots
                                .map(
                                  (x) =>
                                    `${t(`schedule.daysShort.${day(x.day_of_week)}`)} ${x.start_time}–${x.end_time} ${roomLabel(x) ?? ''}`,
                                )
                                .join(' · ')}
                        </p>
                      </div>
                      <div className="mt-auto flex items-center justify-between gap-2">
                        <span className="flex flex-wrap items-center gap-2">
                          <Badge variant="violet">{t(`teaching.roles.${e.role}`)}</Badge>
                          <span className="text-xs text-muted-foreground">
                            {t('teaching.students', { count: e.students })}
                          </span>
                        </span>
                        <div className="flex flex-wrap justify-end gap-2">
                          <Button variant="ghost" size="sm" asChild>
                            <Link to={`/subeler/${e.section_id}/degerlendirme`}>
                              <ClipboardListIcon />
                              {t('teaching.plan')}
                            </Link>
                          </Button>
                          <Button variant="soft" size="sm" asChild>
                            <Link to={`/subeler/${e.section_id}/notlar`}>
                              <TableIcon />
                              {t('teaching.gradebook')}
                            </Link>
                          </Button>
                        </div>
                      </div>
                    </li>
                  )
                })}
              </ul>
            )}
          </section>
          <Card>
            <CardHeader className="flex items-center gap-3">
              <IconChip icon={CalendarRangeIcon} tone="blue" />
              <CardTitle>{t('schedule.weekly')}</CardTitle>
            </CardHeader>
            <CardContent>
              <WeeklyGrid entries={data.items} detail={(e) => roomLabel(e) ?? t('schedule.online')} />
            </CardContent>
          </Card>
        </div>
      )}
    </>
  )
}
