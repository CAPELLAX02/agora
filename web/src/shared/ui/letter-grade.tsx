import { cn } from '@/shared/lib/utils'

/** Harf notlarının rengi: başarıya göre yeşilden ambere, başarısızlıkta kırmızı. */
const colors: Record<string, string> = {
  A: 'bg-emerald-500 text-white',
  B1: 'bg-emerald-500/85 text-white',
  B2: 'bg-teal-500 text-white',
  B3: 'bg-sky-500 text-white',
  C1: 'bg-indigo-500 text-white',
  C2: 'bg-violet-500 text-white',
  C3: 'bg-amber-500 text-white',
  F1: 'bg-rose-600 text-white',
  F2: 'bg-rose-500 text-white',
  BŞR: 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-300',
  BŞZ: 'bg-rose-500/15 text-rose-700 dark:text-rose-300',
  MUAF: 'bg-sky-500/15 text-sky-700 dark:text-sky-300',
}

/** LetterGrade, bir harf notunu renkli bir rozet olarak gösterir. */
export function LetterGrade({ letter, className }: { letter: string; className?: string }) {
  return (
    <span
      className={cn(
        'inline-flex min-w-9 items-center justify-center rounded-lg px-1.5 py-0.5 font-mono text-xs font-extrabold shadow-xs',
        colors[letter] ?? 'bg-muted text-muted-foreground',
        className,
      )}
    >
      {letter}
    </span>
  )
}
