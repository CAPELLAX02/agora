import { useTranslation } from 'react-i18next'

import type { UserStatus } from '@/shared/api/generated'

import { Badge } from './badge'

const variants = {
  ACTIVE: 'success',
  PENDING: 'warning',
  SUSPENDED: 'destructive',
  DISABLED: 'muted',
} as const

export function StatusBadge({ status }: { status: UserStatus }) {
  const { t } = useTranslation()
  return <Badge variant={variants[status]}>{t(`status.${status}`)}</Badge>
}
