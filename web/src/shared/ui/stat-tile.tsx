import type { LucideIcon } from 'lucide-react'
import type { ReactNode } from 'react'

import { cn } from '@/shared/lib/utils'

import { IconChip } from './icon-chip'
import { toneText, type Tone } from './tone'

/** StatTile, bir sayıyı etiketi ve renkli bir ikonla gösteren kutudur (dt/dd çifti). */
export function StatTile({
  label,
  value,
  suffix,
  icon,
  tone = 'indigo',
  className,
}: {
  label: ReactNode
  value: ReactNode
  /** suffix, değerin yanındaki küçük birimdir (ör. AKTS). */
  suffix?: ReactNode
  icon?: LucideIcon
  tone?: Tone
  className?: string
}) {
  return (
    <div
      className={cn(
        'flex items-center gap-3 rounded-xl border border-border/70 bg-card p-3 shadow-xs',
        className,
      )}
    >
      {icon && <IconChip icon={icon} tone={tone} />}
      <div className="min-w-0">
        <dt className="text-xs leading-tight font-semibold text-muted-foreground">{label}</dt>
        <dd className={cn('text-lg font-extrabold whitespace-nowrap tabular-nums', toneText[tone])}>
          {value}
          {suffix && (
            <>
              {' '}
              <span className="text-xs font-semibold text-muted-foreground">{suffix}</span>
            </>
          )}
        </dd>
      </div>
    </div>
  )
}
