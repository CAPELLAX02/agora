import { CalendarClockIcon, CircleCheckIcon, CircleXIcon, Trash2Icon } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { formatDateTime } from '@/i18n/format'
import type { Registration, RegistrationItem } from '@/shared/api/generated'
import { useLocalized } from '@/shared/lib/localized'
import { cn } from '@/shared/lib/utils'
import { Alert, AlertDescription, AlertTitle } from '@/shared/ui/alert'
import { Button } from '@/shared/ui/button'

import { ItemKindBadge, RegistrationStatusBadge } from './badges'
import { SectionSchedule } from './SectionInfo'

/** EctsMeter, sepetin AKTS'sini GANO'ya göre üst sınırla birlikte gösterir. */
export function EctsMeter({ total, limit }: { total: number; limit: number }) {
  const { t } = useTranslation()
  const ratio = limit > 0 ? Math.min(total / limit, 1) : 0
  const over = total > limit
  return (
    <div className="space-y-1">
      <div className="flex items-baseline justify-between text-sm">
        <span className="text-muted-foreground">{t('registration.basket.ects')}</span>
        <span className={cn('font-semibold tabular-nums', over && 'text-destructive')}>
          {t('registration.basket.ectsOf', { total, limit })}
        </span>
      </div>
      <div
        className="h-2 overflow-hidden rounded-full bg-muted"
        role="meter"
        aria-label={t('registration.basket.ects')}
        aria-valuemin={0}
        aria-valuemax={limit}
        aria-valuenow={total}
      >
        <div
          className={cn(
            'h-full rounded-full',
            over ? 'bg-destructive' : ratio > 0.9 ? 'bg-warning' : 'bg-primary',
          )}
          style={{ width: `${ratio * 100}%` }}
        />
      </div>
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
        <span className="flex items-center gap-1 text-muted-foreground">
          <CalendarClockIcon className="size-4" aria-hidden />
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
    return <p className="py-4 text-center text-sm text-muted-foreground">{t('registration.basket.empty')}</p>
  }
  return (
    <ul className="divide-y" aria-label={t('registration.basket.title')}>
      {items.map((it) => (
        <li key={it.id} className="flex items-start justify-between gap-3 py-2.5">
          <div className="min-w-0 space-y-0.5 text-sm">
            <p className="flex flex-wrap items-center gap-2">
              <span className="font-medium">{it.section.label}</span>
              <span className="truncate">{localized(it.section.course)}</span>
              <ItemKindBadge kind={it.kind} />
            </p>
            <p className="text-xs text-muted-foreground">
              <SectionSchedule section={it.section} />
            </p>
          </div>
          <div className="flex shrink-0 items-center gap-1">
            <span className="text-sm tabular-nums">{t('registration.ects', { ects: it.ects })}</span>
            {onRemove && (
              <Button
                variant="ghost"
                size="icon"
                disabled={busy}
                onClick={() => onRemove(it)}
                aria-label={t('registration.basket.remove', { code: it.section.course.code })}
              >
                <Trash2Icon />
              </Button>
            )}
          </div>
        </li>
      ))}
    </ul>
  )
}
