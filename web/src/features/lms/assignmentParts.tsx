import { DownloadIcon, FileIcon, SendIcon } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { formatDateTime, formatNumber, formatRelative } from '@/i18n/format'
import type { AssignmentGrade, Submission, SubmissionComment } from '@/shared/api/generated'
import { formatBytes } from '@/shared/api/upload'
import { cn } from '@/shared/lib/utils'
import { Badge } from '@/shared/ui/badge'
import { Button } from '@/shared/ui/button'
import { RichText } from '@/shared/ui/rich-text'
import { Textarea } from '@/shared/ui/textarea'

import { useLocale } from './assignmentText'

/** SubmissionStatus, son denemenin durumudur: başlamadı, taslak, gönderildi, geç gönderildi. */
export function SubmissionStatus({
  latest,
}: {
  latest: { status: string; is_late: boolean } | null | undefined
}) {
  const { t } = useTranslation()
  if (!latest) {
    return <Badge variant="muted">{t('lms.assignment.status.none')}</Badge>
  }
  if (latest.status === 'DRAFT') {
    return <Badge variant="info">{t('lms.assignment.status.draft')}</Badge>
  }
  return latest.is_late ? (
    <Badge variant="warning">{t('lms.assignment.status.late')}</Badge>
  ) : (
    <Badge variant="success">{t('lms.assignment.status.submitted')}</Badge>
  )
}

/** DueText, son tarihi göreli zamanla yazar; geçtiyse kırmızı. */
export function DueText({ due, now = new Date() }: { due: string | null; now?: Date }) {
  const { t } = useTranslation()
  if (!due) {
    return <>{t('lms.assignment.noDue')}</>
  }
  const past = new Date(due).getTime() < now.getTime()
  return (
    <>
      {formatDateTime(due)}{' '}
      <span
        className={cn(
          'text-xs',
          past ? 'font-semibold text-rose-600 dark:text-rose-400' : 'text-muted-foreground',
        )}
      >
        ({formatRelative(due, now)})
      </span>
    </>
  )
}

/** GradeSummary, puanı, geç kesintisini ve geri bildirimi gösterir. */
export function GradeSummary({ grade }: { grade: AssignmentGrade }) {
  const { t } = useTranslation()
  return (
    <div className="space-y-3">
      {grade.score !== null && (
        <p className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
          <span className="text-3xl font-extrabold text-emerald-600 tabular-nums dark:text-emerald-400">
            {formatNumber(grade.final_score ?? grade.score)}
          </span>
          <span className="text-sm text-muted-foreground">/ {formatNumber(grade.max_score)}</span>
          {grade.late_penalty_pct > 0 && (
            <Badge variant="warning">
              {t('lms.assignment.grade.penalty', {
                pct: formatNumber(grade.late_penalty_pct),
                score: formatNumber(grade.score),
              })}
            </Badge>
          )}
        </p>
      )}
      {grade.feedback_html ? (
        <RichText html={grade.feedback_html} />
      ) : (
        <p className="text-sm text-muted-foreground">{t('lms.assignment.grade.noFeedback')}</p>
      )}
      <p className="text-xs text-muted-foreground">
        {t('lms.assignment.grade.gradedBy', { name: grade.graded_by, time: formatDateTime(grade.graded_at) })}
      </p>
    </div>
  )
}

/** AttemptView, bir denemenin dosyalarını ve metnini gösterir. */
export function AttemptView({
  attempt,
  onDownload,
}: {
  attempt: Submission
  onDownload: (fileId: string) => void
}) {
  const { t } = useTranslation()
  const locale = useLocale()
  const empty = attempt.files.length === 0 && !attempt.online_text_html
  return (
    <div className="space-y-3">
      {attempt.files.length > 0 && (
        <ul className="divide-y rounded-xl border">
          {attempt.files.map((f) => (
            <li key={f.id} className="flex items-center gap-3 px-3 py-2">
              <FileIcon className="size-4 shrink-0 text-sky-600" aria-hidden />
              <span className="min-w-0 flex-1">
                <span className="block truncate text-sm font-medium">{f.name}</span>
                <span className="text-xs text-muted-foreground">{formatBytes(f.size, locale)}</span>
              </span>
              <Button
                size="sm"
                variant="ghost"
                onClick={() => onDownload(f.id)}
                aria-label={t('lms.assignment.downloadFile', { name: f.name })}
              >
                <DownloadIcon />
              </Button>
            </li>
          ))}
        </ul>
      )}
      {attempt.online_text_html && (
        <div className="rounded-xl border bg-muted/20 p-3">
          <RichText html={attempt.online_text_html} />
        </div>
      )}
      {empty && <p className="text-sm text-muted-foreground">{t('lms.assignment.work.noContent')}</p>}
      {attempt.submitted_at && (
        <p className="text-xs text-muted-foreground">
          {t('lms.assignment.work.submittedAt', { time: formatDateTime(attempt.submitted_at) })}
          {attempt.is_late && ` · ${t('lms.assignment.status.late')}`}
        </p>
      )}
    </div>
  )
}

/** CommentThread, gönderim yorumlarıdır: liste ve yeni yorum. */
export function CommentThread({
  comments,
  onSend,
  disabled = false,
}: {
  comments: SubmissionComment[]
  onSend: (body: string) => Promise<boolean>
  disabled?: boolean
}) {
  const { t } = useTranslation()
  const [body, setBody] = useState('')
  const [busy, setBusy] = useState(false)
  const send = async () => {
    if (!body.trim()) {
      return
    }
    setBusy(true)
    if (await onSend(body.trim())) {
      setBody('')
    }
    setBusy(false)
  }
  return (
    <div className="space-y-3">
      {comments.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t('lms.assignment.comments.empty')}</p>
      ) : (
        <ul className="space-y-2">
          {comments.map((c) => (
            <li key={c.id} className="rounded-xl bg-muted/40 px-3 py-2">
              <p className="flex flex-wrap items-baseline gap-x-2 text-xs">
                <span className="font-semibold">{c.author_name}</span>
                <span className="text-muted-foreground">{formatDateTime(c.created_at)}</span>
              </p>
              <p className="text-sm whitespace-pre-wrap">{c.body}</p>
            </li>
          ))}
        </ul>
      )}
      {!disabled && (
        <div className="flex items-end gap-2">
          <Textarea
            aria-label={t('lms.assignment.comments.label')}
            placeholder={t('lms.assignment.comments.placeholder')}
            value={body}
            maxLength={2000}
            rows={2}
            onChange={(e) => setBody(e.target.value)}
            className="min-h-0"
          />
          <Button
            onClick={() => void send()}
            disabled={busy || !body.trim()}
            aria-label={t('lms.assignment.comments.send')}
          >
            <SendIcon />
          </Button>
        </div>
      )}
    </div>
  )
}
