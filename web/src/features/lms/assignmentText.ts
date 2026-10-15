import { useTranslation } from 'react-i18next'

import { formatNumber } from '@/i18n/format'
import type { AssignmentSettings } from '@/shared/api/generated'
import { formatBytes } from '@/shared/api/upload'

/** locale, sayı ve boyut biçimi için dilin yerel ayarıdır. */
export function useLocale() {
  const { i18n } = useTranslation()
  return i18n.language.startsWith('en') ? 'en-GB' : 'tr-TR'
}

/** acceptsText, ödevin kabul ettiği teslimin açıklamasıdır (türler, dosya sınırları). */
export function useAcceptsText() {
  const { t } = useTranslation()
  const locale = useLocale()
  return (a: AssignmentSettings) => {
    const parts: string[] = []
    if (a.submission_types.includes('FILE')) {
      parts.push(
        t('lms.assignment.acceptsFiles', {
          count: a.max_files,
          size: formatBytes(a.max_file_size_bytes, locale),
        }) + (a.allowed_extensions.length > 0 ? ` (${a.allowed_extensions.join(', ')})` : ''),
      )
    }
    if (a.submission_types.includes('ONLINE_TEXT')) {
      parts.push(t('lms.assignment.types.ONLINE_TEXT'))
    }
    return parts.join(' · ')
  }
}

/** policyText, geç teslim politikasının açıklamasıdır. */
export function usePolicyText() {
  const { t } = useTranslation()
  return (a: AssignmentSettings) =>
    t(`lms.assignment.policyHint.${a.late_policy}`, { pct: formatNumber(a.late_penalty_pct_per_day) })
}
