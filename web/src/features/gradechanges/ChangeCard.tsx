import { ArrowRightIcon, MessageSquareQuoteIcon } from 'lucide-react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { formatDateTime } from '@/i18n/format'
import type { GradeChangeRequest, GradeValue } from '@/shared/api/generated'
import { useLocalized } from '@/shared/lib/localized'
import { Badge } from '@/shared/ui/badge'
import { LetterGrade } from '@/shared/ui/letter-grade'

const statusVariant = {
  PENDING: 'warning',
  APPLIED: 'success',
  REJECTED: 'destructive',
  WITHDRAWN: 'muted',
} as const

/** ChangeStatusBadge, not düzeltme talebinin durumudur. */
export function ChangeStatusBadge({ status }: { status: GradeChangeRequest['status'] }) {
  const { t } = useTranslation()
  return <Badge variant={statusVariant[status]}>{t(`gradeChanges.status.${status}`)}</Badge>
}

/** GradeDiff, notun önceki ve sonraki halini yan yana gösterir (bileşen puanı ve harf). */
export function GradeDiff({
  kind,
  componentLabel,
  old,
  next,
}: {
  kind: GradeChangeRequest['kind']
  componentLabel?: string
  old: GradeValue
  next: GradeValue
}) {
  const { t } = useTranslation()
  const score = (v: GradeValue) => (v.absent ? t('grades.didNotSit') : (v.score?.toString() ?? '—'))
  return (
    <div className="space-y-2">
      {kind === 'SCORE' && (
        <p className="flex flex-wrap items-center gap-2 text-sm">
          <span className="font-medium">{componentLabel}</span>
          <span className="text-muted-foreground tabular-nums line-through">{score(old)}</span>
          <ArrowRightIcon className="size-3.5 text-muted-foreground" aria-hidden />
          <strong className="tabular-nums">{score(next)}</strong>
        </p>
      )}
      <p className="flex flex-wrap items-center gap-2">
        <LetterGrade letter={old.letter} className="opacity-70" />
        <ArrowRightIcon className="size-4 text-muted-foreground" aria-label={t('gradeChanges.becomes')} />
        <LetterGrade letter={next.letter} className="min-w-11 py-1 text-sm" />
        {old.weighted_score !== next.weighted_score && (
          <span className="text-xs text-muted-foreground tabular-nums">
            {old.weighted_score?.toFixed(2) ?? '—'} → {next.weighted_score?.toFixed(2) ?? '—'}
          </span>
        )}
      </p>
    </div>
  )
}

/** ChangeCard, bir not düzeltme talebidir: ders, öğrenci, değişiklik, gerekçe ve karar. */
export function ChangeCard({ change: c, actions }: { change: GradeChangeRequest; actions?: ReactNode }) {
  const { t, i18n } = useTranslation()
  const localized = useLocalized()
  const label = c.component
    ? i18n.language.startsWith('en')
      ? c.component.label_en
      : c.component.label_tr
    : undefined
  return (
    <article
      className="overflow-hidden rounded-2xl border bg-card shadow-xs"
      aria-label={`${c.course.code} · ${c.student.first_name} ${c.student.last_name}`}
    >
      <header className="flex items-start justify-between gap-3 border-b bg-muted/30 px-4 py-3">
        <div className="min-w-0">
          <p className="font-mono text-xs text-muted-foreground">
            {c.course.code}-{c.section.code} · {c.term_code}
          </p>
          <h2 className="leading-snug font-bold">{localized(c.course)}</h2>
        </div>
        <ChangeStatusBadge status={c.status} />
      </header>
      <div className="grid gap-4 px-4 py-3.5 sm:grid-cols-2">
        <div>
          <p className="text-xs font-semibold text-muted-foreground">{t('gradeChanges.student')}</p>
          <p className="font-semibold">
            {c.student.first_name} {c.student.last_name}
          </p>
          <p className="font-mono text-xs text-muted-foreground">{c.student.student_no}</p>
        </div>
        <div>
          <p className="mb-1 text-xs font-semibold text-muted-foreground">
            {t(`gradeChanges.kinds.${c.kind}`)}
          </p>
          <GradeDiff kind={c.kind} componentLabel={label} old={c.old} next={c.new} />
        </div>
      </div>
      <blockquote className="mx-4 mb-3 flex gap-2 rounded-xl bg-muted/50 px-3 py-2 text-sm">
        <MessageSquareQuoteIcon className="mt-0.5 size-4 shrink-0 text-muted-foreground" aria-hidden />
        <span>{c.reason}</span>
      </blockquote>
      <footer className="flex flex-wrap items-end justify-between gap-3 border-t px-4 py-3 text-xs text-muted-foreground">
        <div className="space-y-0.5">
          <p>
            {t('gradeChanges.requestedBy', { name: c.requested_by, time: formatDateTime(c.requested_at) })}
          </p>
          {c.decided_at && c.reviewed_by && (
            <p>{t('gradeChanges.decidedBy', { name: c.reviewed_by, time: formatDateTime(c.decided_at) })}</p>
          )}
          {c.review_note && <p className="text-foreground">“{c.review_note}”</p>}
        </div>
        {actions && <div className="flex flex-wrap gap-2">{actions}</div>}
      </footer>
    </article>
  )
}
