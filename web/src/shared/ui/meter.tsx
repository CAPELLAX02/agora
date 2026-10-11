import { cn } from '@/shared/lib/utils'

import { toneFill, type Tone } from './tone'

/** Meter, bir değerin üst sınıra oranını renkli bir çubukla gösterir (role="meter"). */
export function Meter({
  value,
  max,
  label,
  tone = 'indigo',
  size = 'md',
  className,
}: {
  value: number
  max: number
  /** label, çubuğun erişilebilir adıdır. */
  label: string
  tone?: Tone
  size?: 'sm' | 'md' | 'lg'
  className?: string
}) {
  const ratio = max > 0 ? Math.min(Math.max(value / max, 0), 1) : 0
  return (
    <div
      role="meter"
      aria-label={label}
      aria-valuemin={0}
      aria-valuemax={max}
      aria-valuenow={value}
      className={cn(
        'overflow-hidden rounded-full bg-muted',
        size === 'sm' ? 'h-1.5' : size === 'lg' ? 'h-3' : 'h-2',
        className,
      )}
    >
      <div
        className={cn('h-full rounded-full transition-[width] duration-500 ease-out', toneFill[tone])}
        style={{ width: `${ratio * 100}%` }}
      />
    </div>
  )
}
