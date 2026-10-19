import { useTranslation } from 'react-i18next'

import logo from '@/assets/ankara-universitesi.png'
import { cn } from '@/shared/lib/utils'

/** BrandMark, Ankara Üniversitesi'nin amblemidir (süs: yanındaki metin markayı söyler). */
export function BrandMark({ className }: { className?: string }) {
  return (
    <img
      src={logo}
      alt=""
      aria-hidden
      width={318}
      height={318}
      draggable={false}
      className={cn('size-9 shrink-0 rounded-full select-none', className)}
    />
  )
}

/**
 * Brand, uygulamanın kimliğidir: amblem, "Ankara Üniversitesi" ve altında "Agora Öğrenci Bilgi
 * Sistemi". inverse, koyu ya da renkli zeminde beyaz metin kullanır.
 */
export function Brand({
  className,
  inverse = false,
  compact = false,
}: {
  className?: string
  inverse?: boolean
  /** compact, dar ekranda (sm altı) sadece amblemi gösterir; ad ekran okuyucuya kalır. */
  compact?: boolean
}) {
  const { t } = useTranslation()
  return (
    <span className={cn('flex min-w-0 items-center gap-2.5', className)}>
      <BrandMark className={cn(inverse && 'bg-white p-px shadow-md')} />
      <span className={cn('flex min-w-0 flex-col', compact && 'max-sm:sr-only')}>
        <span className="truncate text-[15px] leading-tight font-bold tracking-tight">
          {t('brand.university')}
        </span>
        <span
          className={cn(
            'truncate text-[11px] leading-tight font-medium',
            inverse ? 'text-white/75' : 'text-muted-foreground',
          )}
        >
          {t('brand.product')}
        </span>
      </span>
    </span>
  )
}
