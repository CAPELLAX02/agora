import type { ComponentProps } from 'react'

import { cn } from '@/shared/lib/utils'

export function Input({ className, type, ...props }: ComponentProps<'input'>) {
  return (
    <input
      type={type}
      data-slot="input"
      className={cn(
        'flex h-10 w-full min-w-0 rounded-lg border field-focus border-input bg-card px-3.5 py-1 text-base shadow-xs transition-[color,box-shadow,border-color] placeholder:text-muted-foreground/80 disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50 md:text-sm dark:bg-input/30',
        'aria-invalid:border-destructive',
        className,
      )}
      {...props}
    />
  )
}
