import { FileTextIcon, SendIcon, Undo2Icon } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'
import { toast } from 'sonner'

import { ifMatch } from '@/shared/api/etag'
import {
  useAddRegistrationItemMutation,
  useChangeRegistrationSectionMutation,
  useGetRegistrationQuery,
  useListMyProgramsQuery,
  useListRegistrationOfferQuery,
  useRemoveRegistrationItemMutation,
  useSubmitRegistrationMutation,
  useWithdrawRegistrationMutation,
  type Registration,
  type StudentProgram,
} from '@/shared/api/generated'
import { useLocalized } from '@/shared/lib/localized'
import { Alert, AlertDescription } from '@/shared/ui/alert'
import { Button } from '@/shared/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/shared/ui/card'
import { ConfirmDialog } from '@/shared/ui/confirm-dialog'
import { PageHeader } from '@/shared/ui/page-header'
import { Skeleton } from '@/shared/ui/skeleton'
import { EmptyState, ErrorState } from '@/shared/ui/states'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/shared/ui/tabs'

import { AddDropPanel } from './AddDropPanel'
import { BasketItems, EctsMeter, RegistrationState } from './Basket'
import { OfferList } from './OfferList'
import { newKey, shownViolations, useRun, useViolationText } from './shared'
import { ViolationList } from './ViolationList'
import { WeeklyPreview } from './WeeklyPreview'

/**
 * RegistrationPage, öğrencinin dönemlik ders seçimidir: alınabilir dersler (alttan, dönem,
 * seçmeli...), sepet ve AKTS sınırı, kuralların anlık geri bildirimi, haftalık program
 * önizlemesi, danışmana gönderme ve geri çekme. Kayıt onaylandıysa ekle-bırak talepleri ve
 * kayıt onay belgesi. Çift anadal ya da yandal öğrencisi program bağlamını sekmeyle seçer.
 */
export function RegistrationPage() {
  const { t } = useTranslation()
  const localized = useLocalized()
  const { data, error, refetch } = useListMyProgramsQuery()
  const [active, setActive] = useState('')

  if (error) {
    return <ErrorState error={error} onRetry={() => void refetch()} />
  }
  if (!data) {
    return <Skeleton className="h-96" />
  }
  const current = data.items.find((p) => p.id === active) ?? data.items[0]
  return (
    <>
      <PageHeader title={t('registration.title')} description={t('registration.description')} />
      {!current ? (
        <EmptyState>{t('registration.noPrograms')}</EmptyState>
      ) : (
        <div className="space-y-6">
          {data.items.length > 1 && (
            <Tabs value={current.id} onValueChange={setActive}>
              <TabsList aria-label={t('registration.programs')}>
                {data.items.map((p) => (
                  <TabsTrigger key={p.id} value={p.id}>
                    {localized(p.program)}
                  </TabsTrigger>
                ))}
              </TabsList>
            </Tabs>
          )}
          <StudentRegistration key={current.id} program={current} />
        </div>
      )}
    </>
  )
}

function StudentRegistration({ program }: { program: StudentProgram }) {
  const { t } = useTranslation()
  const { data: r, error, refetch } = useGetRegistrationQuery({ id: program.id })
  if (error) {
    return <ErrorState error={error} onRetry={() => void refetch()} />
  }
  if (!r) {
    return <Skeleton className="h-96" />
  }
  return (
    <div className="space-y-6">
      <Card>
        <CardContent className="grid gap-6 md:grid-cols-[minmax(0,1fr)_18rem]">
          <div className="space-y-3">
            <h2 className="text-lg font-semibold">{t('registration.termTitle', { term: r.term.code })}</h2>
            <RegistrationState registration={r} />
          </div>
          <div className="space-y-3">
            <EctsMeter total={r.total_ects} limit={r.ects_limit} />
            <dl className="grid grid-cols-2 gap-2 text-sm">
              <dt className="text-muted-foreground">{t('registration.facts.semester')}</dt>
              <dd className="text-right tabular-nums">{r.student.semester}</dd>
              <dt className="text-muted-foreground">{t('registration.facts.gpa')}</dt>
              <dd className="text-right tabular-nums">{r.student.gpa?.toFixed(2) ?? '—'}</dd>
              <dt className="text-muted-foreground">{t('registration.facts.advisor')}</dt>
              <dd className="truncate text-right">{r.student.advisor ?? '—'}</dd>
            </dl>
          </div>
        </CardContent>
      </Card>

      <div className="grid gap-6 xl:grid-cols-[minmax(0,1fr)_24rem]">
        <Tabs defaultValue="main" className="min-w-0">
          <TabsList>
            <TabsTrigger value="main">
              {r.actions.can_edit
                ? t('registration.tabs.offer')
                : r.status === 'APPROVED'
                  ? t('registration.addDrop.title')
                  : t('registration.tabs.status')}
            </TabsTrigger>
            <TabsTrigger value="weekly">
              {t('registration.weekly')} ({r.items.length})
            </TabsTrigger>
          </TabsList>
          <TabsContent value="main" className="space-y-4 pt-2">
            {r.actions.can_edit ? (
              <Offers registration={r} />
            ) : r.status === 'APPROVED' ? (
              <AddDropPanel registration={r} programId={program.program.id} />
            ) : (
              <Alert>
                <AlertDescription>
                  {r.status === 'SUBMITTED'
                    ? t('registration.lockedSubmitted')
                    : t('registration.windowClosed')}
                </AlertDescription>
              </Alert>
            )}
          </TabsContent>
          <TabsContent value="weekly" className="pt-2">
            <Card>
              <CardContent>
                <WeeklyPreview sections={r.items.map((it) => it.section)} />
              </CardContent>
            </Card>
          </TabsContent>
        </Tabs>
        <BasketCard registration={r} />
      </div>
    </div>
  )
}

