import { PinIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import type { Announcement, AnnouncementAudience } from '@/shared/api/generated'
import { cn } from '@/shared/lib/utils'
import { Badge } from '@/shared/ui/badge'
import { toneSoft } from '@/shared/ui/tone'

import { audienceKey, typeMeta } from './meta'

/** TypeBadge, duyuru türünün renkli rozetidir. */
export function TypeBadge({ type, className }: { type: Announcement['type']; className?: string }) {
  const { t } = useTranslation()
  const meta = typeMeta[type]
  const Icon = meta.icon
  return (
    <span
      className={cn(
        'inline-flex items-center gap-1 rounded-md px-1.5 py-0.5 text-xs font-bold [&>svg]:size-3.5',
        toneSoft[meta.tone],
        className,
      )}
    >
      <Icon aria-hidden />
      {t(`announcements.types.${type}`)}
    </span>
  )
}

/** PinnedMark, sabitlenmiş duyurunun işaretidir. */
export function PinnedMark() {
  const { t } = useTranslation()
  return <PinIcon className="size-4 shrink-0 text-amber-500" aria-label={t('announcements.pinned')} />
}

/** AudienceList, duyurunun hedeflerini rozet olarak gösterir. */
export function AudienceList({ audiences }: { audiences: AnnouncementAudience[] }) {
  const { t } = useTranslation()
  return (
    <ul className="flex flex-wrap gap-1.5" aria-label={t('announcements.audience')}>
      {audiences.map((a) => (
        <li key={audienceKey(a)}>
          <Badge variant="outline">
            {a.scope_type === 'UNIVERSITY' ? t('announcements.university') : a.scope_name} ·{' '}
            {t(`announcements.people.${a.people}`)}
          </Badge>
        </li>
      ))}
    </ul>
  )
}
