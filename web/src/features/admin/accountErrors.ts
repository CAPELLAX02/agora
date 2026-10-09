import { useTranslation } from 'react-i18next'

import type { Messages } from '@/shared/api/useErrorMessage'

/** useAccountErrors, hesap ve rol yönetimi uçlarının hata kodlarının metinleridir. */
export function useAccountErrors(): Messages {
  const { t } = useTranslation()
  const codes = [
    'ACCOUNT_EXISTS',
    'SELF_ACTION_FORBIDDEN',
    'INVALID_STATUS_CHANGE',
    'ACCOUNT_NOT_PENDING',
    'ACCOUNT_NOT_ACTIVE',
    'MFA_NOT_ENABLED',
    'LAST_SYSTEM_ADMIN',
    'ROLE_ALREADY_ASSIGNED',
  ] as const
  return Object.fromEntries(codes.map((c) => [c, t(`users.errors.${c}`)]))
}