function Offers({ registration: r }: { registration: Registration }) {
  const { t } = useTranslation()
  const run = useRun()
  const text = useViolationText()
  const { data: offer, error, refetch } = useListRegistrationOfferQuery({ id: r.student_program_id })
  const [add, adding] = useAddRegistrationItemMutation()
  const [change, changing] = useChangeRegistrationSectionMutation()
  const busy = adding.isLoading || changing.isLoading

  if (error) {
    return <ErrorState error={error} onRetry={() => void refetch()} />
  }
  if (!offer) {
    return <Skeleton className="h-96" />
  }
  const done = (res: Registration) => {
    for (const w of res.warnings) {
      toast.warning(text(w))
    }
  }
  return (
    <OfferList
      offer={offer}
      action={(o, s) => {
        const item = r.items.find((it) => it.section.course.id === o.course.id)
        if (item && item.section.id === s.id) {
          return { label: t('registration.offer.inBasket'), disabled: true, onClick: () => undefined }
        }
        if (item) {
          return {
            label: t('registration.offer.switch'),
            disabled: busy || !s.available,
            onClick: () =>
              void run(
                change({
                  id: r.student_program_id,
                  itemId: item.id,
                  'Idempotency-Key': newKey(),
                  body: { section_id: s.id },
                }),
                (res) => {
                  toast.success(t('registration.offer.switched', { label: s.label }))
                  done(res)
                },
              ),
          }
        }
        return {
          label: t('registration.offer.add'),
          disabled: busy || !s.available,
          onClick: () =>
            void run(
              add({ id: r.student_program_id, 'Idempotency-Key': newKey(), body: { section_id: s.id } }),
              (res) => {
                toast.success(t('registration.offer.added', { label: s.label }))
                done(res)
              },
            ),
        }
      }}
    />
  )
}

function BasketCard({ registration: r }: { registration: Registration }) {
  const { t } = useTranslation()
  const run = useRun()
  const [remove, removing] = useRemoveRegistrationItemMutation()
  const [submit] = useSubmitRegistrationMutation()
  const [withdraw, withdrawing] = useWithdrawRegistrationMutation()
  const [confirming, setConfirming] = useState(false)
  const warnings = r.evaluation.violations.filter((v) => v.severity === 'WARNING').length

  return (
    <Card className="h-fit xl:sticky xl:top-4" role="region" aria-label={t('registration.basket.title')}>
      <CardHeader>
        <CardTitle>{t('registration.basket.title')}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <BasketItems
          items={r.items}
          busy={removing.isLoading}
          onRemove={
            r.actions.can_edit
              ? (it) =>
                  void run(
                    remove({ id: r.student_program_id, itemId: it.id, 'Idempotency-Key': newKey() }),
                    () => toast.success(t('registration.basket.removed', { label: it.section.label })),
                  )
              : undefined
          }
        />
        <EctsMeter total={r.total_ects} limit={r.ects_limit} />
        <ViolationList violations={shownViolations(r)} />
        <div className="flex flex-wrap gap-2">
          {(r.status === 'DRAFT' || r.status === 'REJECTED') && r.actions.can_edit && (
            <Button disabled={!r.actions.can_submit} onClick={() => setConfirming(true)}>
              <SendIcon />
              {t('registration.submit')}
            </Button>
          )}
          {r.actions.can_withdraw && (
            <Button
              variant="outline"
              loading={withdrawing.isLoading}
              onClick={() =>
                void run(
                  withdraw({
                    id: r.student_program_id,
                    'If-Match': ifMatch(r.version),
                    'Idempotency-Key': newKey(),
                  }),
                  () => toast.success(t('registration.withdrawn')),
                )
              }
            >
              <Undo2Icon />
              {t('registration.withdraw')}
            </Button>
          )}
          {r.status === 'APPROVED' && (
            <Button asChild variant="outline">
              <Link to={`/ders-secimi/belge?program=${r.student_program_id}`}>
                <FileTextIcon />
                {t('registration.report.open')}
              </Link>
            </Button>
          )}
        </div>
        {r.evaluation.blocking && r.items.length > 0 && r.actions.can_edit && (
          <p className="text-xs text-muted-foreground">{t('registration.blockingHint')}</p>
        )}
      </CardContent>
      {confirming && (
        <ConfirmDialog
          title={t('registration.confirmSubmit.title')}
          description={t('registration.confirmSubmit.description', {
            count: r.items.length,
            ects: r.total_ects,
            warnings,
          })}
          confirm={t('registration.submit')}
          onClose={() => setConfirming(false)}
          onConfirm={async () => {
            await run(
              submit({
                id: r.student_program_id,
                'If-Match': ifMatch(r.version),
                'Idempotency-Key': newKey(),
              }),
              () => {
                toast.success(t('registration.submitted'))
                setConfirming(false)
              },
            )
          }}
        />
      )}
    </Card>
  )
}
