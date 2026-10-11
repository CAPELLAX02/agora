import { useTranslation } from 'react-i18next'

import { WeeklyGrid } from '@/features/schedule/WeeklyGrid'
import type { RegistrationSection } from '@/shared/api/generated'

import { toEntries } from './shared'

/**
 * WeeklyPreview, sepetteki şubelerin haftalık programıdır. Çakışan oturumlar yan yana
 * şeritlere bölünür; çakışma kurallar listesinde de yazar.
 */
export function WeeklyPreview({ sections }: { sections: RegistrationSection[] }) {
  const { t } = useTranslation()
  const instructors = new Map(sections.map((s) => [s.id, s.instructors[0] ?? '']))
  return (
    <WeeklyGrid
      entries={toEntries(sections)}
      detail={(e) => {
        const room = e.classroom ? `${e.classroom.building_code} ${e.classroom.code}` : t('schedule.online')
        const who = instructors.get(e.section_id)
        return who ? `${room} · ${who}` : room
      }}
    />
  )
}
