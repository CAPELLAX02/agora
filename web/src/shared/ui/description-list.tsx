import type { ReactNode } from 'react'

import { cn } from '@/shared/lib/utils'

/** DescriptionList, etiket-değer çiftlerini iki sütunlu gösterir. */
export function DescriptionList({
  items,
  className,
}: {
  items: [ReactNode, ReactNode][]
  className?: string
}) {
  return (
    <dl className={cn('grid gap-x-6 gap-y-4 sm:grid-cols-2', className)}>
      {items.map(([label, value], i) => (
        <div key={i} className="space-y-1">
          <dt className="text-xs font-medium tracking-wide text-muted-foreground uppercase">{label}</dt>
          <dd className="text-sm break-words">{value}</dd>
        </div>
      ))}
    </dl>
  )
}
