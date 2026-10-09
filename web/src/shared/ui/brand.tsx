import { cn } from '@/shared/lib/utils'

/** BrandMark, Agora'nın işaretidir (favicon ile aynı çizim). */
export function BrandMark({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 32 32" aria-hidden className={cn('size-8', className)}>
      <rect width="32" height="32" rx="5" className="fill-primary" />
      <path
        d="M16 6 7 26h4.2l1.7-4h6.2l1.7 4H25L16 6Zm-1.7 12.4L16 14l1.7 4.4h-3.4Z"
        className="fill-primary-foreground"
      />
      <circle cx="24" cy="8" r="2.5" className="fill-highlight" />
    </svg>
  )
}

export function Brand({ className }: { className?: string }) {
  return (
    <span className={cn('flex items-center gap-2.5 font-semibold tracking-tight', className)}>
      <BrandMark />
      <span className="text-lg">Agora</span>
    </span>
  )
}
