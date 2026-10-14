import {
  AwardIcon,
  BarChart3Icon,
  CircleAlertIcon,
  MegaphoneIcon,
  PencilLineIcon,
  RotateCcwIcon,
  SigmaIcon,
  TableIcon,
  TrendingUpIcon,
  UsersIcon,
} from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useParams } from 'react-router'
import { toast } from 'sonner'

import { navTheme } from '@/features/shell/nav'
import { formatDateTime } from '@/i18n/format'
import {
  useGetGradebookQuery,
  useGetLetterGradesQuery,
  usePublishLetterGradesMutation,
  usePublishMakeupMutation,
  useSetLetterOverridesMutation,
  type LetterGrades,
} from '@/shared/api/generated'
import { useErrorMessage } from '@/shared/api/useErrorMessage'
import { useLocalized } from '@/shared/lib/localized'
import { cn } from '@/shared/lib/utils'
import { Alert, AlertDescription } from '@/shared/ui/alert'
import { Badge } from '@/shared/ui/badge'
import { Button } from '@/shared/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/shared/ui/card'
import { ConfirmDialog } from '@/shared/ui/confirm-dialog'
import { FormDialog } from '@/shared/ui/form-dialog'
import { IconChip } from '@/shared/ui/icon-chip'
import { Label } from '@/shared/ui/label'
import { LetterDistributionChart } from '@/shared/ui/letter-distribution'
import { LetterGrade } from '@/shared/ui/letter-grade'
import { PageHeader } from '@/shared/ui/page-header'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/shared/ui/select'
import { Skeleton } from '@/shared/ui/skeleton'
import { StatTile } from '@/shared/ui/stat-tile'
import { EmptyState, ErrorState } from '@/shared/ui/states'
import { Textarea } from '@/shared/ui/textarea'

type Student = LetterGrades['students'][number]

const reasonVariant = {
  RANGE: 'muted',
  PASS_FAIL: 'muted',
  MANUAL: 'violet',
  ATTENDANCE_FAIL: 'destructive',
  FINAL_ABSENT: 'destructive',
  FINAL_BELOW_MIN: 'warning',
} as const

/**
 * LetterGradesPage, şubenin harf notlarıdır: ilandan önce önizleme (ders başarı puanı,
 * hesaplanan harf ve nedeni, eksik bileşenler, sınıf istatistikleri ve dağılım), elle harf
 * notu ve ilan; ilandan sonra bütünleme hakkı olanlar, bütünlemeyle olacak harfler ve
 * bütünleme ilanı. İlan ve elle harf sorumlu öğretim elemanınındır (MFA).
 */
