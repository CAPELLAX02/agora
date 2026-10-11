import { CircleAlertIcon, TriangleAlertIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import type { RuleViolation } from '@/shared/api/generated'
import { cn } from '@/shared/lib/utils'

import { useViolationText } from './shared'

/**
 * ViolationList, kural motorunun sonuçlarıdır: hatalar (işlemi ya da gönderimi engeller)
 * önce, uyarılar (bilgi) sonra.
 */
export function ViolationList({
  violations,
  className,
}: {
  violations: RuleViolation[]
  className?: string
}) {
  const { t } = useTranslation()
  const text = useViolationText()
  if (violations.length === 0) {
    return null
  }
  const sorted = [...violations].sort((a, b) =>
    a.severity === b.severity ? 0 : a.severity === 'ERROR' ? -1 : 1,
  )
  return (
    <ul className={cn('space-y-2', className)} aria-label={t('registration.rules.title')}>
      {sorted.map((v, i) => {
        const error = v.severity === 'ERROR'
        const Icon = error ? CircleAlertIcon : TriangleAlertIcon
        return (
          <li
            key={`${v.code}-${i}`}
            className={cn(
              'flex gap-2.5 rounded-xl border px-3 py-2.5 text-sm leading-snug',
              error
                ? 'border-rose-500/25 bg-rose-500/[0.07] text-rose-950 dark:text-rose-100'
                : 'border-amber-500/30 bg-amber-500/[0.09] text-amber-950 dark:text-amber-100',
            )}
          >
            <Icon
              className={cn(
                'mt-0.5 size-4 shrink-0',
                error ? 'text-rose-600 dark:text-rose-400' : 'text-amber-600 dark:text-amber-400',
              )}
              aria-hidden
            />
            <span>
              <span className="sr-only">
                {t(error ? 'registration.rules.error' : 'registration.rules.warning')}:{' '}
              </span>
              {text(v)}
            </span>
          </li>
        )
      })}
    </ul>
  )
}
