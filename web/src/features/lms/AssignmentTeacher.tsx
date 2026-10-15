import {
  AlarmClockIcon,
  CheckCheckIcon,
  CircleDashedIcon,
  FilePenLineIcon,
  HourglassIcon,
  MegaphoneIcon,
  PencilLineIcon,
  UsersRoundIcon,
} from 'lucide-react'
import { lazy, Suspense, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { formatDateTime, formatNumber } from '@/i18n/format'
import {
  useListSubmissionsQuery,
  useReleaseAssignmentGradesMutation,
  type SubmissionRosterRow,
} from '@/shared/api/generated'
import { useErrorMessage } from '@/shared/api/useErrorMessage'
import { Badge } from '@/shared/ui/badge'
import { Button } from '@/shared/ui/button'
import { ConfirmDialog } from '@/shared/ui/confirm-dialog'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/shared/ui/select'
import { Skeleton } from '@/shared/ui/skeleton'
import { StatTile } from '@/shared/ui/stat-tile'
import { EmptyState, ErrorState } from '@/shared/ui/states'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/shared/ui/table'

import { DueText, SubmissionStatus } from './assignmentParts'
import { useAcceptsText, usePolicyText } from './assignmentText'

// Puanlama paneli zengin metin editörüyle gelir: sadece açılınca indirilir.
const GradeSheet = lazy(() => import('./GradeSheet').then((m) => ({ default: m.GradeSheet })))

type Filter = 'all' | 'needs' | 'submitted' | 'notStarted'

const matches = (row: SubmissionRosterRow, f: Filter) =>
  f === 'all' ||
  (f === 'needs' && row.needs_grading) ||
  (f === 'submitted' && row.latest?.status === 'SUBMITTED') ||
  (f === 'notStarted' && !row.latest)

/**
 * AssignmentTeacher, öğretim elemanının ödev ekranıdır: teslim özeti, öğrenci listesi (teslim
 * etmeyenler dahil), puanlama paneli ve puanların yayımlanması. İzleyici salt okunur görür.
 */
export function AssignmentTeacher({ activityId }: { activityId: string }) {
  const { t } = useTranslation()
  const message = useErrorMessage()
  const acceptsText = useAcceptsText()
  const policyText = usePolicyText()
  const { data, error, refetch } = useListSubmissionsQuery({ id: activityId })
  const [release] = useReleaseAssignmentGradesMutation()
  const [filter, setFilter] = useState<Filter>('all')
  const [selected, setSelected] = useState<string | null>(null)
  const [confirming, setConfirming] = useState(false)

  if (error) {
    return <ErrorState error={error} onRetry={() => void refetch()} />
  }
  if (!data) {
    return <Skeleton className="h-64" />
  }
  const s = data.summary
  const rows = data.items.filter((r) => matches(r, filter))
  const max = data.assignment.max_score

  return (
    <div className="space-y-4">
      <p className="flex flex-wrap gap-x-4 gap-y-1 rounded-xl border border-rose-200/70 bg-rose-50/40 px-4 py-2.5 text-sm dark:border-rose-500/20 dark:bg-rose-500/5">
        <span>
          <span className="font-semibold">{t('lms.assignment.status.dueAt')}:</span>{' '}
          <DueText due={data.assignment.due_at} />
        </span>
        <span>
          <span className="font-semibold">{t('lms.assignment.status.accepts')}:</span>{' '}
          {acceptsText(data.assignment)}
        </span>
        <span>
          <span className="font-semibold">{t('lms.assignment.status.attempt')}:</span>{' '}
          {data.assignment.max_attempts} ·{' '}
          <span className="font-semibold">{t('lms.assignment.form.maxScore')}:</span> {formatNumber(max)}
        </span>
        <span className="text-muted-foreground">{policyText(data.assignment)}</span>
      </p>
      <dl className="grid grid-cols-2 gap-3 sm:grid-cols-3 xl:grid-cols-6">
        <StatTile
          icon={UsersRoundIcon}
          tone="indigo"
          label={t('lms.assignment.teacher.students')}
          value={s.students}
        />
        <StatTile
          icon={CheckCheckIcon}
          tone="emerald"
          label={t('lms.assignment.teacher.submitted')}
          value={s.submitted}
        />
        <StatTile
          icon={FilePenLineIcon}
          tone="sky"
          label={t('lms.assignment.teacher.drafts')}
          value={s.drafts}
        />
        <StatTile
          icon={CircleDashedIcon}
          tone="slate"
          label={t('lms.assignment.teacher.notStarted')}
          value={s.not_started}
        />
        <StatTile
          icon={AlarmClockIcon}
          tone="amber"
          label={t('lms.assignment.teacher.late')}
          value={s.late}
        />
        <StatTile
          icon={HourglassIcon}
          tone="rose"
          label={t('lms.assignment.teacher.needsGrading')}
          value={s.needs_grading}
        />
      </dl>

      <div className="flex flex-wrap items-center gap-3">
        <Select value={filter} onValueChange={(v) => setFilter(v as Filter)}>
          <SelectTrigger aria-label={t('lms.assignment.teacher.filter')} className="min-w-56">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {(['all', 'needs', 'submitted', 'notStarted'] as const).map((f) => (
              <SelectItem key={f} value={f}>
                {t(`lms.assignment.teacher.filters.${f}`)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <span className="flex-1" />
        {data.assignment.grades_released_at ? (
          <Badge variant="success" className="gap-1">
            <MegaphoneIcon className="size-3" />
            {t('lms.assignment.teacher.released', {
              time: formatDateTime(data.assignment.grades_released_at),
            })}
          </Badge>
        ) : (
          data.can_grade && (
            <Button variant="outline" onClick={() => setConfirming(true)}>
              <MegaphoneIcon />
              {t('lms.assignment.teacher.release')}
            </Button>
          )
        )}
      </div>

      {data.items.length === 0 ? (
        <EmptyState icon={UsersRoundIcon} tone="indigo">
          {t('lms.assignment.teacher.empty')}
        </EmptyState>
      ) : rows.length === 0 ? (
        <p className="py-6 text-center text-sm text-muted-foreground">
          {t('lms.assignment.teacher.noMatch')}
        </p>
      ) : (
        <div className="overflow-x-auto rounded-2xl border border-border/70 bg-card shadow-xs">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('lms.assignment.teacher.columns.student')}</TableHead>
                <TableHead>{t('lms.assignment.teacher.columns.status')}</TableHead>
                <TableHead>{t('lms.assignment.teacher.columns.submittedAt')}</TableHead>
                <TableHead className="text-right">{t('lms.assignment.teacher.columns.files')}</TableHead>
                <TableHead className="text-right">{t('lms.assignment.teacher.columns.score')}</TableHead>
                <TableHead className="text-right">{t('lms.assignment.teacher.columns.comments')}</TableHead>
                <TableHead>
                  <span className="sr-only">{t('common.actions')}</span>
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {rows.map((r) => {
                const name = `${r.student.first_name} ${r.student.last_name}`
                return (
                  <TableRow key={r.student.user_id}>
                    <TableCell>
                      <span className="block font-semibold">{name}</span>
                      <span className="font-mono text-xs text-muted-foreground">{r.student.student_no}</span>
                    </TableCell>
                    <TableCell>
                      <span className="flex flex-wrap items-center gap-1">
                        <SubmissionStatus latest={r.latest} />
                        {r.needs_grading && (
                          <Badge variant="destructive">{t('lms.assignment.teacher.needsBadge')}</Badge>
                        )}
                      </span>
                    </TableCell>
                    <TableCell className="text-sm whitespace-nowrap">
                      {r.latest?.submitted_at ? formatDateTime(r.latest.submitted_at) : '—'}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">{r.latest?.file_count ?? '—'}</TableCell>
                    <TableCell className="text-right font-semibold whitespace-nowrap tabular-nums">
                      {r.grade?.score != null
                        ? `${formatNumber(r.grade.final_score ?? r.grade.score)} / ${formatNumber(max)}`
                        : '—'}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">{r.comments || '—'}</TableCell>
                    <TableCell className="text-right">
                      <Button
                        size="sm"
                        variant={r.needs_grading ? 'default' : 'soft'}
                        aria-label={t(
                          data.can_grade
                            ? 'lms.assignment.teacher.gradeFor'
                            : 'lms.assignment.teacher.viewFor',
                          { name },
                        )}
                        onClick={() => setSelected(r.student.user_id)}
                      >
                        <PencilLineIcon />
                        {data.can_grade
                          ? t('lms.assignment.teacher.grade')
                          : t('lms.assignment.teacher.view')}
                      </Button>
                    </TableCell>
                  </TableRow>
                )
              })}
            </TableBody>
          </Table>
        </div>
      )}

      {selected && (
        <Suspense fallback={null}>
          <GradeSheet activityId={activityId} userId={selected} onClose={() => setSelected(null)} />
        </Suspense>
      )}
      {confirming && (
        <ConfirmDialog
          title={t('lms.assignment.teacher.releaseTitle')}
          description={t('lms.assignment.teacher.releaseDescription', { count: s.needs_grading })}
          confirm={t('lms.assignment.teacher.release')}
          icon={MegaphoneIcon}
          tone="emerald"
          onClose={() => setConfirming(false)}
          onConfirm={async () => {
            const res = await release({ id: activityId })
            if (res.error) {
              toast.error(message(res.error))
              return
            }
            setConfirming(false)
            toast.success(t('lms.assignment.teacher.releasedToast'))
          }}
        />
      )}
    </div>
  )
}
