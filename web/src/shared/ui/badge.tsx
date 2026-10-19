import { cva, type VariantProps } from 'class-variance-authority'
import type { ComponentProps } from 'react'

import { cn } from '@/shared/lib/utils'

const badgeVariants = cva(
  'inline-flex w-fit shrink-0 items-center justify-center gap-1 overflow-hidden rounded-md border px-2 py-0.5 text-xs font-semibold whitespace-nowrap [&>svg]:pointer-events-none [&>svg]:size-3',
  {
    variants: {
      variant: {
        default: 'border-transparent bg-primary text-primary-foreground',
        secondary: 'border-transparent bg-secondary text-secondary-foreground',
        outline: 'border-border bg-card text-foreground',
        success:
          'border-transparent bg-emerald-500/12 text-emerald-700 dark:bg-emerald-400/15 dark:text-emerald-300',
        warning: 'border-transparent bg-amber-500/15 text-amber-800 dark:bg-amber-400/15 dark:text-amber-300',
        destructive: 'border-transparent bg-rose-500/12 text-rose-700 dark:bg-rose-400/15 dark:text-rose-300',
        info: 'border-transparent bg-sky-500/12 text-sky-700 dark:bg-sky-400/15 dark:text-sky-300',
        violet:
          'border-transparent bg-violet-500/12 text-violet-700 dark:bg-violet-400/15 dark:text-violet-300',
        muted: 'border-transparent bg-muted text-muted-foreground',
      },
    },
    defaultVariants: { variant: 'default' },
  },
)

export function Badge({
  className,
  variant,
  ...props
}: ComponentProps<'span'> & VariantProps<typeof badgeVariants>) {
  return <span data-slot="badge" className={cn(badgeVariants({ variant }), className)} {...props} />
}
