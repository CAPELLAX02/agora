import { useTranslation } from 'react-i18next'

import { day } from '@/features/schedule/layout'
import type { RegistrationSection } from '@/shared/api/generated'
import { cn } from '@/shared/lib/utils'

/** SectionSchedule, şubenin haftalık oturumlarını kısa bir satır olarak yazar. */
export function SectionSchedule({ section }: { section: RegistrationSection }) {
  const { t } = useTranslation()
  if (section.slots.length === 0) {
    return <span className="text-muted-foreground">{t('registration.section.noSchedule')}</span>
  }
  return (
    <span className="tabular-nums">
      {section.slots
        .map(
          (s) =>
            `${t(`schedule.days.${day(s.day)}`)} ${s.start}–${s.end}${s.classroom ? ` · ${s.classroom}` : ''}${
              s.kind !== 'THEORY' ? ` (${t(`schedule.sessions.${s.kind}`)})` : ''
            }`,
        )
        .join('; ')}
    </span>
  )
}

/** SectionSeats, şubenin doluluğudur; ayrılmış kontenjanlı şubede programa kalan yer de yazılır. */
export function SectionSeats({ section }: { section: RegistrationSection }) {
  const { t } = useTranslation()
  const left = section.capacity - section.enrolled
  return (
    <span className={cn('tabular-nums', !section.available && 'text-destructive')}>
      {t('registration.section.seats', { enrolled: section.enrolled, capacity: section.capacity })}
      {section.quota_mode === 'RESERVED' &&
        ` · ${t('registration.section.quotaLeft', { count: section.quota_left ?? 0 })}`}
      {section.available ? '' : ` · ${t('registration.section.full')}`}
      {section.available &&
        left <= 5 &&
        section.quota_mode === 'OPEN' &&
        ` · ${t('registration.section.fewLeft', { count: left })}`}
    </span>
  )
}
