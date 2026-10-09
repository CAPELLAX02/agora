import { useTranslation } from 'react-i18next'

import type { CurriculumStatus } from '@/shared/api/generated'
import { Badge } from '@/shared/ui/badge'

const variants = { DRAFT: 'warning', ACTIVE: 'success', ARCHIVED: 'muted' } as const

export function CurriculumStatusBadge({ status }: { status: CurriculumStatus }) {
  const { t } = useTranslation()
  return <Badge variant={variants[status]}>{t(`curriculum.statuses.${status}`)}</Badge>
}

/** YearRange, sürümü izleyen giriş yıllarını gösterir: "2018-2022 girişliler". */
export function YearRange({ from, to }: { from: number; to: number | null }) {
  const { t } = useTranslation()
  return <>{to === null ? t('curriculum.yearsOpen', { from }) : t('curriculum.years', { from, to })}</>
}
