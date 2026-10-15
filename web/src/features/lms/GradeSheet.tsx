import { AwardIcon, MessageSquareIcon, SaveIcon, UploadCloudIcon } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { formatNumber } from '@/i18n/format'
import { ifMatch } from '@/shared/api/etag'
import {
  useAddSubmissionCommentMutation,
  useGetStudentSubmissionQuery,
  useGradeSubmissionMutation,
  type StudentWork,
} from '@/shared/api/generated'
import { useErrorMessage } from '@/shared/api/useErrorMessage'
import { Button } from '@/shared/ui/button'
import { IconChip } from '@/shared/ui/icon-chip'
import { Input } from '@/shared/ui/input'
import { Label } from '@/shared/ui/label'
import { RichTextEditor } from '@/shared/ui/rich-text-editor'
import { Sheet, SheetContent, SheetDescription, SheetTitle } from '@/shared/ui/sheet'
import { Skeleton } from '@/shared/ui/skeleton'
import { ErrorState } from '@/shared/ui/states'

import { AttemptView, CommentThread, GradeSummary, SubmissionStatus } from './assignmentParts'
import { useSubmissionDownload } from './download'

/** parseScore, yazılan puanı okur: boşsa puansız (null), geçersizse undefined. */
function parseScore(text: string, max: number): number | null | undefined {
  const s = text.trim()
  if (s === '') {
    return null
  }
  if (!/^\d{1,4}([.,]\d{1,2})?$/.test(s)) {
    return undefined
  }
  const v = Number(s.replace(',', '.'))
  return v <= max ? v : undefined
}

/**
 * GradeSheet, bir öğrencinin teslimini yandan açılan panelde gösterir: denemeler (dosyalar ve
 * metin), puan ve geri bildirim formu, gönderim yorumları. İzleyici salt okunur görür.
 */
export function GradeSheet({
  activityId,
  userId,
  onClose,
}: {
  activityId: string
  userId: string
  onClose: () => void
}) {
  const { t } = useTranslation()
  const { data, error, refetch } = useGetStudentSubmissionQuery({ id: activityId, userId })
  return (
    <Sheet open onOpenChange={(open) => !open && onClose()}>
      <SheetContent
        side="right"
        closeLabel={t('common.close')}
        className="w-full overflow-y-auto bg-background p-5 sm:max-w-2xl"
      >
        {error ? (
          <ErrorState error={error} onRetry={() => void refetch()} />
        ) : !data ? (
          <>
            <SheetTitle className="sr-only">{t('common.loading')}</SheetTitle>
            <Skeleton className="h-64" />
          </>
        ) : (
          <SheetBody activityId={activityId} work={data} />
        )}
      </SheetContent>
    </Sheet>
  )
}

