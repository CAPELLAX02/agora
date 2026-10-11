import { useMemo } from 'react'

import { useGetMyPermissionsQuery, type Grant } from '@/shared/api/generated'

/**
 * usePermissions, kullanıcının bu oturumdaki etkin yetkilerini döndürür. Arayüz menü ve
 * düğmeleri buna göre gösterir. Asıl kontrol her zaman sunucudadır. grantsOf, bir yetkinin
 * hangi kapsamlarda verildiğidir (ör. bölüm başkanının bölümü varsayılan seçilir).
 */
export function usePermissions() {
  const { data, isLoading, isError } = useGetMyPermissionsQuery()
  return useMemo(() => {
    const items = data?.items ?? []
    const codes = new Set(items.map((g) => g.permission))
    return {
      has: (permission: string) => codes.has(permission),
      /**
       * hasScoped, yetkinin bir birimi kapsayan (NONE olmayan) bir verilişi olduğunu söyler.
       * Danışmanın öğrenci okuma yetkisi ilişkiye dayalıdır (NONE): öğrenci listesi ona boş döner.
       */
      hasScoped: (permission: string) =>
        items.some((g) => g.permission === permission && g.scope_type !== 'NONE'),
      /** hasAny, verilen yetkilerden en az birinin olup olmadığını söyler. */
      hasAny: (permission: string | string[]) =>
        (Array.isArray(permission) ? permission : [permission]).some((p) => codes.has(p)),
      grantsOf: (permission: string): Grant[] => items.filter((g) => g.permission === permission),
      isLoading,
      isError,
    }
  }, [data, isLoading, isError])
}
