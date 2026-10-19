import type { LucideIcon } from 'lucide-react'
import type { ReactNode } from 'react'

import { cn } from '@/shared/lib/utils'

import { IconChip } from './icon-chip'
import type { Tone } from './tone'

/**
 * PageHeader, sayfanın başlığı, açıklaması ve eylemleridir. icon verilirse başlığın
 * solunda sayfanın rengini taşıyan bir ikon kutusu gösterilir (yan menüdekiyle aynı).
 */
export function PageHeader({
  title,
  description,
  actions,
  icon,
  tone = 'indigo',
  className,
}: {
  title: ReactNode
  description?: ReactNode
  actions?: ReactNode
  icon?: LucideIcon
  tone?: Tone
  className?: string
}) {
  return (
    <div className={cn('mb-7 flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between', className)}>
      <div className="flex min-w-0 items-center gap-4">
        {icon && <IconChip icon={icon} tone={tone} size="lg" solid className="hidden sm:inline-flex" />}
        <div className="min-w-0 space-y-1">
          <h1 className="text-2xl font-bold tracking-tight sm:text-[1.65rem]">{title}</h1>
          {description && <p className="text-sm text-muted-foreground">{description}</p>}
        </div>
      </div>
      {actions && <div className="flex shrink-0 flex-wrap gap-2">{actions}</div>}
    </div>
  )
}
