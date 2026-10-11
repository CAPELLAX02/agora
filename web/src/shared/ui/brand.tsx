import { useId } from 'react'

import { cn } from '@/shared/lib/utils'

/** BrandMark, Agora'nın işaretidir (favicon ile aynı çizim, marka geçişiyle). */
export function BrandMark({ className }: { className?: string }) {
  const id = useId()
  return (
    <svg viewBox="0 0 32 32" aria-hidden className={cn('size-8 drop-shadow-sm', className)}>
      <defs>
        <linearGradient id={id} x1="0" y1="0" x2="1" y2="1">
          <stop offset="0" stopColor="oklch(0.5 0.22 268)" />
          <stop offset="0.55" stopColor="oklch(0.56 0.24 295)" />
          <stop offset="1" stopColor="oklch(0.66 0.21 345)" />
        </linearGradient>
      </defs>
      <rect width="32" height="32" rx="9" fill={`url(#${id})`} />
      <path d="M16 6 7 26h4.2l1.7-4h6.2l1.7 4H25L16 6Zm-1.7 12.4L16 14l1.7 4.4h-3.4Z" fill="white" />
      <circle cx="24.5" cy="7.5" r="2.5" className="fill-highlight" />
    </svg>
  )
}

export function Brand({ className }: { className?: string }) {
  return (
    <span className={cn('flex items-center gap-2.5 font-extrabold tracking-tight', className)}>
      <BrandMark />
      <span className="text-lg">Agora</span>
    </span>
  )
}
