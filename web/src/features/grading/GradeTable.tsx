import { useRef, type KeyboardEvent } from 'react'
import { useTranslation } from 'react-i18next'

import type { Gradebook } from '@/shared/api/generated'
import { cn } from '@/shared/lib/utils'
import { Badge } from '@/shared/ui/badge'
import { Checkbox } from '@/shared/ui/checkbox'
import { LetterGrade } from '@/shared/ui/letter-grade'

import { componentColors } from './colors'
import { cellKey, formatCell, parseCell, sameCell } from './scoreCell'

type Component = Gradebook['components'][number]

/**
 * GradeTable, şubenin not tablosudur: satırlar öğrenciler, sütunlar bileşenler. Düzenlenebilir
 * bileşenlerin hücreleri giriş alanıdır; yazılanlar kaydedilene kadar taslaktır (sarı), geçersiz
 * değer kırmızıdır. Klavye: ↑ ↓ ve Enter satır, ← → (imleç kenardayken) sütun değiştirir, Esc
 * hücreyi geri alır. Devamsızlık kutusu hemen kaydedilir.
 */
export function GradeTable({
  book,
  drafts,
  editable,
  onDraft,
  attendanceEditable,
  onAttendance,
}: {
  book: Gradebook
  drafts: Map<string, string>
  editable: (c: Component) => boolean
  onDraft: (key: string, text: string | undefined) => void
  attendanceEditable: boolean
  onAttendance: (enrollmentId: string, fail: boolean) => void
}) {
  const { t, i18n } = useTranslation()
  const en = i18n.language.startsWith('en')
  const absentMark = en ? 'ABS' : 'G'
  const decimal = en ? '.' : ','
  const colorOf = componentColors(book.components)
  const inputs = useRef(new Map<string, HTMLInputElement>())

  // Odak taşıma: hedef yönde ilk giriş alanı.
  const focus = (row: number, col: number, dRow: number, dCol: number) => {
    const rows = book.students.length
    const cols = book.components.length
    for (let r = row + dRow, c = col + dCol; r >= 0 && r < rows && c >= 0 && c < cols; r += dRow, c += dCol) {
      const el = inputs.current.get(`${r}:${c}`)
      if (el) {
        el.focus()
        el.select()
        return
      }
      if (dRow === 0 && dCol === 0) {
        return
      }
    }
  }

  const onKeyDown = (e: KeyboardEvent<HTMLInputElement>, row: number, col: number, key: string) => {
    const el = e.currentTarget
    // Hücre odaklanınca tamamı seçilir: o durumda oklar doğrudan komşu hücreye geçer.
    const all = el.selectionStart === 0 && el.selectionEnd === el.value.length
    const atStart = all || (el.selectionStart === 0 && el.selectionEnd === 0)
    const atEnd = all || el.selectionStart === el.value.length
    switch (e.key) {
      case 'ArrowDown':
        e.preventDefault()
        focus(row, col, 1, 0)
        break
      case 'Enter':
        e.preventDefault()
        focus(row, col, e.shiftKey ? -1 : 1, 0)
        break
      case 'ArrowUp':
        e.preventDefault()
        focus(row, col, -1, 0)
        break
      case 'ArrowRight':
        if (atEnd) {
          e.preventDefault()
          focus(row, col, 0, 1)
        }
        break
      case 'ArrowLeft':
        if (atStart) {
          e.preventDefault()
          focus(row, col, 0, -1)
        }
        break
      case 'Escape':
        onDraft(key, undefined)
        break
    }
  }

  return (
    <div className="overflow-x-auto rounded-2xl border border-border/70 bg-card shadow-xs">
      <table className="w-full min-w-max border-collapse text-sm">
        <thead>
          <tr className="border-b bg-muted text-left">
            <th scope="col" className="sticky left-0 z-20 bg-muted px-3 py-2.5 font-semibold">
              {t('gradebook.student')}
            </th>
            <th scope="col" className="px-3 py-2.5 font-semibold">
              {t('gradebook.attendance')}
            </th>
            {book.components.map((c) => (
              <th key={c.id} scope="col" className="min-w-28 px-2 py-2.5 text-center font-semibold">
                <span className="flex items-center justify-center gap-1.5">
                  <span aria-hidden className={cn('size-2 rounded-full', colorOf(c))} />
                  {en ? c.label_en : c.label_tr}
                </span>
                <span className="block text-xs font-normal text-muted-foreground">
                  %{c.weight}
                  {editable(c) && ` · ${t('gradebook.editable')}`}
                  {c.lms_fed && ` · ${t('gradebook.lmsFed')}`}
                </span>
              </th>
            ))}
            <th scope="col" className="px-3 py-2.5 text-center font-semibold">
              {t('gradebook.letter')}
            </th>
          </tr>
        </thead>
        <tbody>
          {book.students.map((st, row) => {
            const fail = st.attendance_status === 'ATTENDANCE_FAIL'
            const name = `${st.student.first_name} ${st.student.last_name}`
            return (
              <tr
                key={st.enrollment_id}
                className="border-b border-border/60 last:border-0 hover:bg-primary/[0.03]"
              >
                <th scope="row" className="sticky left-0 z-10 bg-card px-3 py-2 text-left font-normal">
                  <span className="block font-semibold">{name}</span>
                  <span className="flex items-center gap-1.5 font-mono text-xs text-muted-foreground">
                    {st.student.student_no}
                    {st.attempt_no > 1 && (
                      <Badge variant="info" className="px-1.5 py-0 font-sans">
                        {t('gradebook.repeat', { n: st.attempt_no })}
                      </Badge>
                    )}
                  </span>
                </th>
                <td className="px-3 py-2">
                  {attendanceEditable ? (
                    <label className="flex cursor-pointer items-center gap-2 text-xs">
                      <Checkbox
                        checked={fail}
                        onCheckedChange={(v) => onAttendance(st.enrollment_id, v === true)}
                        aria-label={t('gradebook.attendanceFailFor', { name })}
                      />
                      <span className={cn(fail ? 'font-semibold text-rose-600' : 'text-muted-foreground')}>
                        {t('gradebook.attendanceFail')}
                      </span>
                    </label>
                  ) : fail ? (
                    <Badge variant="destructive">{t('gradebook.attendanceFail')}</Badge>
                  ) : (
                    <span className="text-xs text-muted-foreground">{t('gradebook.continuing')}</span>
                  )}
                </td>
                {book.components.map((c, col) => {
                  const saved = st.scores[c.id]
                  const key = cellKey(st.enrollment_id, c.id)
                  const draft = drafts.get(key)
                  const label = `${name} · ${en ? c.label_en : c.label_tr}`
                  // Bütünleme notu sadece hakkı olan öğrenciye girilir.
                  if (!editable(c) || (c.type.category === 'MAKEUP' && !st.makeup_eligible)) {
                    return (
                      <td key={c.id} className="px-2 py-2 text-center tabular-nums">
                        {saved?.absent ? (
                          <Badge variant="warning">{absentMark}</Badge>
                        ) : (
                          <span className={cn(!saved && 'text-muted-foreground/60')}>
                            {formatCell(saved, absentMark, decimal) || '—'}
                          </span>
                        )}
                      </td>
                    )
                  }
                  const parsed = draft === undefined ? null : parseCell(draft)
                  const invalid = draft !== undefined && parsed === null
                  const dirty = draft !== undefined && !invalid && !sameCell(parsed ?? undefined, saved)
                  return (
                    <td key={c.id} className="px-1.5 py-1.5">
                      <input
                        ref={(el) => {
                          if (el) {
                            inputs.current.set(`${row}:${col}`, el)
                          } else {
                            inputs.current.delete(`${row}:${col}`)
                          }
                        }}
                        aria-label={label}
                        aria-invalid={invalid || undefined}
                        inputMode="decimal"
                        autoComplete="off"
                        value={draft ?? formatCell(saved, absentMark, decimal)}
                        onChange={(e) => onDraft(key, e.target.value)}
                        onFocus={(e) => e.currentTarget.select()}
                        onKeyDown={(e) => onKeyDown(e, row, col, key)}
                        className={cn(
                          'h-9 w-full min-w-20 rounded-lg border border-input bg-card px-2 text-center font-semibold uppercase tabular-nums outline-none',
                          'focus-visible:border-primary/60 focus-visible:ring-4 focus-visible:ring-ring/20',
                          dirty && 'border-amber-400 bg-amber-50 dark:bg-amber-400/10',
                          invalid && 'border-destructive bg-rose-50 ring-destructive/20 dark:bg-rose-500/10',
                          /^(g|gr|abs|absent)$/i.test(draft ?? formatCell(saved, absentMark, decimal)) &&
                            'text-amber-700 dark:text-amber-300',
                        )}
                      />
                    </td>
                  )
                })}
                <td className="px-3 py-2 text-center">
                  {st.letter ? (
                    <LetterGrade letter={st.letter} />
                  ) : (
                    <span className="text-muted-foreground/60">—</span>
                  )}
                </td>
              </tr>
            )
          })}
        </tbody>
      </table>
    </div>
  )
}
