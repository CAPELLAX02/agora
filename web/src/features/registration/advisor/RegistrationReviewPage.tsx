import { ArrowLeftIcon, CheckIcon, ClockIcon, Undo2Icon, XIcon } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useParams } from 'react-router'
import { toast } from 'sonner'

import { formatDateTime } from '@/i18n/format'
import { ifMatch } from '@/shared/api/etag'
import {
  useApproveRegistrationMutation,
  useGetRegistrationByIdQuery,
  useGetStudentProgressQuery,
  useGrantRegistrationExtensionMutation,
  useListCourseHistoryQuery,
  useListRegistrationEventsQuery,
  useRejectRegistrationMutation,
  useRevokeRegistrationMutation,
  type Registration,
} from '@/shared/api/generated'
import { Button } from '@/shared/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/shared/ui/card'
import { FormDialog } from '@/shared/ui/form-dialog'
import { Input } from '@/shared/ui/input'
import { Label } from '@/shared/ui/label'
import { PageHeader } from '@/shared/ui/page-header'
import { Skeleton } from '@/shared/ui/skeleton'
import { ErrorState } from '@/shared/ui/states'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/shared/ui/tabs'
import { Textarea } from '@/shared/ui/textarea'

import { BasketItems, EctsMeter, RegistrationState } from '../Basket'
import { HistoryTable, ProgressView } from '../ProgressView'
import { newKey, shownViolations, useRun } from '../shared'
import { ViolationList } from '../ViolationList'
import { WeeklyPreview } from '../WeeklyPreview'
import { DecisionDialog } from './DecisionDialog'

type Decision = 'approve' | 'reject' | 'revoke'

/**
 * RegistrationReviewPage, danışmanın bir öğrencinin ders seçimini incelediği sayfadır: sepet ve
 * kuralların sonucu, haftalık program, müfredata göre ders durumu, ders geçmişi ve olay
 * günlüğü. Onay, red (gerekçeyle) ve onayı geri alma buradan yapılır; öğrenci işleri ek süre
 * tanıyabilir.
 */
export function RegistrationReviewPage() {
  const { t } = useTranslation()
  const { id = '' } = useParams()
  const { data: r, error, refetch } = useGetRegistrationByIdQuery({ id })
  const [decision, setDecision] = useState<Decision | null>(null)
  const [extending, setExtending] = useState(false)

  if (error) {
    return <ErrorState error={error} onRetry={() => void refetch()} />
  }
  if (!r) {
    return <Skeleton className="h-96" />
  }
  const name = `${r.student.first_name} ${r.student.last_name}`
  return (
    <>
      <Button asChild variant="ghost" size="sm" className="mb-2 -ml-2">
        <Link to="/danismanlik">
          <ArrowLeftIcon />
          {t('registration.advisor.title')}
        </Link>
      </Button>
      <PageHeader
        title={name}
        description={t('registration.review.subtitle', {
          no: r.student.student_no,
          program: r.student.program_code,
          term: r.term.code,
        })}
        actions={
          <>
            {r.actions.can_approve && (
              <Button onClick={() => setDecision('approve')}>
                <CheckIcon />
                {t('registration.advisor.approve')}
              </Button>
            )}
            {r.actions.can_reject && (
              <Button variant="outline" onClick={() => setDecision('reject')}>
                <XIcon />
                {t('registration.advisor.reject')}
              </Button>
            )}
            {r.actions.can_revoke && (
              <Button variant="outline" onClick={() => setDecision('revoke')}>
                <Undo2Icon />
                {t('registration.advisor.revoke')}
              </Button>
            )}
            {r.actions.can_extend && (
              <Button variant="outline" onClick={() => setExtending(true)}>
                <ClockIcon />
                {t('registration.extension.grant')}
              </Button>
            )}
          </>
        }
      />
      <div className="grid gap-6 xl:grid-cols-[minmax(0,1fr)_22rem]">
        <Card role="region" aria-label={t('registration.basket.title')}>
          <CardHeader>
            <CardTitle>{t('registration.review.basket')}</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <BasketItems items={r.items} />
            <ViolationList violations={shownViolations(r)} />
          </CardContent>
        </Card>
        <Card className="h-fit">
          <CardContent className="space-y-4">
            <RegistrationState registration={r} />
            <EctsMeter total={r.total_ects} limit={r.ects_limit} />
            <dl className="grid grid-cols-2 gap-2 text-sm">
              <dt className="text-muted-foreground">{t('registration.facts.semester')}</dt>
              <dd className="text-right tabular-nums">{r.student.semester}</dd>
              <dt className="text-muted-foreground">{t('registration.facts.gpa')}</dt>
              <dd className="text-right tabular-nums">{r.student.gpa?.toFixed(2) ?? '—'}</dd>
              {r.gpa_snapshot !== null && (
                <>
                  <dt className="text-muted-foreground">{t('registration.review.snapshot')}</dt>
                  <dd className="text-right tabular-nums">
                    {r.gpa_snapshot.toFixed(2)} · {r.max_ects_allowed} {t('catalog.ects')}
                  </dd>
                </>
              )}
              <dt className="text-muted-foreground">{t('registration.facts.advisor')}</dt>
              <dd className="truncate text-right">{r.student.advisor ?? '—'}</dd>
            </dl>
          </CardContent>
        </Card>
      </div>
      <Tabs defaultValue="weekly" className="mt-6">
        <TabsList>
          <TabsTrigger value="weekly">{t('registration.weekly')}</TabsTrigger>
          <TabsTrigger value="progress">{t('registration.progress.title')}</TabsTrigger>
          <TabsTrigger value="history">{t('registration.history.title')}</TabsTrigger>
          <TabsTrigger value="events">{t('registration.events.title')}</TabsTrigger>
        </TabsList>
        <TabsContent value="weekly" className="pt-4">
          <WeeklyPreview sections={r.items.map((it) => it.section)} />
        </TabsContent>
        <TabsContent value="progress" className="pt-4">
          <Progress programId={r.student_program_id} />
        </TabsContent>
        <TabsContent value="history" className="pt-4">
          <History programId={r.student_program_id} />
        </TabsContent>
        <TabsContent value="events" className="pt-4">
          <Events registration={r} />
        </TabsContent>
      </Tabs>
      {decision && <Decide registration={r} decision={decision} onClose={() => setDecision(null)} />}
      {extending && <Extension registration={r} onClose={() => setExtending(false)} />}
    </>
  )
}

