import { ArrowLeftIcon, LockIcon, RefreshCcwIcon } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useParams } from 'react-router'

import { usePermissions } from '@/features/auth/usePermissions'
import { RegistrationStatusBadge } from '@/features/registration/badges'
import { HistoryTable, ProgressView } from '@/features/registration/ProgressView'
import { formatDate, formatDateTime } from '@/i18n/format'
import {
  useGetRegistrationQuery,
  useGetStudentProgressQuery,
  useGetStudentQuery,
  useListCourseHistoryQuery,
  useListStudentProgramHoldsQuery,
  useListStudentProgramStatusHistoryQuery,
  type Hold,
  type StudentProgram,
} from '@/shared/api/generated'
import { Badge } from '@/shared/ui/badge'
import { Button } from '@/shared/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/shared/ui/card'
import { PageHeader } from '@/shared/ui/page-header'
import { Skeleton } from '@/shared/ui/skeleton'
import { ErrorState } from '@/shared/ui/states'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/shared/ui/tabs'

import { EnrollmentStatusBadge, HoldTypeBadge } from './badges'
import { PlaceHoldDialog, ReleaseHoldDialog, StatusDialog } from './dialogs'

/**
 * StudentDetailPage, bir öğrencinin program kayıtlarıdır: her program için durum, süreler,
 * danışman, kayıt engelleri, dönemin ders seçimi, müfredata göre ders durumu, ders geçmişi ve
 * durum geçmişi. Yetkiye göre durum değiştirme ve engel koyma/kaldırma buradan yapılır.
 */
export function StudentDetailPage() {
  const { t } = useTranslation()
  const { id = '' } = useParams()
  const { data: st, error, refetch } = useGetStudentQuery({ id })
  if (error) {
    return <ErrorState error={error} onRetry={() => void refetch()} />
  }
  if (!st) {
    return <Skeleton className="h-96" />
  }
  return (
    <>
      <Button asChild variant="ghost" size="sm" className="mb-2 -ml-2">
        <Link to="/ogrenciler">
          <ArrowLeftIcon />
          {t('students.title')}
        </Link>
      </Button>
      <PageHeader
        title={`${st.first_name} ${st.last_name}`}
        description={[st.student_no, st.email].filter(Boolean).join(' · ')}
      />
      <div className="space-y-8">
        {st.programs.map((sp) => (
          <Program key={sp.id} program={sp} />
        ))}
      </div>
    </>
  )
}

function Program({ program: sp }: { program: StudentProgram }) {
  const { t } = useTranslation()
  const { has } = usePermissions()
  const [dialog, setDialog] = useState<'status' | 'hold' | null>(null)
  const ongoing = ['PREP', 'ACTIVE', 'FROZEN', 'SUSPENDED'].includes(sp.status)
  return (
    <section className="space-y-4" aria-label={sp.program.name_tr}>
      <Card>
        <CardHeader>
          <CardTitle className="flex flex-wrap items-center justify-between gap-2">
            <span className="flex flex-wrap items-center gap-2">
              {sp.program.name_tr}
              <EnrollmentStatusBadge status={sp.status} />
              <Badge variant="outline">{t(`curriculum.my.kinds.${sp.kind}`)}</Badge>
            </span>
            <span className="flex gap-2">
              {has('student_program:manage') && ongoing && (
                <Button size="sm" variant="outline" onClick={() => setDialog('status')}>
                  <RefreshCcwIcon />
                  {t('students.statusChange.title')}
                </Button>
              )}
              {has('hold:manage') && ongoing && (
                <Button size="sm" variant="outline" onClick={() => setDialog('hold')}>
                  <LockIcon />
                  {t('students.holds.place')}
                </Button>
              )}
            </span>
          </CardTitle>
        </CardHeader>
        <CardContent>
          <dl className="grid gap-x-8 gap-y-2 text-sm sm:grid-cols-2 lg:grid-cols-3">
            {(
              [
                [t('students.faculty'), sp.program.faculty.name_tr],
                [
                  t('students.admission'),
                  `${sp.admission_year} · ${t(`students.admissionTypes.${sp.admission_type}`)}`,
                ],
                [
                  t('students.semester'),
                  t('students.semesterOf', {
                    current: sp.current_semester,
                    normal: sp.normal_semesters,
                    max: sp.max_semesters,
                  }),
                ],
                [t('registration.facts.gpa'), sp.gpa?.toFixed(2) ?? '—'],
                [t('students.earned'), `${sp.earned_ects} ${t('catalog.ects')}`],
                [
                  t('registration.facts.advisor'),
                  sp.advisor
                    ? [sp.advisor.title, sp.advisor.first_name, sp.advisor.last_name]
                        .filter(Boolean)
                        .join(' ')
                    : '—',
                ],
                [t('students.curriculum'), sp.curriculum?.name_tr ?? '—'],
              ] as const
            ).map(([label, value]) => (
              <div key={label} className="flex justify-between gap-4 border-b border-dashed pb-1">
                <dt className="text-muted-foreground">{label}</dt>
                <dd className="text-right">{value}</dd>
              </div>
            ))}
          </dl>
          {sp.current_semester > sp.max_semesters && (
            <p className="mt-3 text-sm text-destructive">{t('students.beyondMax')}</p>
          )}
        </CardContent>
      </Card>
      <Tabs defaultValue="holds">
        <TabsList className="flex-wrap">
          <TabsTrigger value="holds">{t('students.holds.title')}</TabsTrigger>
          <TabsTrigger value="registration">{t('registration.title')}</TabsTrigger>
          <TabsTrigger value="progress">{t('registration.progress.title')}</TabsTrigger>
          <TabsTrigger value="history">{t('registration.history.title')}</TabsTrigger>
          <TabsTrigger value="status">{t('students.statusHistory')}</TabsTrigger>
        </TabsList>
        <TabsContent value="holds" className="pt-4">
          <Holds programId={sp.id} />
        </TabsContent>
        <TabsContent value="registration" className="pt-4">
          <CurrentRegistration programId={sp.id} />
        </TabsContent>
        <TabsContent value="progress" className="pt-4">
          <ProgressTab programId={sp.id} />
        </TabsContent>
        <TabsContent value="history" className="pt-4">
          <HistoryTab programId={sp.id} />
        </TabsContent>
        <TabsContent value="status" className="pt-4">
          <StatusHistory programId={sp.id} />
        </TabsContent>
      </Tabs>
      {dialog === 'status' && <StatusDialog program={sp} onClose={() => setDialog(null)} />}
      {dialog === 'hold' && <PlaceHoldDialog program={sp} onClose={() => setDialog(null)} />}
    </section>
  )
}

