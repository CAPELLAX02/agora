import { AlarmClockIcon, ArrowRightIcon, HourglassIcon, ListTodoIcon, NotebookPenIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'

import { formatDateTime, formatRelative } from '@/i18n/format'
import { useMyTimelineQuery, type TimelineItem } from '@/shared/api/generated'
import { useLocalized } from '@/shared/lib/localized'
import { cn } from '@/shared/lib/utils'
import { Badge } from '@/shared/ui/badge'
import { IconChip } from '@/shared/ui/icon-chip'
import { Skeleton } from '@/shared/ui/skeleton'

import { DashCard } from './DashCard'

// Kartta en çok bu kadar öğe gösterilir; gerisi ders alanlarından görülür.
const limit = 6

const look = {
  OVERDUE: { icon: AlarmClockIcon, tone: 'rose' },
  DUE: { icon: NotebookPenIcon, tone: 'sky' },
  TO_GRADE: { icon: HourglassIcon, tone: 'amber' },
} as const

/**
 * TimelineCard, kullanıcının yapılacaklarıdır: öğrenciye geciken ve yaklaşan ödevler, öğretim
 * elemanına puanlanmayı bekleyen teslimler; öğeden ödevin sayfasına geçilir.
 */
export function TimelineCard() {
  const { t } = useTranslation()
  const { data, isError } = useMyTimelineQuery({})
  if (isError) {
    return null
  }
  const items = data?.items ?? []
  return (
    <DashCard
      icon={ListTodoIcon}
      tone="rose"
      title={t('dashboard.timeline.title')}
      description={t('dashboard.timeline.description')}
      action={
        <Link to="/derslerim" className="text-sm font-semibold text-primary hover:underline">
          {t('dashboard.timeline.all')}
        </Link>
      }
    >
      {!data ? (
        <Skeleton className="h-28" />
      ) : items.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t('dashboard.timeline.none')}</p>
      ) : (
        <ul className="grid grid-cols-[minmax(0,1fr)] gap-2">
          {items.slice(0, limit).map((it) => (
            <TimelineRow key={`${it.kind}-${it.activity_id}`} item={it} />
          ))}
          {items.length > limit && (
            <li className="text-xs text-muted-foreground">
              {t('dashboard.timeline.more', { count: items.length - limit })}
            </li>
          )}
        </ul>
      )}
    </DashCard>
  )
}

function TimelineRow({ item }: { item: TimelineItem }) {
  const { t } = useTranslation()
  const localized = useLocalized()
  const { icon, tone } = look[item.kind]
  return (
    <li className="min-w-0">
      <Link
        to={`/derslerim/${item.section_id}/etkinlik/${item.activity_id}`}
        className="group flex items-center gap-3 rounded-xl border border-border/70 px-3 py-2.5 text-sm transition-all hover:border-primary/25 hover:bg-primary/[0.03]"
      >
        <IconChip icon={icon} tone={tone} size="sm" />
        <span className="min-w-0 flex-1">
          <span className="block truncate font-semibold">{item.title}</span>
          <span className="block truncate text-xs text-muted-foreground">
            {item.course.code}-{item.section_code} · {localized(item.course)}
          </span>
        </span>
        {item.kind === 'TO_GRADE' ? (
          <Badge variant="warning">{t('dashboard.timeline.toGrade', { count: item.needs_grading })}</Badge>
        ) : (
          <span
            className={cn(
              'shrink-0 text-right text-xs',
              item.kind === 'OVERDUE'
                ? 'font-semibold text-rose-600 dark:text-rose-400'
                : 'text-muted-foreground',
            )}
            title={formatDateTime(item.due_at)}
          >
            {item.kind === 'OVERDUE' && <span className="block">{t('dashboard.timeline.overdue')}</span>}
            {item.due_at && formatRelative(item.due_at)}
          </span>
        )}
        <ArrowRightIcon
          className="size-4 shrink-0 text-muted-foreground transition-transform group-hover:translate-x-0.5"
          aria-hidden
        />
      </Link>
    </li>
  )
}
