import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'

import { TermSelect } from '@/features/calendar/TermSelect'
import { useTermParam } from '@/features/calendar/useTermParam'
import { useGetMeQuery, useMyTeachingQuery, type ScheduleEntry } from '@/shared/api/generated'
import { useLocalized } from '@/shared/lib/localized'
import { Badge } from '@/shared/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/shared/ui/card'
import { PageHeader } from '@/shared/ui/page-header'
import { Skeleton } from '@/shared/ui/skeleton'
import { EmptyState, ErrorState } from '@/shared/ui/states'

import { day, roomLabel } from './layout'
import { WeeklyGrid } from './WeeklyGrid'

/**
 * MyTeachingPage, öğretim elemanının dönemdeki haftalık programı ve verdiği şubelerdir.
 * Şubeden değerlendirme planına geçilir.
 */
export function MyTeachingPage() {
  const { t } = useTranslation()
  const localized = useLocalized()
  const [termId, setTermId] = useTermParam()
  const { data, error, refetch } = useMyTeachingQuery(termId ? { termId } : {}, { skip: !termId })
  // Personelin kullanıcı adı sicil numarasıdır: şubedeki rolü buradan bulunur.
  const { data: me } = useGetMeQuery()

  const sections = new Map<string, ScheduleEntry>()
  for (const e of data?.items ?? []) {
    if (!sections.has(e.section_id)) {
      sections.set(e.section_id, e)
    }
  }

  return (
    <>
      <PageHeader
        title={t('teaching.title')}
        description={t('teaching.description')}
        actions={<TermSelect value={termId} onChange={setTermId} />}
      />
      {error ? (
        <ErrorState error={error} onRetry={() => void refetch()} />
      ) : !data ? (
        <Skeleton className="h-96" />
      ) : (
        <div className="space-y-6">
          <Card>
            <CardHeader>
              <CardTitle>{t('teaching.sections')}</CardTitle>
            </CardHeader>
            <CardContent>
              {sections.size === 0 ? (
                <EmptyState>{t('teaching.empty')}</EmptyState>
              ) : (
                <ul className="divide-y">
                  {[...sections.values()].map((e) => {
                    const mine = e.instructors.find((i) => i.staff_no === me?.username)
                    return (
                      <li
                        key={e.section_id}
                        className="flex flex-wrap items-center justify-between gap-3 py-3"
                      >
                        <div>
                          <p className="text-sm font-medium">
                            <span className="font-mono text-xs text-muted-foreground">
                              {e.course.code}-{e.section_code}
                            </span>{' '}
                            {localized(e.course)}
                          </p>
                          <p className="text-xs text-muted-foreground">
                            {data.items
                              .filter((x) => x.section_id === e.section_id)
                              .map(
                                (x) =>
                                  `${t(`schedule.daysShort.${day(x.day_of_week)}`)} ${x.start_time}–${x.end_time} ${roomLabel(x) ?? ''}`,
                              )
                              .join(' · ')}
                          </p>
                        </div>
                        <div className="flex items-center gap-2">
                          {mine && <Badge variant="outline">{t(`teaching.roles.${mine.role}`)}</Badge>}
                          <Link
                            to={`/subeler/${e.section_id}/degerlendirme`}
                            className="text-sm font-medium text-primary hover:underline"
                          >
                            {t('teaching.plan')}
                          </Link>
                        </div>
                      </li>
                    )
                  })}
                </ul>
              )}
            </CardContent>
          </Card>
          <Card>
            <CardHeader>
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
