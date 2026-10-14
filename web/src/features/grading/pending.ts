import type { Gradebook } from '@/shared/api/generated'

type Component = Gradebook['components'][number]

/**
 * pendingComponents, not girişi bekleyen bileşenlerdir: pencere açık, plan kilitli, notun
 * aşaması uygun (ilandan önce bütünleme dışındakiler, ilandan sonra bütünleme) ve girilen not
 * beklenenden az (bütünlemede sadece hakkı olanlar beklenir). Asistan not girmez.
 */
export function pendingComponents(book: Gradebook): { component: Component; missing: number }[] {
  const s = book.section
  if (!s.plan_locked || s.instructor_role === 'ASSISTANT') {
    return []
  }
  return book.components.flatMap((c) => {
    const makeup = c.type.category === 'MAKEUP'
    const phase = makeup ? Boolean(s.grades_published_at) && !s.makeup_published_at : !s.grades_published_at
    if (!c.entry_window.open || !phase) {
      return []
    }
    const expected = makeup ? book.students.filter((st) => st.makeup_eligible).length : book.students.length
    const missing = expected - c.scored_count - c.absent_count
    return missing > 0 ? [{ component: c, missing }] : []
  })
}
