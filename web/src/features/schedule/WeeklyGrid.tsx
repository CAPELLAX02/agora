import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'

import type { ScheduleEntry } from '@/shared/api/generated'
import { useLocalized } from '@/shared/lib/localized'
import { cn } from '@/shared/lib/utils'
import { EmptyState } from '@/shared/ui/states'

import { day, minutes, placeEntries, roomLabel } from './layout'

const HOUR_PX = 56

const tones = {
  THEORY: 'border-primary/40 bg-primary/10',
  PRACTICE: 'border-warning/50 bg-warning/15',
  LAB: 'border-success/40 bg-success/10',
} as const

/**
 * WeeklyGrid, haftalık programı gün sütunları ve saat satırlarıyla çizer. Aynı saatte
 * birden fazla oturum varsa (ör. bölümün seçmelileri) yan yana şeritlere bölünür. Dar
 * ekranda güne göre liste gösterilir. detail, blokta dersin altında gösterilecek bilgidir
 * (derslik görünümünde öğretim elemanı, öğretim elemanı görünümünde derslik ...).
 */
export function WeeklyGrid({
  entries,
  detail,
  linkTo,
}: {
  entries: ScheduleEntry[]
  detail: (e: ScheduleEntry) => string
  linkTo?: (e: ScheduleEntry) => string
}) {
  const { t } = useTranslation()
  if (entries.length === 0) {
    return <EmptyState>{t('schedule.empty')}</EmptyState>
  }
  const days = [1, 2, 3, 4, 5, ...[6, 7].filter((d) => entries.some((e) => e.day_of_week === d))]
  const first = Math.min(8, ...entries.map((e) => Math.floor(minutes(e.start_time) / 60)))
  const last = Math.max(18, ...entries.map((e) => Math.ceil(minutes(e.end_time) / 60)))
  const hours = Array.from({ length: last - first }, (_, i) => first + i)
  const placed = placeEntries(entries)

  return (
    <>
      <div className="hidden overflow-x-auto md:block">
        <div
          className="grid min-w-[44rem]"
          style={{ gridTemplateColumns: `3.5rem repeat(${days.length}, minmax(0, 1fr))` }}
          role="table"
          aria-label={t('schedule.weekly')}
        >
          <div role="row" className="contents">
            <div role="columnheader" />
            {days.map((d) => (
              <div key={d} role="columnheader" className="border-b px-2 pb-2 text-sm font-medium">
                {t(`schedule.days.${day(d)}`)}
              </div>
            ))}
          </div>
          <div role="row" className="contents">
            <div role="rowheader" className="relative" style={{ height: hours.length * HOUR_PX }}>
              {hours.map((h, i) => (
                <span
                  key={h}
                  className="absolute right-2 -translate-y-1/2 text-xs text-muted-foreground tabular-nums"
                  style={{ top: i * HOUR_PX }}
                >
                  {String(h).padStart(2, '0')}:00
                </span>
              ))}
            </div>
            {days.map((d) => (
              <div
                key={d}
                role="cell"
                aria-label={t(`schedule.days.${day(d)}`)}
                className="relative border-l"
                style={{
                  height: hours.length * HOUR_PX,
                  backgroundImage: `repeating-linear-gradient(to bottom, var(--color-border) 0 1px, transparent 1px ${HOUR_PX}px)`,
                }}
              >
                {placed
                  .filter((p) => p.entry.day_of_week === d)
                  .map((p) => (
                    <Block
                      key={p.entry.id}
                      entry={p.entry}
                      detail={detail(p.entry)}
                      href={linkTo?.(p.entry)}
                      style={{
                        top: ((minutes(p.entry.start_time) - first * 60) / 60) * HOUR_PX,
                        height:
                          ((minutes(p.entry.end_time) - minutes(p.entry.start_time)) / 60) * HOUR_PX - 2,
                        left: `calc(${(p.lane / p.lanes) * 100}% + 2px)`,
                        width: `calc(${100 / p.lanes}% - 4px)`,
                      }}
                    />
                  ))}
              </div>
            ))}
          </div>
        </div>
      </div>
      <ol className="space-y-4 md:hidden">
        {days
          .filter((d) => entries.some((e) => e.day_of_week === d))
          .map((d) => (
            <li key={d}>
              <h3 className="mb-2 text-sm font-medium">{t(`schedule.days.${day(d)}`)}</h3>
              <ul className="space-y-2">
                {entries
                  .filter((e) => e.day_of_week === d)
                  .map((e) => (
                    <li key={e.id}>
                      <Block entry={e} detail={detail(e)} href={linkTo?.(e)} />
                    </li>
                  ))}
              </ul>
            </li>
          ))}
      </ol>
    </>
  )
}

function Block({
  entry: e,
  detail,
  href,
  style,
}: {
  entry: ScheduleEntry
  detail: string
  href?: string | undefined
  style?: React.CSSProperties
}) {
  const { t } = useTranslation()
  const localized = useLocalized()
  const time = `${e.start_time}–${e.end_time}`
  const label = `${e.course.code}-${e.section_code} ${localized(e.course)}, ${t(`schedule.days.${day(e.day_of_week)}`)} ${time}, ${roomLabel(e) ?? t('schedule.online')}`
  const body = (
    <>
      <p className="truncate font-medium">
        {e.course.code}-{e.section_code}
        {e.session_type !== 'THEORY' && (
          <span className="ml-1 font-normal text-muted-foreground">
            · {t(`schedule.sessions.${e.session_type}`)}
          </span>
        )}
      </p>
      <p className="truncate text-muted-foreground">{localized(e.course)}</p>
      <p className="truncate tabular-nums">{time}</p>
      {detail && <p className="truncate text-muted-foreground">{detail}</p>}
    </>
  )
  const className = cn(
    'block overflow-hidden rounded-md border px-2 py-1 text-xs leading-snug',
    style && 'absolute',
    tones[e.session_type],
    href &&
      'transition-shadow hover:shadow-md focus-visible:ring-[3px] focus-visible:ring-ring/50 focus-visible:outline-none',
  )
  return href ? (
    <Link to={href} className={className} style={style} aria-label={label}>
      {body}
    </Link>
  ) : (
    <article className={className} style={style} aria-label={label}>
      {body}
    </article>
  )
}
