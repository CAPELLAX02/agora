import { useTranslation } from 'react-i18next'

import type { AddDropRequest, RegistrationItem, RegistrationStatus } from '@/shared/api/generated'
import { Badge } from '@/shared/ui/badge'

const statusVariant = {
  NOT_STARTED: 'muted',
  DRAFT: 'secondary',
  SUBMITTED: 'warning',
  APPROVED: 'success',
  REJECTED: 'destructive',
} as const

/** RegistrationStatusBadge, ders seçme kaydının durumudur. */
export function RegistrationStatusBadge({ status }: { status: RegistrationStatus }) {
  const { t } = useTranslation()
  return <Badge variant={statusVariant[status]}>{t(`registration.status.${status}`)}</Badge>
}

/** ItemKindBadge, dersin alınış türüdür (yeni alınışta gösterilmez). */
export function ItemKindBadge({ kind }: { kind: RegistrationItem['kind'] }) {
  const { t } = useTranslation()
  if (kind === 'NEW') {
    return null
  }
  return (
    <Badge variant={kind === 'REPEAT_UPGRADE' ? 'outline' : 'warning'}>
      {t(`registration.kinds.${kind}`)}
    </Badge>
  )
}

const addDropVariant = {
  PENDING_ADVISOR: 'warning',
  PENDING_DEPT_HEAD: 'warning',
  APPROVED: 'success',
  REJECTED: 'destructive',
  WITHDRAWN: 'muted',
} as const

/** AddDropStatusBadge, ekle-bırak talebinin durumudur. */
export function AddDropStatusBadge({ status }: { status: AddDropRequest['status'] }) {
  const { t } = useTranslation()
  return <Badge variant={addDropVariant[status]}>{t(`registration.addDrop.status.${status}`)}</Badge>
}
