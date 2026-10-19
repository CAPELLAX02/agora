import { cn } from '@/shared/lib/utils'

import { letterColors } from './letter-colors'

/** LetterGrade, bir harf notunu renkli bir rozet olarak gösterir. */
export function LetterGrade({ letter, className }: { letter: string; className?: string }) {
  return (
    <span
      className={cn(
        'inline-flex min-w-9 items-center justify-center rounded-md px-1.5 py-0.5 font-mono text-xs font-bold shadow-xs',
        letterColors[letter] ?? 'bg-muted text-muted-foreground',
        className,
      )}
    >
      {letter}
    </span>
  )
}
