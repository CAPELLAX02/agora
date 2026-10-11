import type { LucideIcon } from 'lucide-react'
import type { ReactNode } from 'react'

import { cn } from '@/shared/lib/utils'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/shared/ui/card'
import { IconChip } from '@/shared/ui/icon-chip'
import type { Tone } from '@/shared/ui/tone'

/** DashCard, ana sayfa kartıdır: renkli ikon kutusu, başlık, açıklama ve içerik. */
export function DashCard({
  icon,
  tone,
  title,
  description,
  action,
  className,
  children,
}: {
  icon: LucideIcon
  tone: Tone
  title: ReactNode
  description?: ReactNode
  action?: ReactNode
  className?: string
  children: ReactNode
}) {
  return (
    <Card className={cn('gap-5', className)}>
      <CardHeader className="flex items-center gap-3">
        <IconChip icon={icon} tone={tone} />
        <div className="min-w-0 flex-1 space-y-0.5">
          <CardTitle>{title}</CardTitle>
          {description && <CardDescription>{description}</CardDescription>}
        </div>
        {action}
      </CardHeader>
      <CardContent className="flex flex-1 flex-col gap-4">{children}</CardContent>
    </Card>
  )
}
