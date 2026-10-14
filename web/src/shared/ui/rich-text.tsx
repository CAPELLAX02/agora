import { cn } from '@/shared/lib/utils'

/**
 * RichText, sunucunun izin listesiyle temizlediği HTML'i gösterir. İçerik sunucuda
 * temizlendiği için doğrudan yazılır (ders içeriği, sayfa, tanıtım metni).
 */
export function RichText({ html, className }: { html: string; className?: string }) {
  if (!html) {
    return null
  }
  return <div className={cn('rich-text', className)} dangerouslySetInnerHTML={{ __html: html }} />
}
