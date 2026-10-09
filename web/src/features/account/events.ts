import { useTranslation } from 'react-i18next'

import type { SecurityEvent } from '@/shared/api/generated'

const reasons = [
  'unknown_user',
  'wrong_password',
  'account_locked',
  'account_inactive',
  'wrong_otp',
  'wrong_recovery_code',
  'mfa_unavailable',
  'password_change_wrong_current',
  'mfa_enable_wrong_password',
  'mfa_disable_wrong_password',
] as const

type Reason = (typeof reasons)[number]

/** useEventDetail, bir güvenlik olayının ayrıntısını (ör. başarısızlık sebebi) okunur hale getirir. */
export function useEventDetail() {
  const { t } = useTranslation()
  return (e: SecurityEvent): string => {
    const reason = e.details.reason as string | undefined
    if (reason && reasons.includes(reason as Reason)) {
      return t(`events.reasons.${reason as Reason}`)
    }
    const method = e.details.mfa_method as string | undefined
    if (method === 'recovery_code') {
      return t('auth.mfa.recoveryCode')
    }
    return reason ?? ''
  }
}
