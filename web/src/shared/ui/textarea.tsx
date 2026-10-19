import type { ComponentProps } from 'react'

import { cn } from '@/shared/lib/utils'

export function Textarea({ className, ...props }: ComponentProps<'textarea'>) {
  return (
    <textarea
      data-slot="textarea"
      className={cn(
        'flex field-sizing-content min-h-20 w-full rounded-lg border field-focus border-input bg-card px-3.5 py-2.5 text-base shadow-xs transition-[color,box-shadow,border-color] placeholder:text-muted-foreground/80 disabled:cursor-not-allowed disabled:opacity-50 md:text-sm dark:bg-input/30',
        'aria-invalid:border-destructive',
        className,
      )}
      {...props}
    />
  )
}
