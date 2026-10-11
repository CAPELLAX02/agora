import { ClockIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { day } from '@/features/schedule/layout'
import type { RegistrationSection } from '@/shared/api/generated'
import { cn } from '@/shared/lib/utils'

/** SectionSchedule, şubenin haftalık oturumlarını gün ve saat çipleri olarak gösterir. */
export function SectionSchedule({
  section,
  compact = false,
}: {
  section: RegistrationSection
  /** compact, derslik adını yazmaz (dar alanlar: sepet). */
  compact?: boolean
}) {
  const { t } = useTranslation()
  if (section.slots.length === 0) {
    return <span className="text-muted-foreground italic">{t('registration.section.noSchedule')}</span>
  }
  return (
    <span className="inline-flex flex-wrap gap-1.5">
      {section.slots.map((s, i) => (
        <span
          key={i}
          className="inline-flex items-center gap-1 rounded-md bg-card px-1.5 py-0.5 text-[11px] whitespace-nowrap tabular-nums ring-1 ring-border/70"
        >
          <ClockIcon className="size-3 text-muted-foreground" aria-hidden />
          <span className="font-semibold">{t(`schedule.days.${day(s.day)}`)}</span>
          {s.start}–{s.end}
          {s.classroom && !compact && <span className="text-muted-foreground">· {s.classroom}</span>}
          {s.kind !== 'THEORY' && (
            <span className="font-semibold text-violet-600 dark:text-violet-300">
              ({t(`schedule.sessions.${s.kind}`)})
            </span>
          )}
        </span>
      ))}
    </span>
  )
}

/** SectionSeats, şubenin doluluğudur; ayrılmış kontenjanlı şubede programa kalan yer de yazılır. */
export function SectionSeats({ section }: { section: RegistrationSection }) {
  const { t } = useTranslation()
  const left = section.capacity - section.enrolled
  const few = section.available && left <= 5 && section.quota_mode === 'OPEN'
  return (
    <span
      className={cn(
        'tabular-nums',
        !section.available
          ? 'font-semibold text-rose-600 dark:text-rose-400'
          : few
            ? 'font-semibold text-amber-700 dark:text-amber-300'
            : 'text-muted-foreground',
      )}
    >
      {t('registration.section.seats', { enrolled: section.enrolled, capacity: section.capacity })}
      {section.quota_mode === 'RESERVED' &&
        ` · ${t('registration.section.quotaLeft', { count: section.quota_left ?? 0 })}`}
      {section.available ? '' : ` · ${t('registration.section.full')}`}
      {few && ` · ${t('registration.section.fewLeft', { count: left })}`}
    </span>
  )
}
