import type { TFunction } from 'i18next'

import type { Term } from '@/shared/api/generated'

/** termLabel, dönemi "2026-2027 Güz" biçiminde gösterir; içinde bulunulan dönem işaretlenir. */
export function termLabel(t: TFunction, term: Term): string {
  const label = `${term.academic_year} ${t(`calendar.termTypes.${term.term_type}`)}`
  return term.is_current ? `${label} · ${t('calendar.current')}` : label
}
