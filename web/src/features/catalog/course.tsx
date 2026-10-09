import { useTranslation } from 'react-i18next'

import type { CourseKind } from '@/shared/api/generated'
import { Badge } from '@/shared/ui/badge'

/** CourseKindBadge, olağan dersler dışındaki ders türlerini (staj, proje ...) işaretler. */
export function CourseKindBadge({ kind }: { kind: CourseKind }) {
  const { t } = useTranslation()
  if (kind === 'REGULAR') {
    return null
  }
  return <Badge variant="outline">{t(`catalog.kinds.${kind}`)}</Badge>
}
