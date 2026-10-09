import { useTranslation } from 'react-i18next'

import type { ScheduleEntry } from '@/shared/api/generated'
import { useLocalized } from '@/shared/lib/localized'
import { Badge } from '@/shared/ui/badge'
import { EmptyState } from '@/shared/ui/states'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/shared/ui/table'

import { day, instructorNames, minutes, roomLabel } from './layout'

/**
 * WeeklyAgenda, haftalık programı gün gün tablo olarak listeler. Bir bölümün bütün
 * dersleri gibi aynı saatte çok sayıda oturum olduğunda ızgaradan daha okunurdur.
 */
export function WeeklyAgenda({ entries }: { entries: ScheduleEntry[] }) {
  const { t } = useTranslation()
  const localized = useLocalized()
  if (entries.length === 0) {
    return <EmptyState>{t('schedule.empty')}</EmptyState>
  }
  const days = [...new Set(entries.map((e) => e.day_of_week))].sort()
  return (
    <div className="space-y-6">
      {days.map((d) => (
        <section key={d} aria-label={t(`schedule.days.${day(d)}`)}>
          <h3 className="mb-2 text-sm font-medium">{t(`schedule.days.${day(d)}`)}</h3>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="w-28">{t('schedule.agenda.time')}</TableHead>
                <TableHead>{t('schedule.agenda.course')}</TableHead>
                <TableHead className="hidden sm:table-cell">{t('schedule.classroom')}</TableHead>
                <TableHead className="hidden md:table-cell">{t('schedule.agenda.instructor')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {entries
                .filter((e) => e.day_of_week === d)
                .sort(
                  (a, b) =>
                    minutes(a.start_time) - minutes(b.start_time) ||
                    a.course.code.localeCompare(b.course.code),
                )
                .map((e) => (
                  <TableRow key={e.id}>
                    <TableCell className="font-mono text-xs whitespace-nowrap">
                      {e.start_time}–{e.end_time}
                    </TableCell>
                    <TableCell>
                      <span className="font-mono text-xs text-muted-foreground">
                        {e.course.code}-{e.section_code}
                      </span>{' '}
                      {localized(e.course)}
                      {e.session_type !== 'THEORY' && (
                        <Badge variant="outline" className="ml-2">
                          {t(`schedule.sessions.${e.session_type}`)}
                        </Badge>
                      )}
                    </TableCell>
                    <TableCell className="hidden sm:table-cell">
                      {roomLabel(e) ?? t('schedule.online')}
                    </TableCell>
                    <TableCell className="hidden text-muted-foreground md:table-cell">
                      {instructorNames(e) || '—'}
                    </TableCell>
                  </TableRow>
                ))}
            </TableBody>
          </Table>
        </section>
      ))}
    </div>
  )
}