function SheetBody({ activityId, work }: { activityId: string; work: StudentWork }) {
  const { t } = useTranslation()
  const message = useErrorMessage()
  const download = useSubmissionDownload(activityId, work.student.user_id)
  const [comment] = useAddSubmissionCommentMutation()
  const latest = work.attempts.at(-1)
  const attempts = [...work.attempts].reverse()
  return (
    <div className="space-y-6 pr-6">
      <header className="space-y-1">
        <SheetTitle className="text-xl font-extrabold">
          {work.student.first_name} {work.student.last_name}
        </SheetTitle>
        <SheetDescription className="flex flex-wrap items-center gap-2">
          <span className="font-mono">{work.student.student_no}</span>
          <SubmissionStatus latest={latest} />
        </SheetDescription>
      </header>

      <section className="space-y-3" aria-labelledby="sheet-attempts">
        <h3 id="sheet-attempts" className="flex items-center gap-2 text-sm font-bold">
          <IconChip icon={UploadCloudIcon} tone="sky" size="sm" />
          {t('lms.assignment.sheet.attempts')}
        </h3>
        {attempts.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t('lms.assignment.sheet.noAttempts')}</p>
        ) : (
          attempts.map((a) => (
            <div key={a.id} className="space-y-2">
              <p className="text-xs font-bold text-muted-foreground uppercase">
                {t('lms.assignment.work.attemptN', { n: a.attempt_no })}
                {a.status === 'DRAFT' && ` · ${t('lms.assignment.status.draft')}`}
              </p>
              <AttemptView attempt={a} onDownload={(f) => void download(f)} />
            </div>
          ))
        )}
      </section>

      <section className="space-y-3" aria-labelledby="sheet-grade">
        <h3 id="sheet-grade" className="flex items-center gap-2 text-sm font-bold">
          <IconChip icon={AwardIcon} tone="emerald" size="sm" />
          {t('lms.assignment.grade.title')}
        </h3>
        {work.can_grade ? (
          <GradeForm key={work.grade?.version ?? 0} activityId={activityId} work={work} />
        ) : work.grade ? (
          <GradeSummary grade={work.grade} />
        ) : (
          <p className="text-sm text-muted-foreground">{t('lms.assignment.status.notGraded')}</p>
        )}
      </section>

      <section className="space-y-3" aria-labelledby="sheet-comments">
        <h3 id="sheet-comments" className="flex items-center gap-2 text-sm font-bold">
          <IconChip icon={MessageSquareIcon} tone="violet" size="sm" />
          {t('lms.assignment.comments.title')}
        </h3>
        <CommentThread
          comments={work.comments}
          disabled={!work.can_grade}
          onSend={async (body) => {
            const res = await comment({
              id: activityId,
              userId: work.student.user_id,
              commentInput: { body },
            })
            if (res.error) {
              toast.error(message(res.error))
              return false
            }
            return true
          }}
        />
      </section>
    </div>
  )
}

function GradeForm({ activityId, work }: { activityId: string; work: StudentWork }) {
  const { t } = useTranslation()
  const message = useErrorMessage()
  const [grade, saving] = useGradeSubmissionMutation()
  const max = work.assignment.max_score
  const [score, setScore] = useState(
    work.grade?.score != null ? String(work.grade.score).replace('.', ',') : '',
  )
  const [feedback, setFeedback] = useState(work.grade?.feedback_html ?? '')
  const [error, setError] = useState<string | null>(null)
  const parsed = parseScore(score, max)

  const save = async () => {
    if (parsed === undefined) {
      setError(t('lms.assignment.sheet.scoreInvalid', { max: formatNumber(max) }))
      return
    }
    setError(null)
    const res = await grade({
      id: activityId,
      userId: work.student.user_id,
      'If-Match': work.grade ? ifMatch(work.grade.version) : undefined,
      gradeInput: { score: parsed, feedback_html: feedback },
    })
    if (res.error) {
      setError(message(res.error))
      return
    }
    toast.success(t('lms.assignment.sheet.saved'))
  }

  return (
    <div className="space-y-3">
      {work.grade && work.grade.late_penalty_pct > 0 && work.grade.final_score != null && (
        <p className="rounded-lg bg-amber-500/10 px-3 py-2 text-sm text-amber-800 dark:text-amber-300">
          {t('lms.assignment.sheet.penaltyNote', {
            pct: formatNumber(work.grade.late_penalty_pct),
            final: formatNumber(work.grade.final_score),
          })}
        </p>
      )}
      <div className="max-w-40 space-y-1.5">
        <Label htmlFor="grade-score">
          {t('lms.assignment.sheet.scoreLabel', { max: formatNumber(max) })}
        </Label>
        <Input
          id="grade-score"
          inputMode="decimal"
          value={score}
          aria-invalid={parsed === undefined}
          onChange={(e) => setScore(e.target.value)}
        />
      </div>
      <div className="space-y-1.5">
        <span className="text-sm font-medium">{t('lms.assignment.sheet.feedbackLabel')}</span>
        <RichTextEditor
          value={feedback}
          onChange={setFeedback}
          label={t('lms.assignment.sheet.feedbackLabel')}
          minHeight="min-h-28"
        />
      </div>
      {error && (
        <p role="alert" className="text-sm text-rose-700 dark:text-rose-300">
          {error}
        </p>
      )}
      <div className="flex justify-end">
        <Button onClick={() => void save()} disabled={saving.isLoading}>
          <SaveIcon />
          {t('lms.assignment.sheet.save')}
        </Button>
      </div>
    </div>
  )
}
