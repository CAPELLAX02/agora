import { useCallback } from 'react'
import { useTranslation } from 'react-i18next'

import { formatDuration } from '@/i18n/format'

import { errorCode, httpStatus, isNetworkError, problemOf, retryAfter, type AnyError } from './errors'

/** Messages, sayfaya özgü hata kodu → metin eşlemesidir. time, Retry-After süresidir. */
export type Messages = Partial<Record<string, string | ((ctx: { time: string }) => string)>>

/**
 * useErrorMessage, bir API hatasını kullanıcıya gösterilecek metne çevirir. Önce sayfaya
 * özgü eşlemeye, sonra ortak kodlara bakılır. Bilinmeyen kodlarda sunucunun mesajı
 * (detail), o da yoksa genel bir mesaj gösterilir.
 */
export function useErrorMessage() {
  const { t } = useTranslation()
  return useCallback(
    (error: AnyError, messages: Messages = {}): string => {
      if (!error) {
        return ''
      }
      if (isNetworkError(error)) {
        return t('errors.network')
      }
      const code = errorCode(error)
      const wait = retryAfter(error)
      const time = wait ? formatDuration(wait) : ''
      const own = code ? messages[code] : undefined
      if (own) {
        return typeof own === 'function' ? own({ time }) : own
      }
      switch (code) {
        case 'MFA_REQUIRED':
          return t('errors.mfaRequired')
        case 'FORBIDDEN':
          return t('errors.forbidden')
        case 'NOT_FOUND':
          return t('errors.notFound')
        case 'RATE_LIMITED':
          return t('errors.rateLimited')
        case 'SERVICE_UNAVAILABLE':
          return t('errors.unavailable')
      }
      const status = httpStatus(error)
      if (status !== undefined && status >= 500) {
        return t('errors.generic')
      }
      return problemOf(error)?.detail ?? t('errors.generic')
    },
    [t],
  )
}
