import { CalendarClockIcon, CircleCheckIcon, CircleXIcon, ShoppingBasketIcon, Trash2Icon } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { formatDateTime } from '@/i18n/format'
import type { Registration, RegistrationItem } from '@/shared/api/generated'
import { useLocalized } from '@/shared/lib/localized'
import { cn } from '@/shared/lib/utils'
import { Alert, AlertDescription, AlertTitle } from '@/shared/ui/alert'
import { Button } from '@/shared/ui/button'
import { IconChip } from '@/shared/ui/icon-chip'
import { Meter } from '@/shared/ui/meter'
import { toneFill, toneFor, toneSoft } from '@/shared/ui/tone'

import { ItemKindBadge, RegistrationStatusBadge } from './badges'
import { SectionSchedule } from './SectionInfo'

/** EctsMeter, sepetin AKTS'sini GANO'ya göre üst sınırla birlikte gösterir. */
export function EctsMeter({ total, limit }: { total: number; limit: number }) {
  const { t } = useTranslation()
  const ratio = limit > 0 ? total / limit : 0
  const over = total > limit
  return (
    <div className="space-y-1.5">
      <div className="flex items-baseline justify-between text-sm">
        <span className="font-semibold text-muted-foreground">{t('registration.basket.ects')}</span>
        <span className={cn('font-bold tabular-nums', over && 'text-destructive')}>
          {t('registration.basket.ectsOf', { total, limit })}
        </span>
      </div>
      <Meter
        value={total}
        max={limit}
        label={t('registration.basket.ects')}
        tone={over ? 'rose' : ratio > 0.9 ? 'amber' : 'violet'}
        size="lg"
      />
    </div>
  )
}

/**
 * RegistrationState, kaydın durumu, pencereler ve danışman kararıdır: reddedilen kayıtta
 * danışman notu, onaylanan kayıtta onay bilgisi.
 */
export function RegistrationState({ registration: r }: { registration: Registration }) {
  const { t } = useTranslation()
  const w = r.registration_window
  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center gap-2 text-sm">
        <RegistrationStatusBadge status={r.status} />
        <span
          className={cn(
            'inline-flex items-center gap-1.5 rounded-md px-2.5 py-0.5 text-xs font-semibold',
            w.open || w.extended
              ? 'bg-emerald-500/10 text-emerald-700 dark:text-emerald-300'
              : 'bg-muted text-muted-foreground',
          )}
        >
          <CalendarClockIcon className="size-3.5" aria-hidden />
          {w.open && w.ends_at
            ? t('registration.window.openUntil', {
                time: formatDateTime(new Date(new Date(w.ends_at).getTime() - 60_000)),
              })
            : w.extended && r.extension_until
              ? t('registration.window.extendedUntil', { time: formatDateTime(r.extension_until) })
              : w.next_starts_at
                ? t('registration.window.opensAt', { time: formatDateTime(w.next_starts_at) })
                : t('registration.window.closed')}
        </span>
      </div>
      {r.status === 'REJECTED' && (
        <Alert variant="destructive">
          <CircleXIcon />
          <AlertTitle>{t('registration.decision.rejected', { by: r.decided_by ?? '' })}</AlertTitle>
          {r.decision_note && <AlertDescription>{r.decision_note}</AlertDescription>}
        </Alert>
      )}
      {r.status === 'APPROVED' && (
        <Alert>
          <CircleCheckIcon className="text-success" />
          <AlertTitle>
            {t('registration.decision.approved', {
              by: r.decided_by ?? '',
              time: formatDateTime(r.decided_at),
            })}
          </AlertTitle>
          {r.decision_note && <AlertDescription>{r.decision_note}</AlertDescription>}
        </Alert>
      )}
      {r.status === 'SUBMITTED' && (
        <p className="text-sm text-muted-foreground">
          {t('registration.decision.submitted', { time: formatDateTime(r.submitted_at) })}
        </p>
      )}
    </div>
  )
}

/** BasketItems, sepetteki derslerdir. onRemove verilirse dersler çıkarılabilir. */
export function BasketItems({
  items,
  onRemove,
  busy,
}: {
  items: RegistrationItem[]
  onRemove?: (item: RegistrationItem) => void
  busy?: boolean
}) {
  const { t } = useTranslation()
  const localized = useLocalized()
  if (items.length === 0) {
    return (
      <div className="flex flex-col items-center gap-2 rounded-xl border border-dashed border-border px-4 py-6 text-center">
        <IconChip icon={ShoppingBasketIcon} tone="violet" size="lg" />
        <p className="text-sm text-muted-foreground">{t('registration.basket.empty')}</p>
      </div>
    )
  }
  return (
    <ul className="space-y-2" aria-label={t('registration.basket.title')}>
      {items.map((it) => {
        const tone = toneFor(it.section.course.code)
        return (
          <li
            key={it.id}
            className="relative flex items-start gap-3 overflow-hidden rounded-xl border border-border/70 bg-card py-2.5 pr-2 pl-4 shadow-xs"
          >
            <span aria-hidden className={cn('absolute inset-y-0 left-0 w-1', toneFill[tone])} />
            <div className="min-w-0 flex-1 space-y-1 text-sm">
              <p className="flex flex-wrap items-center gap-x-2 gap-y-1">
                <span className="font-bold">{it.section.label}</span>
                <ItemKindBadge kind={it.kind} />
              </p>
              <p className="truncate text-[13px] text-muted-foreground">{localized(it.section.course)}</p>
              <p className="text-xs">
                <SectionSchedule section={it.section} compact />
              </p>
            </div>
            <div className="flex shrink-0 items-center gap-0.5">
              <span className={cn('rounded-md px-2 py-0.5 text-xs font-bold tabular-nums', toneSoft[tone])}>
                {t('registration.ects', { ects: it.ects })}
              </span>
              {onRemove && (
                <Button
                  variant="ghost"
                  size="icon-sm"
                  className="text-muted-foreground hover:bg-rose-500/10 hover:text-rose-600"
                  disabled={busy}
                  onClick={() => onRemove(it)}
                  aria-label={t('registration.basket.remove', { code: it.section.course.code })}
                >
                  <Trash2Icon />
                </Button>
              )}
            </div>
          </li>
        )
      })}
    </ul>
  )
}