function Progress({ programId }: { programId: string }) {
  const { data } = useGetStudentProgressQuery({ id: programId })
  return data ? <ProgressView progress={data} /> : <Skeleton className="h-64" />
}

function History({ programId }: { programId: string }) {
  const { data } = useListCourseHistoryQuery({ id: programId })
  return data ? <HistoryTable items={data.items} /> : <Skeleton className="h-64" />
}

function Events({ registration: r }: { registration: Registration }) {
  const { t } = useTranslation()
  const { data } = useListRegistrationEventsQuery({ id: r.student_program_id, termId: r.term.id })
  if (!data) {
    return <Skeleton className="h-40" />
  }
  return (
    <ol className="space-y-2 border-l pl-4" aria-label={t('registration.events.title')}>
      {[...data.items].reverse().map((e) => (
        <li key={e.id} className="text-sm">
          <p>
            <span className="font-medium">{t(`registration.events.types.${e.type}`)}</span>
            {e.actor && <span className="text-muted-foreground"> · {e.actor}</span>}
            {typeof e.payload.section === 'string' && (
              <span className="text-muted-foreground"> · {e.payload.section}</span>
            )}
            {e.payload.override === true && (
              <span className="text-warning"> · {t('registration.events.override')}</span>
            )}
          </p>
          {typeof e.payload.note === 'string' && e.payload.note && (
            <p className="text-xs text-muted-foreground">{e.payload.note}</p>
          )}
          <p className="text-xs text-muted-foreground tabular-nums">{formatDateTime(e.occurred_at)}</p>
        </li>
      ))}
    </ol>
  )
}

function Decide({
  registration: r,
  decision,
  onClose,
}: {
  registration: Registration
  decision: Decision
  onClose: () => void
}) {
  const { t } = useTranslation()
  const run = useRun()
  const [approve] = useApproveRegistrationMutation()
  const [reject] = useRejectRegistrationMutation()
  const [revoke] = useRevokeRegistrationMutation()
  const send = (note: string) => {
    const args = { id: r.id!, 'If-Match': ifMatch(r.version), 'Idempotency-Key': newKey() }
    switch (decision) {
      case 'approve':
        return approve({ ...args, body: note ? { note } : {} })
      case 'reject':
        return reject({ ...args, body: { note } })
      case 'revoke':
        return revoke({ ...args, body: { note } })
    }
  }
  return (
    <DecisionDialog
      title={t(`registration.review.${decision}.title`)}
      description={t(`registration.review.${decision}.description`)}
      confirm={t(`registration.advisor.${decision}`)}
      required={decision !== 'approve'}
      destructive={decision !== 'approve'}
      onClose={onClose}
      onConfirm={(note) => run(send(note), () => toast.success(t(`registration.review.${decision}.done`)))}
    />
  )
}

function Extension({ registration: r, onClose }: { registration: Registration; onClose: () => void }) {
  const { t } = useTranslation()
  const run = useRun()
  const [grant, { isLoading }] = useGrantRegistrationExtensionMutation()
  // Varsayılan: iki gün sonra (yerel saatle datetime-local biçiminde).
  const [until, setUntil] = useState(() => {
    const d = new Date(Date.now() + 2 * 24 * 3600_000)
    return new Date(d.getTime() - d.getTimezoneOffset() * 60_000).toISOString().slice(0, 16)
  })
  const [reason, setReason] = useState('')
  const [error, setError] = useState<string | null>(null)
  return (
    <FormDialog
      title={t('registration.extension.title')}
      description={t('registration.extension.description')}
      error={error}
      onClose={onClose}
      onSubmit={() => {
        if (reason.trim().length < 5) {
          setError(t('registration.decision.noteRequired'))
          return
        }
        void run(
          grant({
            id: r.student_program_id,
            'Idempotency-Key': newKey(),
            body: { term_id: r.term.id, until: new Date(until).toISOString(), reason: reason.trim() },
          }),
          () => {
            toast.success(t('registration.extension.done'))
            onClose()
          },
        )
      }}
      submit={
        <Button type="submit" loading={isLoading}>
          {t('registration.extension.grant')}
        </Button>
      }
    >
      <div className="space-y-2">
        <Label htmlFor="extension-until">{t('registration.extension.until')}</Label>
        <Input
          id="extension-until"
          type="datetime-local"
          value={until}
          onChange={(e) => setUntil(e.target.value)}
        />
      </div>
      <div className="space-y-2">
        <Label htmlFor="extension-reason">{t('registration.decision.reason')}</Label>
        <Textarea id="extension-reason" value={reason} onChange={(e) => setReason(e.target.value)} />
      </div>
    </FormDialog>
  )
}
