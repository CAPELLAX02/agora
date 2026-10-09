import { Loader2 } from 'lucide-react'
import type { ComponentProps } from 'react'

import { cn } from '@/shared/lib/utils'

/** Spinner, dönen bir yükleme göstergesidir. label verilmezse ekran okuyucudan gizlenir. */
export function Spinner({ className, label, ...props }: ComponentProps<'svg'> & { label?: string }) {
  return (
    <Loader2
      role={label ? 'status' : undefined}
      aria-label={label}
      aria-hidden={label ? undefined : true}
      className={cn('size-4 animate-spin', className)}
      {...props}
    />
  )
}
