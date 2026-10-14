import type { ComponentScore } from '@/shared/api/generated'

/**
 * Not tablosunun hücre değeri: puan, "girmedi" ya da boş. Sunucunun kurallarının aynası:
 * 0-100 arası, en fazla iki ondalık; ondalık ayracı virgül ya da nokta; girmedi için G.
 */
export type CellValue = { score: number | null; absent: boolean }

const absentMarks = ['g', 'gr', 'girmedi', 'absent', 'abs']

/** parseCell, yazılan metni okur; boş metin notun silinmesidir. Geçersizse null. */
export function parseCell(text: string): CellValue | null {
  const s = text.trim()
  if (s === '') {
    return { score: null, absent: false }
  }
  if (absentMarks.includes(s.toLocaleLowerCase('tr-TR').replace('ı', 'i'))) {
    return { score: null, absent: true }
  }
  if (!/^\d{1,3}([.,]\d{1,2})?$/.test(s)) {
    return null
  }
  const v = Number(s.replace(',', '.'))
  return v >= 0 && v <= 100 ? { score: v, absent: false } : null
}

/** formatCell, değeri hücrede gösterilecek metne çevirir (dilin ondalık ayracıyla). */
export function formatCell(
  v: ComponentScore | CellValue | undefined,
  absentMark: string,
  decimal: string,
): string {
  if (!v) {
    return ''
  }
  if (v.absent) {
    return absentMark
  }
  return v.score === null ? '' : String(v.score).replace('.', decimal)
}

/** sameCell, iki değerin aynı not olduğunu söyler (girilmemiş = boş). */
export function sameCell(
  a: CellValue | ComponentScore | undefined,
  b: CellValue | ComponentScore | undefined,
) {
  const norm = (v: CellValue | ComponentScore | undefined) =>
    !v || (!v.absent && v.score === null) ? 'empty' : v.absent ? 'absent' : String(v.score)
  return norm(a) === norm(b)
}

/** cellKey, taslak hücrenin anahtarıdır. */
export const cellKey = (enrollmentId: string, componentId: string) => `${enrollmentId}:${componentId}`
