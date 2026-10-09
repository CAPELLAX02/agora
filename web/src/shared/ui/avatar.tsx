import { cn } from '@/shared/lib/utils'

/** Avatar, kişinin baş harflerini gösterir. Fotoğraf altyapısı (MinIO) Faz 5'te. */
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
        'inline-flex size-8 shrink-0 items-center justify-center rounded-full bg-primary/10 text-xs font-semibold text-primary',
        className,
      )}
    >
      {initials}
    </span>
  )
}
