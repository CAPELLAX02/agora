import { cn } from '@/shared/lib/utils'

/** Gpa, genel not ortalamasını başarıya göre renklendirerek yazar (yoksa tire). */
export function Gpa({ value, className }: { value: number | null | undefined; className?: string }) {
  if (value === null || value === undefined) {
    return <span className={cn('text-muted-foreground', className)}>—</span>
  }
  return (
    <span
      className={cn(
        'font-bold tabular-nums',
        value >= 3
          ? 'text-emerald-700 dark:text-emerald-300'
          : value < 2
            ? 'text-rose-700 dark:text-rose-300'
            : 'text-foreground',
        className,
      )}
    >
      {value.toFixed(2)}
    </span>
  )
}
