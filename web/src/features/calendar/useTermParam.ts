import { useSearchParams } from 'react-router'

import { useGetCurrentTermQuery, useListTermsQuery } from '@/shared/api/generated'

/**
 * useTermParam, adresteki dönemi (?donem=) okur; verilmemişse içinde bulunulan dönem, o da
 * yoksa en yeni dönem seçilir. Dönem değişince adres güncellenir.
 */
export function useTermParam(): [string, (id: string) => void] {
  const [params, setParams] = useSearchParams()
  const terms = useListTermsQuery()
  const current = useGetCurrentTermQuery()
  const termId =
    params.get('donem') ?? current.data?.id ?? (current.isLoading ? '' : (terms.data?.items[0]?.id ?? ''))
  const setTermId = (id: string) => {
    const next = new URLSearchParams(params)
    next.set('donem', id)
    setParams(next, { replace: true })
  }
  return [termId, setTermId]
}
