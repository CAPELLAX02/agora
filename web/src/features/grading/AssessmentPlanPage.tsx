import { LockIcon, LockOpenIcon, PencilIcon, PlusIcon, Trash2Icon } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useParams } from 'react-router'
import { toast } from 'sonner'

import { formatDate, formatDateTime } from '@/i18n/format'
import { ifMatch } from '@/shared/api/etag'
import { fieldErrors } from '@/shared/api/errors'
import {
  useGetAssessmentPlanQuery,
  useGetOfferingQuery,
  useGetSectionQuery,
  useListAssessmentTypesQuery,
  useLockAssessmentPlanMutation,
  useSetAssessmentPlanMutation,
  useUnlockAssessmentPlanMutation,
  type AssessmentPlan,
} from '@/shared/api/generated'
import { useErrorMessage } from '@/shared/api/useErrorMessage'
import { useLocalized } from '@/shared/lib/localized'
import { Alert, AlertDescription } from '@/shared/ui/alert'
import { Badge } from '@/shared/ui/badge'
import { Button } from '@/shared/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/shared/ui/card'
import { ConfirmDialog } from '@/shared/ui/confirm-dialog'
import { Field } from '@/shared/ui/field'
import { Input } from '@/shared/ui/input'
import { PageHeader } from '@/shared/ui/page-header'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/shared/ui/select'
import { Skeleton } from '@/shared/ui/skeleton'
import { ErrorState } from '@/shared/ui/states'
import { Table, TableBody, TableCell, TableFooter, TableHead, TableHeader, TableRow } from '@/shared/ui/table'
import { Textarea } from '@/shared/ui/textarea'

type Row = { type: string; name: string; weight: string; date: string }

/**
 * AssessmentPlanPage, bir şubenin değerlendirme planıdır. Şubenin öğretim elemanı (ya da
 * bölüm) planı düzenler ve kilitler; kilitli planı gerekçeyle sadece bölüm açar. Bütünleme
 * finalden türetilir, plana eklenmez.
 */
