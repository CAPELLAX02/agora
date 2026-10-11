import { cn } from '@/shared/lib/utils'

import { toneFor, toneSolid } from './tone'

/**
 * Avatar, kişinin baş harflerini adına göre seçilen renkli bir zeminde gösterir: aynı kişi
 * her yerde aynı renktedir. Fotoğraf altyapısı (MinIO) Faz 5'te.
 */
export function Avatar({
  firstName,
  lastName,
  className,
}: {
  firstName: string
  lastName: string
  className?: string
}) {
  const initials = `${firstName.charAt(0)}${lastName.charAt(0)}`.toLocaleUpperCase('tr')
  return (
    <span
      aria-hidden
      className={cn(
        'inline-flex size-8 shrink-0 items-center justify-center rounded-full text-xs font-bold tracking-wide shadow-sm ring-2 ring-card',
        toneSolid[toneFor(`${firstName} ${lastName}`)],
        className,
      )}
    >
      {initials}
    </span>
  )
}
