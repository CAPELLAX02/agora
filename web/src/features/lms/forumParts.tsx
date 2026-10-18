import { cn } from '@/shared/lib/utils'

/** Initials, yazarın baş harfleridir: öğretim elemanı mor, öğrenci mavi tonda. */
export function Initials({
  name,
  instructor,
  className,
}: {
  name: string
  instructor: boolean
  className?: string
}) {
  const letters = name
    .split(/\s+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((p) => p[0]!.toLocaleUpperCase('tr-TR'))
    .join('')
  return (
    <span
      aria-hidden
      className={cn(
        'flex size-9 shrink-0 items-center justify-center rounded-full bg-gradient-to-br text-xs font-bold text-white',
        instructor ? 'from-violet-500 to-indigo-600' : 'from-sky-500 to-cyan-600',
        className,
      )}
    >
      {letters}
    </span>
  )
}
