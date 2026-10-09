import { useMemo } from 'react'

import { useGetMyPermissionsQuery } from '@/shared/api/generated'

/**
 * usePermissions, kullanıcının bu oturumdaki etkin yetkilerini döndürür. Arayüz menü ve
 * düğmeleri buna göre gösterir. Asıl kontrol her zaman sunucudadır.
 */
export function usePermissions() {
  const { data, isLoading, isError } = useGetMyPermissionsQuery()
  return useMemo(() => {
    const codes = new Set(data?.items.map((g) => g.permission))
    return { has: (permission: string) => codes.has(permission), isLoading, isError }
  }, [data, isLoading, isError])
}
