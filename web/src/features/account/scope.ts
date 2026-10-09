import { useTranslation } from 'react-i18next'

import { useListRolesQuery, type RoleAssignment } from '@/shared/api/generated'

/** useScopeLabel, bir rol atamasının kapsamını okunur hale getirir. */
export function useScopeLabel() {
  const { t } = useTranslation()
  return (r: Pick<RoleAssignment, 'scope_type' | 'scope_name'>) =>
    r.scope_type === 'UNIVERSITY'
      ? t('profile.university')
      : r.scope_type === 'NONE'
        ? t('profile.noScope')
        : (r.scope_name ?? t(`scope.${r.scope_type}`))
}

/**
 * useRoleName, rolün kullanıcının dilindeki adını döndürür. Atama yanıtları rol adını
 * Türkçe taşır, İngilizce ad rol kataloğundan gelir (katalog önbellekte tutulur).
 */
export function useRoleName() {
  const { i18n } = useTranslation()
  const en = i18n.language === 'en'
  const { data } = useListRolesQuery(undefined, { skip: !en })
  return (r: Pick<RoleAssignment, 'role' | 'role_name'>) =>
    (en && data?.items.find((d) => d.code === r.role)?.name_en) || r.role_name
}
