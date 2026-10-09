import { useTranslation } from 'react-i18next'

import type { OfferingStatus } from '@/shared/api/generated'
import { Badge } from '@/shared/ui/badge'

const variants = { PLANNED: 'warning', OPEN: 'success', CLOSED: 'muted', CANCELLED: 'destructive' } as const

export function OfferingStatusBadge({ status }: { status: OfferingStatus }) {
  const { t } = useTranslation()
  return <Badge variant={variants[status]}>{t(`offering.statuses.${status}`)}</Badge>
}
