import { useTranslation } from 'react-i18next'

import type { EnrollmentStatus, HoldType } from '@/shared/api/generated'
import { Badge } from '@/shared/ui/badge'

const variant = {
  PREP: 'secondary',
  ACTIVE: 'success',
  FROZEN: 'warning',
  SUSPENDED: 'destructive',
  GRADUATED: 'outline',
  WITHDRAWN: 'muted',
  DISMISSED: 'muted',
} as const

/** EnrollmentStatusBadge, program kaydının durumudur. */
export function EnrollmentStatusBadge({ status }: { status: EnrollmentStatus }) {
  const { t } = useTranslation()
  return <Badge variant={variant[status]}>{t(`students.status.${status}`)}</Badge>
}

/** HoldTypeBadge, kayıt engelinin türüdür. */
export function HoldTypeBadge({ type }: { type: HoldType }) {
  const { t } = useTranslation()
  return <Badge variant="destructive">{t(`students.holds.types.${type}`)}</Badge>
}
