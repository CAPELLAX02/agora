import { useTranslation } from 'react-i18next'

import type { CourseAttempt, ProgressLine, ProgressStatus, StudentProgress } from '@/shared/api/generated'
import { useLocalized } from '@/shared/lib/localized'
import { Badge } from '@/shared/ui/badge'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/shared/ui/table'

const statusVariant = {
  PASSED: 'success',
  FAILED: 'destructive',
  IN_PROGRESS: 'secondary',
  PARTIAL: 'warning',
  NOT_TAKEN: 'muted',
} as const

/** ProgressBadge, müfredat satırının öğrenci için durumudur. */
export function ProgressBadge({ status }: { status: ProgressStatus }) {
  const { t } = useTranslation()
  return <Badge variant={statusVariant[status]}>{t(`registration.progress.status.${status}`)}</Badge>
}

/** ProgressSummary, mezuniyet ilerlemesidir: tamamlanan, süren ve gereken AKTS, kalan ders, GANO. */
export function ProgressSummary({ progress: p }: { progress: StudentProgress }) {
  const { t } = useTranslation()
  const s = p.summary
  const ratio = s.required_ects > 0 ? Math.min(s.completed_ects / s.required_ects, 1) : 0
  return (
    <div className="space-y-3" aria-label={t('registration.progress.summary')} role="group">
      <div className="h-2 overflow-hidden rounded-full bg-muted">
        <div className="h-full bg-success" style={{ width: `${ratio * 100}%` }} />
      </div>
      <dl className="grid grid-cols-[repeat(auto-fill,minmax(9rem,1fr))] gap-3">
        {(
          [
            [t('registration.progress.completed'), `${s.completed_ects} / ${s.required_ects}`],
            [t('registration.progress.inProgress'), String(s.in_progress_ects)],
            [t('registration.progress.remaining'), String(s.remaining_courses)],
            [t('registration.progress.gpa'), p.gpa?.toFixed(2) ?? '—'],
          ] as const
        ).map(([label, value]) => (
          <div key={label} className="rounded-md border p-3">
            <dt className="text-xs text-muted-foreground">{label}</dt>
            <dd className="mt-1 text-lg font-semibold tabular-nums">{value}</dd>
          </div>
        ))}
      </dl>
    </div>
  )
}

/** lineDetail, satırın son denemesini ya da yuvayı dolduran dersleri kısaca yazar. */
function useLineDetail() {
  const { t } = useTranslation()
  return (l: ProgressLine): string => {
    if (l.item_type === 'ELECTIVE_SLOT') {
      const taken = [...l.filled, ...l.in_progress].map(
        (a) => `${a.course_code}${a.letter ? ` (${a.letter})` : ''}`,
      )
      return [
        taken.join(', '),
        l.remaining > 0 ? t('registration.progress.slotRemaining', { count: l.remaining }) : '',
      ]
        .filter(Boolean)
        .join(' · ')
    }
    const a = l.last_attempt
    if (!a) {
      return ''
    }
    const via =
      a.course_code !== l.code ? ` · ${t('registration.progress.via', { code: a.course_code })}` : ''
    return `${a.letter ?? t('registration.progress.ongoing')} · ${a.term_code}${via}`
  }
}

/**
 * ProgressView, müfredata göre ders durumunu yarıyıl yarıyıl gösterir (danışman ve öğrenci
 * işleri ekranları için sıkı bir tablo).
 */
export function ProgressView({ progress: p }: { progress: StudentProgress }) {
  const { t } = useTranslation()
  const localized = useLocalized()
  const detail = useLineDetail()
  const semesters = [...new Set(p.lines.map((l) => l.semester_no))].sort((a, b) => a - b)
  return (
    <div className="space-y-4">
      <ProgressSummary progress={p} />
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead className="w-12">{t('registration.progress.semester')}</TableHead>
            <TableHead className="w-28">{t('catalog.code')}</TableHead>
            <TableHead>{t('catalog.name')}</TableHead>
            <TableHead className="w-14 text-right">{t('catalog.ects')}</TableHead>
            <TableHead className="w-32">{t('registration.progress.state')}</TableHead>
            <TableHead>{t('registration.progress.detail')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {semesters.flatMap((no) =>
            p.lines
              .filter((l) => l.semester_no === no)
              .map((l, i) => (
                <TableRow key={l.item_id} className={i === 0 ? 'border-t-2' : undefined}>
                  <TableCell className="tabular-nums">{i === 0 ? no : ''}</TableCell>
                  <TableCell className="font-mono text-xs">{l.code}</TableCell>
                  <TableCell>{localized(l)}</TableCell>
                  <TableCell className="text-right tabular-nums">{l.ects}</TableCell>
                  <TableCell>
                    <ProgressBadge status={l.status} />
                  </TableCell>
                  <TableCell className="text-xs text-muted-foreground">{detail(l)}</TableCell>
                </TableRow>
              )),
          )}
        </TableBody>
      </Table>
      {p.extra.length > 0 && (
        <p className="text-xs text-muted-foreground">
          {t('registration.progress.extra', { courses: p.extra.map((a) => a.course_code).join(', ') })}
        </p>
      )}
    </div>
  )
}

/** HistoryTable, ders geçmişidir: dönem, ders, deneme, not; yerini yeni denemeye bırakanlar soluk. */
export function HistoryTable({ items }: { items: CourseAttempt[] }) {
  const { t } = useTranslation()
  const localized = useLocalized()
  if (items.length === 0) {
    return <p className="text-sm text-muted-foreground">{t('registration.history.empty')}</p>
  }
  return (
    <Table aria-label={t('registration.history.title')}>
      <TableHeader>
        <TableRow>
          <TableHead className="w-28">{t('registration.history.term')}</TableHead>
          <TableHead className="w-28">{t('catalog.code')}</TableHead>
          <TableHead>{t('catalog.name')}</TableHead>
          <TableHead className="w-16 text-right">{t('registration.history.attempt')}</TableHead>
          <TableHead className="w-14 text-right">{t('catalog.ects')}</TableHead>
          <TableHead className="w-20">{t('registration.history.letter')}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {items.map((a) => (
          <TableRow key={a.id} className={a.superseded ? 'text-muted-foreground' : undefined}>
            <TableCell className="tabular-nums">{a.term.code}</TableCell>
            <TableCell className="font-mono text-xs">{a.course.code}</TableCell>
            <TableCell>{localized(a.course)}</TableCell>
            <TableCell className="text-right tabular-nums">{a.attempt_no}</TableCell>
            <TableCell className="text-right tabular-nums">{a.ects}</TableCell>
            <TableCell>
              {a.letter_grade ? (
                <Badge variant={a.is_passing ? 'outline' : 'destructive'}>
                  {a.letter_grade}
                  {a.attendance_fail && (
                    <span className="sr-only"> ({t('registration.history.absence')})</span>
                  )}
                </Badge>
              ) : (
                <Badge variant="secondary">{t('registration.history.ongoing')}</Badge>
              )}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}
