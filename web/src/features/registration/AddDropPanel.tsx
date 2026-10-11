import { MinusIcon, PlusIcon, Undo2Icon } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { formatDateTime } from '@/i18n/format'
import { ifMatch } from '@/shared/api/etag'
import {
  useGetCalendarWindowsQuery,
  useListAddDropRequestsQuery,
  useListRegistrationOfferQuery,
  useRequestAddDropMutation,
  useWithdrawAddDropMutation,
  type AddDropRequest,
  type Registration,
  type RegistrationSection,
} from '@/shared/api/generated'
import { useLocalized } from '@/shared/lib/localized'
import { Badge } from '@/shared/ui/badge'
import { Button } from '@/shared/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/shared/ui/card'
import { Checkbox } from '@/shared/ui/checkbox'
import { Label } from '@/shared/ui/label'
import { Skeleton } from '@/shared/ui/skeleton'
import { Textarea } from '@/shared/ui/textarea'

import { AddDropStatusBadge } from './badges'
import { OfferList } from './OfferList'
import { newKey, useRun } from './shared'

/**
 * AddDropPanel, onaylanmış kayıtta ekle-bırak talepleridir. Ekle-bırak (öğrenci) penceresi
 * açıkken öğrenci bırakacağı dersleri işaretler, ekleyeceği şubeleri seçer ve talebi açar;
 * talep danışmana, sonra bölüm başkanına gider. Açık talep geri çekilebilir.
 */
export function AddDropPanel({
  registration: r,
  programId,
}: {
  registration: Registration
  programId: string
}) {
  const { t } = useTranslation()
  const { data: requests } = useListAddDropRequestsQuery({ id: r.student_program_id })
  const { data: windows } = useGetCalendarWindowsQuery({ programId })
  const window = windows?.items.find((w) => w.type.code === 'ADD_DROP_STUDENT')
  const open = requests?.items.some((q) => q.status === 'PENDING_ADVISOR' || q.status === 'PENDING_DEPT_HEAD')
  const [composing, setComposing] = useState(false)

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader>
          <CardTitle>{t('registration.addDrop.title')}</CardTitle>
          <CardDescription>
            {window?.open && window.current
              ? t('registration.addDrop.openUntil', {
                  time: formatDateTime(new Date(new Date(window.current.ends_at).getTime() - 60_000)),
                })
              : window?.next
                ? t('registration.addDrop.opensAt', { time: formatDateTime(window.next.starts_at) })
                : t('registration.addDrop.closed')}
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          {!requests ? (
            <Skeleton className="h-16" />
          ) : requests.items.length === 0 ? (
            <p className="text-sm text-muted-foreground">{t('registration.addDrop.none')}</p>
          ) : (
            <ul className="space-y-3" aria-label={t('registration.addDrop.requests')}>
              {requests.items.map((q) => (
                <RequestItem key={q.id} request={q} />
              ))}
            </ul>
          )}
          {window?.open && !open && !composing && (
            <Button onClick={() => setComposing(true)}>{t('registration.addDrop.new')}</Button>
          )}
        </CardContent>
      </Card>
      {composing && <Compose registration={r} onDone={() => setComposing(false)} />}
    </div>
  )
}

function RequestItem({ request: q }: { request: AddDropRequest }) {
  const { t } = useTranslation()
  const localized = useLocalized()
  const run = useRun()
  const [withdraw, { isLoading }] = useWithdrawAddDropMutation()
  return (
    <li className="space-y-2 rounded-md border p-3 text-sm">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <span className="flex items-center gap-2">
          <AddDropStatusBadge status={q.status} />
          <span className="text-xs text-muted-foreground">{formatDateTime(q.created_at)}</span>
        </span>
        {q.actions.can_withdraw && (
          <Button
            size="sm"
            variant="outline"
            loading={isLoading}
            onClick={() =>
              void run(
                withdraw({ id: q.id, 'If-Match': ifMatch(q.version), 'Idempotency-Key': newKey() }),
                () => toast.success(t('registration.addDrop.withdrawn')),
              )
            }
          >
            <Undo2Icon />
            {t('registration.addDrop.withdraw')}
          </Button>
        )}
      </div>
      <ul className="space-y-1">
        {q.items.map((it) => (
          <li key={it.id} className="flex items-center gap-2">
            {it.action === 'ADD' ? (
              <PlusIcon className="size-4 text-success" aria-label={t('registration.addDrop.add')} />
            ) : (
              <MinusIcon className="size-4 text-destructive" aria-label={t('registration.addDrop.drop')} />
            )}
            <span className="font-medium">{it.section.label}</span>
            <span className="truncate text-muted-foreground">{localized(it.section.course)}</span>
          </li>
        ))}
      </ul>
      {q.student_note && <p className="text-xs text-muted-foreground">{q.student_note}</p>}
      {[q.advisor, q.dept_head].map(
        (d, i) =>
          d && (
            <p key={i} className="text-xs text-muted-foreground">
              {t(i === 0 ? 'registration.addDrop.advisorDecision' : 'registration.addDrop.headDecision', {
                by: d.by ?? '',
                time: formatDateTime(d.at),
              })}
              {d.note && ` — ${d.note}`}
            </p>
          ),
      )}
    </li>
  )
}