function Holds({ programId }: { programId: string }) {
  const { t } = useTranslation()
  const { has } = usePermissions()
  const { data } = useListStudentProgramHoldsQuery({ id: programId })
  const [releasing, setReleasing] = useState<Hold | null>(null)
  if (!data) {
    return <Skeleton className="h-24" />
  }
  if (data.items.length === 0) {
    return <p className="text-sm text-muted-foreground">{t('students.holds.none')}</p>
  }
  return (
    <ul className="space-y-2" aria-label={t('students.holds.title')}>
      {data.items.map((h) => (
        <li
          key={h.id}
          className="flex flex-wrap items-start justify-between gap-3 rounded-md border p-3 text-sm"
        >
          <div className="space-y-1">
            <p className="flex flex-wrap items-center gap-2">
              <HoldTypeBadge type={h.hold_type} />
              {!h.active && <Badge variant="muted">{t('students.holds.releasedBadge')}</Badge>}
              <span className="text-xs text-muted-foreground">
                {h.blocks.map((b) => t(`students.holds.blockLabels.${b}`)).join(', ')}
              </span>
            </p>
            <p>{h.reason}</p>
            <p className="text-xs text-muted-foreground">
              {t('students.holds.placedBy', { by: h.placed_by ?? '—', time: formatDateTime(h.placed_at) })}
              {h.released_at &&
                ` · ${t('students.holds.releasedBy', { by: h.released_by ?? '—', time: formatDateTime(h.released_at) })}: ${h.release_reason ?? ''}`}
            </p>
          </div>
          {h.active && has('hold:manage') && (
            <Button size="sm" variant="outline" onClick={() => setReleasing(h)}>
              {t('students.holds.release')}
            </Button>
          )}
        </li>
      ))}
      {releasing && <ReleaseHoldDialog hold={releasing} onClose={() => setReleasing(null)} />}
    </ul>
  )
}

function CurrentRegistration({ programId }: { programId: string }) {
  const { t } = useTranslation()
  const { data: r, error } = useGetRegistrationQuery({ id: programId })
  if (error) {
    return <ErrorState error={error} />
  }
  if (!r) {
    return <Skeleton className="h-24" />
  }
  return (
    <div className="flex flex-wrap items-center gap-3 text-sm">
      <span>{t('registration.termTitle', { term: r.term.code })}</span>
      <RegistrationStatusBadge status={r.status} />
      <span className="tabular-nums">
        {t('registration.basket.ectsOf', { total: r.total_ects, limit: r.ects_limit })}
      </span>
      {r.id && (
        <Button asChild size="sm" variant="outline">
          <Link to={`/danismanlik/kayitlar/${r.id}`}>{t('students.openRegistration')}</Link>
        </Button>
      )}
    </div>
  )
}

function ProgressTab({ programId }: { programId: string }) {
  const { data } = useGetStudentProgressQuery({ id: programId })
  return data ? <ProgressView progress={data} /> : <Skeleton className="h-64" />
}

function HistoryTab({ programId }: { programId: string }) {
  const { data } = useListCourseHistoryQuery({ id: programId })
  return data ? <HistoryTable items={data.items} /> : <Skeleton className="h-64" />
}

function StatusHistory({ programId }: { programId: string }) {
  const { data } = useListStudentProgramStatusHistoryQuery({ id: programId })
  if (!data) {
    return <Skeleton className="h-24" />
  }
  return (
    <ol className="space-y-2 border-l pl-4">
      {data.items.map((h, i) => (
        <li key={i} className="text-sm">
          <p className="flex flex-wrap items-center gap-2">
            {h.from_status && <EnrollmentStatusBadge status={h.from_status} />}
            {h.from_status && '→'}
            <EnrollmentStatusBadge status={h.to_status} />
            {h.decision_ref && <span className="text-xs text-muted-foreground">{h.decision_ref}</span>}
          </p>
          {h.reason && <p className="text-xs">{h.reason}</p>}
          <p className="text-xs text-muted-foreground">
            {formatDate(h.changed_at)}
            {h.changed_by && ` · ${h.changed_by}`}
          </p>
        </li>
      ))}
    </ol>
  )
}
