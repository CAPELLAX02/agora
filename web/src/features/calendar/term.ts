import type { TFunction } from 'i18next'

import type { Term } from '@/shared/api/generated'

/** termLabel, dönemi "2026-2027 Güz" biçiminde gösterir; içinde bulunulan dönem işaretlenir. */
export function termLabel(t: TFunction, term: Term): string {
  const label = `${term.academic_year} ${t(`calendar.termTypes.${term.term_type}`)}`
  return term.is_current ? `${label} · ${t('calendar.current')}` : label
}

/** toLocalInput, ISO zamanı datetime-local girdisinin beklediği yerel biçime çevirir. */
export function toLocalInput(iso: string): string {
  const d = new Date(iso)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}