function Compose({ registration: r, onDone }: { registration: Registration; onDone: () => void }) {
  const { t } = useTranslation()
  const localized = useLocalized()
  const run = useRun()
  const { data: offer } = useListRegistrationOfferQuery({ id: r.student_program_id })
  const [request, { isLoading }] = useRequestAddDropMutation()
  const [drops, setDrops] = useState<string[]>([])
  const [adds, setAdds] = useState<RegistrationSection[]>([])
  const [note, setNote] = useState('')

  const delta =
    adds.reduce((sum, s) => sum + s.course.ects, 0) -
    r.items.filter((it) => drops.includes(it.id)).reduce((sum, it) => sum + it.ects, 0)

  const submit = () =>
    void run(
      request({
        id: r.student_program_id,
        'Idempotency-Key': newKey(),
        body: { adds: adds.map((s) => s.id), drops, ...(note.trim() && { note: note.trim() }) },
      }),
      () => {
        toast.success(t('registration.addDrop.sent'))
        onDone()
      },
    )

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('registration.addDrop.compose')}</CardTitle>
        <CardDescription>{t('registration.addDrop.composeHint')}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-5">
        <fieldset className="space-y-2">
          <legend className="text-sm font-medium">{t('registration.addDrop.dropLegend')}</legend>
          {r.items.map((it) => (
            <div key={it.id} className="flex items-center gap-2 text-sm">
              <Checkbox
                id={`drop-${it.id}`}
                checked={drops.includes(it.id)}
                onCheckedChange={(v) => setDrops(v ? [...drops, it.id] : drops.filter((d) => d !== it.id))}
              />
              <Label htmlFor={`drop-${it.id}`} className="font-normal">
                {it.section.label} {localized(it.section.course)} ·{' '}
                {t('registration.ects', { ects: it.ects })}
              </Label>
            </div>
          ))}
        </fieldset>
        <div className="space-y-2">
          <p className="text-sm font-medium">{t('registration.addDrop.addLegend')}</p>
          {adds.length > 0 && (
            <ul className="flex flex-wrap gap-2" aria-label={t('registration.addDrop.adds')}>
              {adds.map((s) => (
                <li key={s.id}>
                  <Badge variant="secondary" className="gap-1">
                    {s.label}
                    <button
                      type="button"
                      className="ml-1"
                      aria-label={t('registration.addDrop.removeAdd', { label: s.label })}
                      onClick={() => setAdds(adds.filter((a) => a.id !== s.id))}
                    >
                      ×
                    </button>
                  </Badge>
                </li>
              ))}
            </ul>
          )}
          {!offer ? (
            <Skeleton className="h-40" />
          ) : (
            <OfferList
              offer={offer}
              action={(o, s) => {
                const kept = o.basket_item_id && !drops.includes(o.basket_item_id)
                const chosen = adds.some((a) => a.id === s.id)
                const sameCourse = adds.some((a) => a.course.id === o.course.id)
                if (kept) {
                  return { label: t('registration.offer.inBasket'), disabled: true, onClick: () => undefined }
                }
                return {
                  label: chosen ? t('registration.addDrop.chosen') : t('registration.addDrop.addToRequest'),
                  disabled: chosen || sameCourse || !s.available,
                  onClick: () => setAdds([...adds, s]),
                }
              }}
            />
          )}
        </div>
        <div className="space-y-2">
          <Label htmlFor="add-drop-note">{t('registration.addDrop.note')}</Label>
          <Textarea
            id="add-drop-note"
            value={note}
            maxLength={1000}
            onChange={(e) => setNote(e.target.value)}
          />
        </div>
        <div className="flex flex-wrap items-center justify-between gap-3">
          <p className="text-sm text-muted-foreground">
            {t('registration.addDrop.summary', { adds: adds.length, drops: drops.length, delta })}
          </p>
          <div className="flex gap-2">
            <Button variant="outline" onClick={onDone}>
              {t('common.cancel')}
            </Button>
            <Button loading={isLoading} disabled={adds.length + drops.length === 0} onClick={submit}>
              {t('registration.addDrop.send')}
            </Button>
          </div>
        </div>
      </CardContent>
    </Card>
  )
}
