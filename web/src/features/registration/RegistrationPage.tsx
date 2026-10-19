import { FileTextIcon, SendIcon, ShoppingBasketIcon, Undo2Icon } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'
import { toast } from 'sonner'

import { navTheme } from '@/features/shell/nav'
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
import { cn } from '@/shared/lib/utils'
import { Alert, AlertDescription } from '@/shared/ui/alert'
import { Badge } from '@/shared/ui/badge'
import { Button } from '@/shared/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/shared/ui/card'
import { ConfirmDialog } from '@/shared/ui/confirm-dialog'
import { IconChip } from '@/shared/ui/icon-chip'
import { PageHeader } from '@/shared/ui/page-header'
import { Skeleton } from '@/shared/ui/skeleton'
import { EmptyState, ErrorState } from '@/shared/ui/states'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/shared/ui/tabs'
import { toneText } from '@/shared/ui/tone'

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
      <PageHeader
        title={t('registration.title')}
        description={t('registration.description')}
        {...navTheme('/ders-secimi')}
      />
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
      <Card className="relative overflow-hidden border-violet-500/20 bg-gradient-to-br from-violet-500/[0.07] via-card to-sky-500/[0.06]">
        <CardContent className="grid gap-6 xl:grid-cols-[minmax(0,1fr)_minmax(0,26rem)]">
          <div className="space-y-3">
            <h2 className="text-xl font-bold tracking-tight">
              {t('registration.termTitle', { term: r.term.code })}
            </h2>
            <RegistrationState registration={r} />
          </div>
          <div className="space-y-4">
            <dl className="grid grid-cols-[minmax(0,0.8fr)_minmax(0,0.8fr)_minmax(0,1.6fr)] gap-2">
              {(
                [
                  [t('registration.facts.semester'), String(r.student.semester), 'sky'],
                  [t('registration.facts.gpa'), r.student.gpa?.toFixed(2) ?? '—', 'emerald'],
                  [t('registration.facts.advisor'), r.student.advisor ?? '—', 'violet'],
                ] as const
              ).map(([label, value, tone]) => (
                <div key={label} className="min-w-0 rounded-xl border border-border/60 bg-card/80 px-3 py-2">
                  <dt className="text-[11px] font-bold tracking-wide text-muted-foreground uppercase">
                    {label}
                  </dt>
                  <dd
                    className={cn(
                      'truncate font-bold tabular-nums',
                      value.length > 6 ? 'text-sm leading-6' : 'text-lg',
                      toneText[tone],
                    )}
                    title={value}
                  >
                    {value}
                  </dd>
                </div>
              ))}
            </dl>
            <EctsMeter total={r.total_ects} limit={r.ects_limit} />
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
          return {
            label: t('registration.offer.inBasket'),
            kind: 'done',
            disabled: true,
            onClick: () => undefined,
          }
        }
        if (item) {
          return {
            label: t('registration.offer.switch'),
            kind: 'switch',
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
          kind: 'add',
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
    // Yapışkan sepet ekrandan uzun olmasın: dersler ve kurallar kendi içinde kayar, AKTS ve
    // gönder düğmesi hep görünür kalır.
    <Card
      className="h-fit xl:sticky xl:top-20 xl:max-h-[calc(100svh-6rem)]"
      role="region"
      aria-label={t('registration.basket.title')}
    >
      <CardHeader className="flex items-center gap-3">
        <IconChip icon={ShoppingBasketIcon} tone="violet" solid />
        <CardTitle className="flex-1 text-lg">{t('registration.basket.title')}</CardTitle>
        <Badge variant="violet">{t('dashboard.registration.courses', { count: r.items.length })}</Badge>
      </CardHeader>
      <CardContent className="flex min-h-0 flex-col gap-4">
        <div className="-mr-2 min-h-0 space-y-4 overflow-y-auto pr-2">
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
          <ViolationList violations={shownViolations(r)} />
        </div>
        <EctsMeter total={r.total_ects} limit={r.ects_limit} />
        <div className="flex flex-wrap gap-2">
          {(r.status === 'DRAFT' || r.status === 'REJECTED') && r.actions.can_edit && (
            <Button
              variant="brand"
              size="lg"
              className="w-full"
              disabled={!r.actions.can_submit}
              onClick={() => setConfirming(true)}
            >
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
          icon={SendIcon}
          tone="violet"
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