export function AssessmentPlanPage() {
  const { t, i18n } = useTranslation()
  const { id = '' } = useParams()
  const localized = useLocalized()
  const section = useGetSectionQuery({ id })
  const offering = useGetOfferingQuery({ id: section.data?.offering_id ?? '' }, { skip: !section.data })
  const { data: plan, error, refetch } = useGetAssessmentPlanQuery({ id })
  const [editing, setEditing] = useState(false)
  const [confirm, setConfirm] = useState<'lock' | 'unlock' | null>(null)

  if (error) {
    return <ErrorState error={error} onRetry={() => void refetch()} />
  }
  if (!plan || !offering.data || !section.data) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-10 w-96" />
        <Skeleton className="h-64" />
      </div>
    )
  }
  const en = i18n.language === 'en'

  return (
    <>
      <PageHeader
        title={t('plan.title')}
        description={`${offering.data.course.code}-${section.data.section_code} ${localized(offering.data.course)} · ${offering.data.term.code}`}
        actions={
          <div className="flex flex-wrap gap-2">
            {plan.editable && !editing && (
              <>
                <Button variant="outline" onClick={() => setEditing(true)}>
                  <PencilIcon />
                  {t('plan.edit')}
                </Button>
                <Button onClick={() => setConfirm('lock')} disabled={!plan.is_complete}>
                  <LockIcon />
                  {t('plan.lock')}
                </Button>
              </>
            )}
            {plan.can_unlock && (
              <Button variant="outline" onClick={() => setConfirm('unlock')}>
                <LockOpenIcon />
                {t('plan.unlock')}
              </Button>
            )}
          </div>
        }
      />
      {plan.locked_at && (
        <Alert className="mb-6">
          <LockIcon />
          <AlertDescription>
            {t('plan.lockedInfo', {
              time: formatDateTime(plan.locked_at),
              name: plan.locked_by ?? t('plan.someone'),
            })}
          </AlertDescription>
        </Alert>
      )}
      {editing ? (
        <PlanEditor sectionId={id} plan={plan} onDone={() => setEditing(false)} />
      ) : (
        <Card className="gap-0 py-0">
          <CardHeader className="border-b py-4">
            <CardTitle>{t('plan.components')}</CardTitle>
            <CardDescription>{t('plan.rule')}</CardDescription>
          </CardHeader>
          {plan.components.length === 0 ? (
            <CardContent className="py-6 text-sm text-muted-foreground">{t('plan.empty')}</CardContent>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('plan.component')}</TableHead>
                  <TableHead className="hidden sm:table-cell">{t('plan.date')}</TableHead>
                  <TableHead className="w-24 text-right">{t('plan.weight')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {plan.components.map((c) => (
                  <TableRow
                    key={c.id}
                    className={c.type.category === 'MAKEUP' ? 'text-muted-foreground' : undefined}
                  >
                    <TableCell>
                      {en ? c.label_en : c.label_tr}
                      {c.type.category === 'MAKEUP' && (
                        <Badge variant="muted" className="ml-2">
                          {t('plan.replacesFinal')}
                        </Badge>
                      )}
                    </TableCell>
                    <TableCell className="hidden sm:table-cell">
                      {c.scheduled_on ? formatDate(c.scheduled_on) : '—'}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">%{c.weight}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
              <TableFooter>
                <TableRow>
                  <TableCell>{t('plan.total')}</TableCell>
                  <TableCell className="hidden sm:table-cell" />
                  <TableCell className="text-right tabular-nums">
                    %{plan.in_term_weight + plan.final_weight}
                  </TableCell>
                </TableRow>
              </TableFooter>
            </Table>
          )}
        </Card>
      )}
      {confirm === 'lock' && <LockDialog sectionId={id} onClose={() => setConfirm(null)} />}
      {confirm === 'unlock' && <UnlockDialog sectionId={id} onClose={() => setConfirm(null)} />}
    </>
  )
}

function LockDialog({ sectionId, onClose }: { sectionId: string; onClose: () => void }) {
  const { t } = useTranslation()
  const message = useErrorMessage()
  const [lock] = useLockAssessmentPlanMutation()
  return (
    <ConfirmDialog
      title={t('plan.lockTitle')}
      description={t('plan.lockDescription')}
      confirm={t('plan.lock')}
      onClose={onClose}
      onConfirm={async () => {
        const res = await lock({ id: sectionId })
        if (res.error) {
          toast.error(message(res.error))
          return
        }
        toast.success(t('plan.locked'))
        onClose()
      }}
    />
  )
}

function UnlockDialog({ sectionId, onClose }: { sectionId: string; onClose: () => void }) {
  const { t } = useTranslation()
  const message = useErrorMessage()
  const [unlock] = useUnlockAssessmentPlanMutation()
  const [reason, setReason] = useState('')
  const [invalid, setInvalid] = useState(false)
  return (
    <ConfirmDialog
      title={t('plan.unlockTitle')}
      description={t('plan.unlockDescription')}
      confirm={t('plan.unlock')}
      onClose={onClose}
      onConfirm={async () => {
        if (reason.trim().length < 5) {
          setInvalid(true)
          return
        }
        const res = await unlock({ id: sectionId, body: { reason: reason.trim() } })
        if (res.error) {
          toast.error(message(res.error))
          return
        }
        toast.success(t('plan.unlocked'))
        onClose()
      }}
    >
      <Field
        label={t('common.reason')}
        hint={t('common.reasonHint')}
        error={invalid ? t('plan.reasonTooShort') : undefined}
      >
        {(p) => <Textarea {...p} value={reason} onChange={(e) => setReason(e.target.value)} rows={2} />}
      </Field>
    </ConfirmDialog>
  )
}

/**
 * PlanEditor, planın dönem içi bileşenlerini ve finali düzenler. Toplam ve tek final kuralı
 * yazarken gösterilir; asıl doğrulama sunucudadır.
 */
function PlanEditor({
  sectionId,
  plan,
  onDone,
}: {
  sectionId: string
  plan: AssessmentPlan
  onDone: () => void
}) {
  const { t } = useTranslation()
  const localized = useLocalized()
  const message = useErrorMessage()
  const types = useListAssessmentTypesQuery()
  const [save, { error, isLoading }] = useSetAssessmentPlanMutation()
  const [rows, setRows] = useState<Row[]>(() => {
    const own = plan.components.filter((c) => c.type.category !== 'MAKEUP')
    return own.length > 0
      ? own.map((c) => ({
          type: c.type.code,
          name: c.name_tr ?? '',
          weight: String(c.weight),
          date: c.scheduled_on ?? '',
        }))
      : [
          { type: 'MIDTERM', name: '', weight: '40', date: '' },
          { type: 'FINAL', name: '', weight: '60', date: '' },
        ]
  })
  const choices = types.data?.items.filter((ty) => ty.category !== 'MAKEUP') ?? []
  const total = rows.reduce((s, r) => s + (Number(r.weight) || 0), 0)
  const finals = rows.filter((r) => r.type === 'FINAL').length
  const update = (i: number, patch: Partial<Row>) =>
    setRows(rows.map((r, j) => (j === i ? { ...r, ...patch } : r)))

  const submit = async () => {
    const res = await save({
      id: sectionId,
      'If-Match': ifMatch(plan.version),
      body: {
        components: rows.map((r) => ({
          type: r.type,
          weight: Number(r.weight),
          ...(r.name.trim() && { name_tr: r.name.trim() }),
          ...(r.date && { scheduled_on: r.date }),
        })),
      },
    })
    if (res.data) {
      toast.success(t('plan.saved'))
      onDone()
    }
  }
  const serverError = error ? (Object.values(fieldErrors(error))[0]?.[0]?.message ?? message(error)) : null

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('plan.edit')}</CardTitle>
        <CardDescription>{t('plan.rule')}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {serverError && (
          <Alert variant="destructive">
            <AlertDescription>{serverError}</AlertDescription>
          </Alert>
        )}
        <ul className="space-y-3">
          {rows.map((r, i) => (
            <li key={i} className="grid gap-2 sm:grid-cols-[12rem_1fr_6rem_10rem_auto] sm:items-end">
              <Field label={t('plan.type')}>
                {(p) => (
                  <Select value={r.type} onValueChange={(v) => update(i, { type: v })}>
                    <SelectTrigger {...p} className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {choices.map((ty) => (
                        <SelectItem key={ty.code} value={ty.code}>
                          {localized(ty)}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                )}
              </Field>
              <Field label={t('plan.name')}>
                {(p) => (
                  <Input
                    {...p}
                    value={r.name}
                    onChange={(e) => update(i, { name: e.target.value })}
                    placeholder={t('plan.namePlaceholder')}
                  />
                )}
              </Field>
              <Field label={t('plan.weight')}>
                {(p) => (
                  <Input
                    {...p}
                    value={r.weight}
                    onChange={(e) => update(i, { weight: e.target.value })}
                    inputMode="decimal"
                  />
                )}
              </Field>
              <Field label={t('plan.date')}>
                {(p) => (
                  <Input
                    {...p}
                    type="date"
                    value={r.date}
                    onChange={(e) => update(i, { date: e.target.value })}
                  />
                )}
              </Field>
              <Button
                type="button"
                variant="ghost"
                size="icon"
                onClick={() => setRows(rows.filter((_, j) => j !== i))}
                aria-label={t('plan.removeRow', { n: i + 1 })}
              >
                <Trash2Icon />
              </Button>
            </li>
          ))}
        </ul>
        <div className="flex flex-wrap items-center justify-between gap-3">
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => setRows([...rows, { type: 'HOMEWORK', name: '', weight: '10', date: '' }])}
          >
            <PlusIcon />
            {t('plan.addRow')}
          </Button>
          <p
            className={
              total === 100 && finals === 1 ? 'text-sm text-muted-foreground' : 'text-sm text-destructive'
            }
            role="status"
          >
            {t('plan.check', { total, finals })}
          </p>
        </div>
        <div className="flex justify-end gap-2">
          <Button type="button" variant="outline" onClick={onDone}>
            {t('common.cancel')}
          </Button>
          <Button type="button" loading={isLoading} onClick={() => void submit()}>
            {t('common.save')}
          </Button>
        </div>
      </CardContent>
    </Card>
  )
}
