import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'

import { fieldErrors, type AnyError } from '@/shared/api/errors'

const violations = [
  'TOO_SHORT',
  'TOO_LONG',
  'TOO_COMMON',
  'CONTAINS_PERSONAL_INFO',
  'SAME_AS_CURRENT',
] as const
type Violation = (typeof violations)[number]

const isViolation = (code: string | undefined): code is Violation => violations.includes(code as Violation)

/**
 * useNewPasswordSchema, yeni parola ve tekrarının şemasıdır. İstemcideki kontrol sadece
 * uzunluk ve eşleşmedir: yaygın parola ve kişisel bilgi kontrolü sunucudadır.
 */
export function useNewPasswordSchema() {
  const { t } = useTranslation()
  return useMemo(
    () => ({
      newPassword: z
        .string()
        .min(10, t('password.violations.TOO_SHORT'))
        .max(128, t('password.violations.TOO_LONG')),
      confirm: z.string(),
      mismatch: { path: ['confirm'], message: t('password.mismatch') },
    }),
    [t],
  )
}

/**
 * usePasswordViolation, sunucunun parola politikası ihlallerini (VALIDATION_FAILED,
 * errors[].code) kullanıcının dilinde tek bir metne çevirir. İhlal yoksa undefined döner.
 */
export function usePasswordViolation() {
  const { t } = useTranslation()
  return (error: AnyError): string | undefined => {
    const errors = fieldErrors(error).new_password
    if (!errors?.length) {
      return undefined
    }
    return errors.map((e) => (isViolation(e.code) ? t(`password.violations.${e.code}`) : e.message)).join(' ')
  }
}