export function LetterGradesPage() {
  const { t, i18n } = useTranslation()
  const { id = '' } = useParams()
  const localized = useLocalized()
  const message = useErrorMessage()
  const { data, error, refetch } = useGetLetterGradesQuery({ id })
  const book = useGetGradebookQuery({ id })
  const [publish] = usePublishLetterGradesMutation()
  const [publishMakeup] = usePublishMakeupMutation()
  const [confirm, setConfirm] = useState<'publish' | 'makeup' | null>(null)
  const [overriding, setOverriding] = useState<Student | null>(null)
  const [clearing, setClearing] = useState<Student | null>(null)
  const [setOverrides] = useSetLetterOverridesMutation()
  const locale = i18n.language.startsWith('en') ? 'en-GB' : 'tr-TR'
  const num = (v: number | null, digits = 2) =>
    v === null ? '—' : v.toLocaleString(locale, { maximumFractionDigits: digits })

  if (error || book.error) {
    return <ErrorState error={error ?? book.error} onRetry={() => void refetch()} />
  }
  if (!data || !book.data) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-10 w-96" />
        <Skeleton className="h-40" />
        <Skeleton className="h-96" />
      </div>
    )
  }
  const s = book.data.section
  const published = Boolean(data.published_at)
  const incomplete = data.students.filter((st) => !st.complete && !st.override)
  const eligible = data.students.filter((st) => st.makeup_eligible)
  const makeupEntered = eligible.filter((st) => st.makeup_letter !== null)

  return (
    <>
      <PageHeader
        {...navTheme('/verdigim-dersler')}
        title={t('letters.title')}
        description={`${s.course.code}-${s.section_code} ${localized(s.course)} · ${s.term.code}`}
        actions={
          <div className="flex flex-wrap gap-2">
            <Button variant="ghost" asChild>
              <Link to={`/subeler/${id}/notlar`}>
                <TableIcon />
                {t('teaching.gradebook')}
              </Link>
            </Button>
            {data.can_publish && (
              <Button onClick={() => setConfirm('publish')}>
                <MegaphoneIcon />
                {t('letters.publish')}
              </Button>
            )}
            {data.can_publish_makeup && (
              <Button onClick={() => setConfirm('makeup')}>
                <MegaphoneIcon />
                {t('letters.publishMakeup')}
              </Button>
            )}
          </div>
        }
      />

      <div className="mb-6 space-y-3">
        {published ? (
          <Alert variant="success">
            <AwardIcon />
            <AlertDescription>
              {t('letters.publishedInfo', {
                time: formatDateTime(data.published_at),
                name: data.published_by ?? '—',
              })}
              {data.makeup_published_at &&
                ` ${t('letters.makeupPublishedInfo', { time: formatDateTime(data.makeup_published_at) })}`}
            </AlertDescription>
          </Alert>
        ) : (
          <Alert variant="info">
            <BarChart3Icon />
            <AlertDescription>{t('letters.previewInfo', { min: data.final_min_score })}</AlertDescription>
          </Alert>
        )}
        {!published && incomplete.length > 0 && (
          <Alert variant="warning">
            <CircleAlertIcon />
            <AlertDescription>{t('letters.incomplete', { count: incomplete.length })}</AlertDescription>
          </Alert>
        )}
        {!published && !data.can_publish && incomplete.length === 0 && !data.publish_window.open && (
          <Alert>
            <AlertDescription>
              {data.publish_window.next_starts_at
                ? t('letters.windowNext', { time: formatDateTime(data.publish_window.next_starts_at) })
                : t('letters.windowClosed')}
            </AlertDescription>
          </Alert>
        )}
      </div>

      <dl className="mb-6 grid grid-cols-2 gap-3 lg:grid-cols-5">
        <StatTile
          icon={UsersIcon}
          tone="indigo"
          label={t('letters.summary.students')}
          value={`${data.summary.complete}/${data.summary.students}`}
        />
        <StatTile
          icon={SigmaIcon}
          tone="violet"
          label={t('letters.summary.mean')}
          value={num(data.summary.mean)}
        />
        <StatTile
          icon={BarChart3Icon}
          tone="sky"
          label={t('letters.summary.median')}
          value={num(data.summary.median)}
        />
        <StatTile
          icon={TrendingUpIcon}
          tone="amber"
          label={t('letters.summary.stddev')}
          value={num(data.summary.stddev)}
        />
        <StatTile
          icon={AwardIcon}
          tone="emerald"
          label={t('letters.summary.passRate')}
          value={data.summary.pass_rate === null ? '—' : `%${num(data.summary.pass_rate * 100, 1)}`}
        />
      </dl>

      <div
        className={cn(
          'mb-6 grid gap-6',
          published && data.grading_mode === 'LETTER' && 'lg:grid-cols-[2fr_1fr]',
        )}
      >
        <Card>
          <CardHeader className="flex items-center gap-3">
            <IconChip icon={BarChart3Icon} tone="violet" />
            <div>
              <CardTitle>{t('letters.distribution')}</CardTitle>
              <CardDescription>{t('letters.distributionHint')}</CardDescription>
            </div>
          </CardHeader>
          <CardContent>
            <LetterDistributionChart distribution={data.distribution} />
          </CardContent>
        </Card>
        {published && data.grading_mode === 'LETTER' && (
          <Card>
            <CardHeader className="flex items-center gap-3">
              <IconChip icon={RotateCcwIcon} tone="rose" />
              <div>
                <CardTitle>{t('letters.makeup')}</CardTitle>
                <CardDescription>
                  {data.makeup_published_at
                    ? t('letters.makeupDone')
                    : data.makeup_window.open
                      ? t('letters.makeupOpen', { time: formatDateTime(data.makeup_window.ends_at) })
                      : t('letters.makeupClosed')}
                </CardDescription>
              </div>
            </CardHeader>
            <CardContent className="space-y-3 text-sm">
              <p className="tabular-nums">
                {t('letters.makeupEligible', { count: eligible.length })} ·{' '}
                {t('letters.makeupEntered', { count: makeupEntered.length })}
              </p>
              {!data.makeup_published_at && eligible.length > 0 && (
                <Button variant="soft" size="sm" asChild>
                  <Link to={`/subeler/${id}/notlar`}>
                    <TableIcon />
                    {t('letters.enterMakeup')}
                  </Link>
                </Button>
              )}
            </CardContent>
          </Card>
        )}
      </div>

      {data.students.length === 0 ? (
        <EmptyState icon={UsersIcon} tone="fuchsia">
          {t('gradebook.empty')}
        </EmptyState>
      ) : (
        <div className="overflow-x-auto rounded-2xl border border-border/70 bg-card shadow-xs">
          <table className="w-full min-w-max text-sm">
            <thead>
              <tr className="border-b bg-muted text-left">
                <th scope="col" className="px-3 py-2.5 font-semibold">
                  {t('gradebook.student')}
                </th>
                <th scope="col" className="px-3 py-2.5 text-right font-semibold">
                  {t('letters.weighted')}
                </th>
                <th scope="col" className="px-3 py-2.5 font-semibold">
                  {t('letters.computed')}
                </th>
                <th scope="col" className="px-3 py-2.5 font-semibold">
                  {t('letters.override')}
                </th>
                <th scope="col" className="px-3 py-2.5 text-center font-semibold">
                  {t('gradebook.letter')}
                </th>
                {published && (
                  <th scope="col" className="px-3 py-2.5 font-semibold">
                    {t('letters.makeup')}
                  </th>
                )}
              </tr>
            </thead>
            <tbody>
              {data.students.map((st) => {
                const name = `${st.student.first_name} ${st.student.last_name}`
                return (
                  <tr key={st.enrollment_id} className="border-b border-border/60 last:border-0">
                    <th scope="row" className="px-3 py-2 text-left font-normal">
                      <span className="block font-semibold">{name}</span>
                      <span className="font-mono text-xs text-muted-foreground">{st.student.student_no}</span>
                    </th>
                    <td className="px-3 py-2 text-right tabular-nums">
                      {st.weighted_score === null ? (
                        <span className="text-muted-foreground/60">—</span>
                      ) : (
                        <>
                          <span className="font-semibold">{num(st.weighted_score)}</span>
                          {st.used_makeup && (
                            <span className="block text-xs text-muted-foreground">
                              {t('letters.usedMakeup')}
                            </span>
                          )}
                        </>
                      )}
                    </td>
                    <td className="px-3 py-2">
                      {st.complete ? (
                        <span className="flex flex-wrap items-center gap-1.5">
                          {st.computed_letter && <LetterGrade letter={st.computed_letter} />}
                          {st.reason && st.reason !== 'RANGE' && st.reason !== 'PASS_FAIL' && (
                            <Badge variant={reasonVariant[st.reason]}>
                              {t(`letters.reasons.${st.reason}`)}
                            </Badge>
                          )}
                        </span>
                      ) : (
                        <span className="text-xs text-amber-700 dark:text-amber-300">
                          {t('letters.missing', { components: st.missing_components.join(', ') })}
                        </span>
                      )}
                    </td>
                    <td className="px-3 py-2">
                      {st.override ? (
                        <span className="flex flex-wrap items-center gap-1.5">
                          <LetterGrade letter={st.override.letter} />
                          <span
                            className="max-w-56 truncate text-xs text-muted-foreground"
                            title={st.override.reason}
                          >
                            {st.override.reason}
                          </span>
                          {data.can_override && (
                            <Button size="sm" variant="ghost" onClick={() => setClearing(st)}>
                              {t('letters.clearOverride')}
                            </Button>
                          )}
                        </span>
                      ) : data.can_override ? (
                        <Button
                          size="sm"
                          variant="ghost"
                          onClick={() => setOverriding(st)}
                          aria-label={t('letters.overrideFor', { name })}
                        >
                          <PencilLineIcon />
                          {t('letters.setOverride')}
                        </Button>
                      ) : (
                        <span className="text-muted-foreground/60">—</span>
                      )}
                    </td>
                    <td className="px-3 py-2 text-center">
                      {st.letter ? (
                        <span className="inline-flex items-center gap-1.5">
                          {st.previous_letter && (
                            <LetterGrade letter={st.previous_letter} className="line-through opacity-50" />
                          )}
                          <LetterGrade letter={st.letter} className="min-w-11 py-1 text-sm" />
                        </span>
                      ) : (
                        <span className="text-muted-foreground/60">—</span>
                      )}
                    </td>
                    {published && (
                      <td className="px-3 py-2">
                        {st.makeup_eligible ? (
                          st.makeup_letter ? (
                            <span className="flex items-center gap-1.5 text-xs">
                              {t('letters.makeupWillBe')} <LetterGrade letter={st.makeup_letter} />
                            </span>
                          ) : (
                            <Badge variant="warning">{t('letters.makeupRight')}</Badge>
                          )
                        ) : (
                          <span className="text-muted-foreground/60">—</span>
                        )}
                      </td>
                    )}
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      )}

      {confirm && (
        <ConfirmDialog
          title={confirm === 'publish' ? t('letters.publishTitle') : t('letters.publishMakeupTitle')}
          description={
            confirm === 'publish'
              ? t('letters.publishDescription', { count: data.summary.students })
              : t('letters.publishMakeupDescription', { count: makeupEntered.length })
          }
          confirm={confirm === 'publish' ? t('letters.publish') : t('letters.publishMakeup')}
          icon={MegaphoneIcon}
          tone="emerald"
          onClose={() => setConfirm(null)}
          onConfirm={async () => {
            const res = confirm === 'publish' ? await publish({ id }) : await publishMakeup({ id })
            if ('error' in res && res.error) {
              toast.error(message(res.error))
              return
            }
            toast.success(
              confirm === 'publish' ? t('letters.publishedToast') : t('letters.makeupPublishedToast'),
            )
            setConfirm(null)
          }}
        >
          {confirm === 'publish' && (
            <LetterDistributionChart distribution={data.distribution} className="h-28" />
          )}
        </ConfirmDialog>
      )}
      {overriding && (
        <OverrideDialog
          student={overriding}
          scale={data.scale}
          onClose={() => setOverriding(null)}
          onSave={async (letter, reason) => {
            const res = await setOverrides({
              id,
              body: { items: [{ enrollment_id: overriding.enrollment_id, letter, reason }] },
            })
            if ('error' in res && res.error) {
              return message(res.error)
            }
            toast.success(t('letters.overrideSaved'))
            setOverriding(null)
            return null
          }}
        />
      )}
      {clearing && (
        <ConfirmDialog
          title={t('letters.clearTitle', {
            name: `${clearing.student.first_name} ${clearing.student.last_name}`,
          })}
          description={t('letters.clearDescription')}
          confirm={t('letters.clearOverride')}
          destructive
          onClose={() => setClearing(null)}
          onConfirm={async () => {
            const res = await setOverrides({
              id,
              body: { items: [{ enrollment_id: clearing.enrollment_id, letter: null }] },
            })
            if ('error' in res && res.error) {
              toast.error(message(res.error))
              return
            }
            setClearing(null)
          }}
        />
      )}
    </>
  )
}

/** OverrideDialog, bir öğrenciye gerekçeyle elle harf notu verir. */
function OverrideDialog({
  student,
  scale,
  onClose,
  onSave,
}: {
  student: Student
  scale: LetterGrades['scale']
  onClose: () => void
  onSave: (letter: string, reason: string) => Promise<string | null>
}) {
  const { t } = useTranslation()
  const [letter, setLetter] = useState('')
  const [reason, setReason] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const name = `${student.student.first_name} ${student.student.last_name}`
  return (
    <FormDialog
      title={t('letters.overrideTitle', { name })}
      description={t('letters.overrideDescription', {
        computed: student.computed_letter ?? '—',
      })}
      icon={PencilLineIcon}
      tone="violet"
      submit={
        <Button type="submit" disabled={busy}>
          <PencilLineIcon />
          {t('letters.setOverride')}
        </Button>
      }
      error={error}
      onClose={onClose}
      onSubmit={() => {
        if (!letter) {
          setError(t('letters.chooseLetter'))
          return
        }
        if (reason.trim().length < 5) {
          setError(t('letters.reasonShort'))
          return
        }
        setBusy(true)
        void onSave(letter, reason.trim()).then((err) => {
          setError(err)
          setBusy(false)
        })
      }}
    >
      <div className="space-y-1.5">
        <Label htmlFor="override-letter">{t('letters.letter')}</Label>
        <Select value={letter} onValueChange={setLetter}>
          <SelectTrigger id="override-letter" aria-label={t('letters.letter')}>
            <SelectValue placeholder={t('letters.chooseLetter')} />
          </SelectTrigger>
          <SelectContent>
            {scale.map((g) => (
              <SelectItem key={g.letter} value={g.letter}>
                <span className={cn('font-mono font-bold', !g.passing && 'text-rose-600')}>{g.letter}</span>
                {g.min_score !== null && g.max_score !== null && (
                  <span className="text-muted-foreground">
                    {' '}
                    · {g.min_score}–{g.max_score}
                  </span>
                )}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <div className="space-y-1.5">
        <Label htmlFor="override-reason">{t('letters.reason')}</Label>
        <Textarea
          id="override-reason"
          value={reason}
          onChange={(e) => setReason(e.target.value)}
          maxLength={500}
          placeholder={t('letters.reasonHint')}
        />
      </div>
    </FormDialog>
  )
}
