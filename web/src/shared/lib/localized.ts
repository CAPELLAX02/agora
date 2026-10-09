import { useCallback } from 'react'
import { useTranslation } from 'react-i18next'

type Named = { name_tr: string; name_en?: string | null | undefined }

/**
 * useLocalized, iki dilde adı olan bir kaydın kullanıcının dilindeki adını döndürür.
 * İngilizce ad yoksa Türkçe ad gösterilir.
 */
export function useLocalized() {
  const { i18n } = useTranslation()
  const en = i18n.language === 'en'
  return useCallback((r: Named) => (en && r.name_en) || r.name_tr, [en])
}
