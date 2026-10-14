import { CalendarClockIcon, EyeOffIcon, LockIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import type { Availability } from '@/shared/api/generated'
import { formatDateTime } from '@/i18n/format'
import { Badge } from '@/shared/ui/badge'

type Item = {
  is_visible: boolean
  availability: Availability
  available_from: string | null
  available_until: string | null
}

/** AvailabilityBadges, bölüm ya da etkinliğin gizli, açılacak, kapandı ve kapanacak etiketleridir. */
export function AvailabilityBadges({ item }: { item: Item }) {
  const { t } = useTranslation()
  return (
    <>
      {!item.is_visible && (
        <Badge variant="muted" className="gap-1">
          <EyeOffIcon className="size-3" />
          {t('lms.hidden')}
        </Badge>
      )}
      {item.availability === 'NOT_YET' && item.available_from && (
        <Badge variant="info" className="gap-1">
          <CalendarClockIcon className="size-3" />
          {t('lms.opensAt', { time: formatDateTime(item.available_from) })}
        </Badge>
      )}
      {item.availability === 'CLOSED' && (
        <Badge variant="muted" className="gap-1">
          <LockIcon className="size-3" />
          {t('lms.closed')}
        </Badge>
      )}
      {item.availability === 'OPEN' && item.available_until && (
        <Badge variant="warning" className="gap-1">
          <CalendarClockIcon className="size-3" />
          {t('lms.closesAt', { time: formatDateTime(item.available_until) })}
        </Badge>
      )}
    </>
  )
}
