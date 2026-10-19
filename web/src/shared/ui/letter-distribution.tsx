import { useTranslation } from 'react-i18next'

import type { LetterDistribution } from '@/shared/api/generated'
import { cn } from '@/shared/lib/utils'

import { letterBars } from './letter-colors'

/**
 * LetterDistributionChart, harf notu dağılımının çubuk grafiğidir: her harf için öğrenci
 * sayısı, en kalabalık harfe göre yükseklik. Ekran okuyucu için liste olarak da okunur.
 */
export function LetterDistributionChart({
  distribution,
  highlight,
  className,
}: {
  distribution: LetterDistribution
  /** highlight, vurgulanan harftir (ör. öğrencinin kendi notu). */
  highlight?: string | null
  className?: string
}) {
  const { t } = useTranslation()
  const max = Math.max(1, ...distribution.map((d) => d.count))
  return (
    <ul
      aria-label={t('grades.distribution')}
      className={cn('flex h-44 items-end gap-1.5 sm:gap-2', className)}
    >
      {distribution.map((d) => (
        <li key={d.letter} className="flex h-full min-w-0 flex-1 flex-col items-center justify-end gap-1">
          <span className="text-xs font-bold tabular-nums">{d.count}</span>
          <span
            aria-hidden
            className={cn(
              'w-full max-w-12 rounded-t-sm transition-all',
              letterBars[d.letter] ?? 'bg-muted-foreground/40',
              d.count === 0 && 'opacity-25',
              highlight === d.letter && 'ring-4 ring-primary/40',
            )}
            style={{ height: `${Math.max(4, (d.count / max) * 100)}%` }}
          />
          <span className="font-mono text-xs font-bold">{d.letter}</span>
          <span className="sr-only">
            {t('grades.distributionItem', { letter: d.letter, count: d.count })}
          </span>
        </li>
      ))}
    </ul>
  )
}
