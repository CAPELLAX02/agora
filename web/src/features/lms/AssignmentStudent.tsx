import {
  AwardIcon,
  ClipboardListIcon,
  CopyPlusIcon,
  InfoIcon,
  MessageSquareIcon,
  SaveIcon,
  SendIcon,
  UploadCloudIcon,
} from 'lucide-react'
import { lazy, Suspense, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { formatDateTime } from '@/i18n/format'
import { ifMatch } from '@/shared/api/etag'
import {
  useAddMySubmissionCommentMutation,
  useGetMySubmissionQuery,
  useSaveMySubmissionMutation,
  useStartNewAttemptMutation,
  useSubmitMySubmissionMutation,
  type MySubmission,
} from '@/shared/api/generated'
import { useErrorMessage } from '@/shared/api/useErrorMessage'
import { Alert, AlertDescription } from '@/shared/ui/alert'
import { Button } from '@/shared/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/shared/ui/card'
import { ConfirmDialog } from '@/shared/ui/confirm-dialog'
import { DescriptionList } from '@/shared/ui/description-list'
import { IconChip } from '@/shared/ui/icon-chip'
import { Skeleton } from '@/shared/ui/skeleton'
import { ErrorState } from '@/shared/ui/states'

import { AttemptView, CommentThread, DueText, GradeSummary, SubmissionStatus } from './assignmentParts'
import { useAcceptsText, usePolicyText } from './assignmentText'
import { useSubmissionDownload } from './download'
import { FileDropzone, type UploadedFile } from './FileDropzone'

// Zengin metin editörü sadece çevrimiçi metin isteyen ödevde ve taslak düzenlenirken indirilir.
const RichTextEditor = lazy(() =>
  import('@/shared/ui/rich-text-editor').then((m) => ({ default: m.RichTextEditor })),
)

/** useAssignmentErrors, ödev işlemlerinin hata kodlarını dilin metnine çevirir. */
function useAssignmentErrors() {
  const { t } = useTranslation()
  const message = useErrorMessage()
  return (e: unknown) =>
    message(e as never, {
      SUBMISSION_NOT_OPEN: t('lms.assignment.errors.SUBMISSION_NOT_OPEN'),
      SUBMISSION_CLOSED: t('lms.assignment.errors.SUBMISSION_CLOSED'),
      ALREADY_SUBMITTED: t('lms.assignment.errors.ALREADY_SUBMITTED'),
      EMPTY_SUBMISSION: t('lms.assignment.errors.EMPTY_SUBMISSION'),
      NO_ATTEMPTS_LEFT: t('lms.assignment.errors.NO_ATTEMPTS_LEFT'),
      VERSION_MISMATCH: t('lms.assignment.errors.VERSION_MISMATCH'),
    })
}

/**
 * AssignmentStudent, öğrencinin ödev ekranıdır: teslim durumu tablosu, taslak (dosya ve metin),
 * gönderme ve yeni deneme, yayımlanmışsa puan ve geri bildirim, gönderim yorumları.
 */
export function AssignmentStudent({ activityId }: { activityId: string }) {
  const { data, error, refetch } = useGetMySubmissionQuery({ id: activityId })
  if (error) {
    return <ErrorState error={error} onRetry={() => void refetch()} />
  }
  if (!data) {
    return <Skeleton className="h-64" />
  }
  const latest = data.attempts.at(-1)
  // Yeni deneme başlayınca ya da deneme gönderilince taslak alanları sunucudaki hâliyle başlar.
  return (
    <StudentView key={`${latest?.id ?? 'new'}-${latest?.status ?? ''}`} activityId={activityId} data={data} />
  )
}

function StudentView({ activityId, data }: { activityId: string; data: MySubmission }) {
  const { t } = useTranslation()
  const errorText = useAssignmentErrors()
  const acceptsText = useAcceptsText()
  const policyText = usePolicyText()
  const download = useSubmissionDownload(activityId)
  const [save, saving] = useSaveMySubmissionMutation()
  const [submit, submitting] = useSubmitMySubmissionMutation()
  const [startAttempt, starting] = useStartNewAttemptMutation()
  const [addComment] = useAddMySubmissionCommentMutation()
  const a = data.assignment
  const latest = data.attempts.at(-1)
  const draft = latest?.status === 'DRAFT' ? latest : undefined
  const [files, setFiles] = useState<UploadedFile[]>(draft?.files ?? [])
  const [text, setText] = useState(draft?.online_text_html ?? '')
  const [confirming, setConfirming] = useState(false)
  const acceptsFile = a.submission_types.includes('FILE')
  const acceptsText_ = a.submission_types.includes('ONLINE_TEXT')
  const dirty =
    text !== (draft?.online_text_html ?? '') ||
    files.map((f) => f.id).join() !== (draft?.files ?? []).map((f) => f.id).join()
  const hasContent = (acceptsFile && files.length > 0) || (acceptsText_ && text.trim() !== '')

  // saveDraft, taslağı yazar ve yeni sürümünü döndürür; hata olursa null.
  const saveDraft = async (): Promise<number | null> => {
    const res = await save({
      id: activityId,
      'If-Match': latest ? ifMatch(latest.version) : undefined,
      body: {
        online_text_html: acceptsText_ ? text : '',
        file_ids: acceptsFile ? files.map((f) => f.id) : [],
      },
    })
    if (res.error) {
      toast.error(errorText(res.error))
      return null
    }
    return res.data.attempts.at(-1)?.version ?? null
  }

  const doSubmit = async () => {
    const version = dirty || !latest ? await saveDraft() : latest.version
    if (version === null) {
      return
    }
    const res = await submit({ id: activityId, 'If-Match': ifMatch(version) })
    if (res.error) {
      toast.error(errorText(res.error))
      return
    }
    setConfirming(false)
    toast.success(t('lms.assignment.work.submitted'))
  }

  const graded = data.grade
    ? t('lms.assignment.status.graded')
    : a.grades_released_at
      ? t('lms.assignment.status.notGraded')
      : t('lms.assignment.status.notReleased')

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader className="flex items-center gap-3">
          <IconChip icon={ClipboardListIcon} tone="rose" />
          <CardTitle>{t('lms.assignment.status.title')}</CardTitle>
        </CardHeader>
        <CardContent>
          <DescriptionList
            items={[
              [
                t('lms.assignment.status.attempt'),
                t('lms.assignment.status.attemptOf', {
                  n: Math.max(1, data.attempts.length),
                  max: a.max_attempts,
                }),
              ],
              [t('lms.assignment.status.submission'), <SubmissionStatus key="s" latest={latest} />],
              [t('lms.assignment.status.grading'), graded],
              [t('lms.assignment.status.dueAt'), <DueText key="d" due={a.due_at} />],
              ...(a.cutoff_at
                ? ([[t('lms.assignment.status.cutoffAt'), formatDateTime(a.cutoff_at)]] as [string, string][])
                : []),
              ...(a.opens_at && a.submission_state === 'NOT_OPEN'
                ? ([[t('lms.assignment.status.opensAt'), formatDateTime(a.opens_at)]] as [string, string][])
                : []),
              [t('lms.assignment.status.lastModified'), latest ? formatDateTime(latest.updated_at) : '—'],
              [t('lms.assignment.status.accepts'), acceptsText(a)],
              [t('lms.assignment.status.policy'), policyText(a)],
            ]}
          />
        </CardContent>
      </Card>

      {data.grade && (
        <Card>
          <CardHeader className="flex items-center gap-3">
            <IconChip icon={AwardIcon} tone="emerald" />
            <CardTitle>{t('lms.assignment.grade.title')}</CardTitle>
          </CardHeader>
          <CardContent>
            <GradeSummary grade={data.grade} />
          </CardContent>
        </Card>
      )}

      <Card>
        <CardHeader className="flex items-center gap-3">
          <IconChip icon={UploadCloudIcon} tone="sky" />
          <CardTitle className="flex-1">{t('lms.assignment.work.title')}</CardTitle>
          {data.attempts.length > 0 && (
            <span className="text-xs font-semibold text-muted-foreground">
              {t('lms.assignment.work.attemptN', { n: data.attempts.length })}
            </span>
          )}
        </CardHeader>
        <CardContent className="space-y-4">
          {a.submission_state !== 'OPEN' && (
            <Alert>
              <InfoIcon />
              <AlertDescription>
                {a.submission_state === 'NOT_OPEN'
                  ? t('lms.assignment.state.NOT_OPEN', { time: formatDateTime(a.opens_at) })
                  : t('lms.assignment.state.CLOSED')}
              </AlertDescription>
            </Alert>
          )}
          {data.can_edit ? (
            <>
              {acceptsFile && (
                <FileDropzone
                  files={files}
                  onChange={setFiles}
                  multiple={a.max_files > 1}
                  max={a.max_files}
                  accept={a.allowed_extensions.length > 0 ? a.allowed_extensions.join(',') : undefined}
                  hint={acceptsText(a)}
                />
              )}
              {acceptsText_ && (
                <div className="space-y-1.5">
                  <span className="text-sm font-medium">{t('lms.assignment.types.ONLINE_TEXT')}</span>
                  <Suspense fallback={<Skeleton className="h-40" />}>
                    <RichTextEditor
                      value={text}
                      onChange={setText}
                      label={t('lms.assignment.types.ONLINE_TEXT')}
                    />
                  </Suspense>
                </div>
              )}
              {dirty && (
                <p className="text-xs text-amber-700 dark:text-amber-300">
                  {t('lms.assignment.work.unsaved')}
                </p>
              )}
              <div className="flex flex-wrap justify-end gap-2">
                <Button
                  variant="outline"
                  disabled={!dirty || saving.isLoading}
                  onClick={() =>
                    void saveDraft().then((v) => v !== null && toast.success(t('lms.assignment.work.saved')))
                  }
                >
                  <SaveIcon />
                  {t('lms.assignment.work.saveDraft')}
                </Button>
                <Button disabled={!hasContent} onClick={() => setConfirming(true)}>
                  <SendIcon />
                  {t('lms.assignment.work.submit')}
                </Button>
              </div>
              {!hasContent && (
                <p className="text-right text-xs text-muted-foreground">{t('lms.assignment.work.empty')}</p>
              )}
            </>
          ) : latest ? (
            <AttemptView attempt={latest} onDownload={(f) => void download(f)} />
          ) : (
            <p className="text-sm text-muted-foreground">{t('lms.assignment.status.none')}</p>
          )}
          {data.can_new_attempt && (
            <div className="flex justify-end">
              <Button
                variant="soft"
                disabled={starting.isLoading}
                onClick={() =>
                  void startAttempt({ id: activityId }).then((res) =>
                    res.error
                      ? toast.error(errorText(res.error))
                      : toast.success(t('lms.assignment.work.newAttemptStarted')),
                  )
                }
              >
                <CopyPlusIcon />
                {t('lms.assignment.work.newAttempt')}
              </Button>
            </div>
          )}
          {data.attempts.length > 1 && (
            <details className="rounded-xl border px-3 py-2">
              <summary className="cursor-pointer text-sm font-semibold">
                {t('lms.assignment.work.previous')}
              </summary>
              <div className="mt-3 space-y-4">
                {data.attempts.slice(0, -1).map((at) => (
                  <section
                    key={at.id}
                    aria-label={t('lms.assignment.work.attemptN', { n: at.attempt_no })}
                    className="space-y-2"
                  >
                    <h3 className="text-xs font-bold text-muted-foreground uppercase">
                      {t('lms.assignment.work.attemptN', { n: at.attempt_no })}
                    </h3>
                    <AttemptView attempt={at} onDownload={(f) => void download(f)} />
                  </section>
                ))}
              </div>
            </details>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="flex items-center gap-3">
          <IconChip icon={MessageSquareIcon} tone="violet" />
          <CardTitle>{t('lms.assignment.comments.title')}</CardTitle>
        </CardHeader>
        <CardContent>
          <CommentThread
            comments={data.comments}
            onSend={async (body) => {
              const res = await addComment({ id: activityId, commentInput: { body } })
              if (res.error) {
                toast.error(errorText(res.error))
                return false
              }
              return true
            }}
          />
        </CardContent>
      </Card>

      {confirming && (
        <ConfirmDialog
          title={t('lms.assignment.work.submitTitle')}
          description={
            data.attempts.length < a.max_attempts
              ? t('lms.assignment.work.submitDescriptionMore', {
                  count: a.max_attempts - Math.max(1, data.attempts.length),
                })
              : t('lms.assignment.work.submitDescription')
          }
          confirm={t('lms.assignment.work.submit')}
          icon={SendIcon}
          tone="rose"
          onClose={() => setConfirming(false)}
          onConfirm={doSubmit}
        />
      )}
      {submitting.isLoading && <span className="sr-only">{t('common.loading')}</span>}
    </div>
  )
}
