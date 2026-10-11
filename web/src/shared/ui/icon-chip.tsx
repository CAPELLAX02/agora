import type { LucideIcon } from 'lucide-react'

import { cn } from '@/shared/lib/utils'

import { toneSoft, toneSolid, type Tone } from './tone'

const sizes = {
  sm: 'size-7 rounded-lg [&>svg]:size-3.5',
  md: 'size-9 rounded-xl [&>svg]:size-[18px]',
  lg: 'size-11 rounded-xl [&>svg]:size-5',
  xl: 'size-14 rounded-2xl [&>svg]:size-7',
} as const

/** IconChip, bir ikonu renkli, yuvarlak köşeli bir kutuda gösterir. */
export function IconChip({
  icon: Icon,
  tone = 'indigo',
  size = 'md',
  solid = false,
  className,
}: {
  icon: LucideIcon
  tone?: Tone
  size?: keyof typeof sizes
  /** solid, zemini dolgun (geçişli) yapar. */
  solid?: boolean
  className?: string
}) {
  return (
    <span
      aria-hidden
      className={cn(
        'inline-flex shrink-0 items-center justify-center',
        sizes[size],
        solid ? cn(toneSolid[tone], 'shadow-sm') : toneSoft[tone],
        className,
      )}
    >
      <Icon />
    </span>
  )
}
