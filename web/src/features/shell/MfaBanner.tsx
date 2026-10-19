import { ShieldAlertIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Link, useLocation } from 'react-router'

import { useLogout } from '@/features/auth/useLogout'
import { useGetMeQuery } from '@/shared/api/generated'
import { Button } from '@/shared/ui/button'
import { IconChip } from '@/shared/ui/icon-chip'

/**
 * MfaBanner, kullanıcının bazı yetkileri iki adımlı doğrulama olmadığı için
 * kullanılamıyorsa gösterilir. MFA kapalıysa kurulum, açıksa (oturum MFA'dan önce
 * açılmış) yeniden giriş önerilir.
 */
export function MfaBanner() {
  const { t } = useTranslation()
  const { data: me } = useGetMeQuery()
  const location = useLocation()
  const logout = useLogout()
  if (!me?.mfa_required) {
    return null
  }
  return (
    <div className="px-4 pt-4 md:px-6 lg:px-8">
      <div
        role="alert"
        className="mx-auto flex max-w-6xl flex-col gap-3 rounded-xl border border-amber-400/40 bg-gradient-to-r from-amber-50 to-orange-50 px-4 py-3 sm:flex-row sm:items-center dark:from-amber-500/10 dark:to-orange-500/5"
      >
        <IconChip icon={ShieldAlertIcon} tone="amber" />
        <div className="min-w-0 flex-1 text-sm">
          <p className="font-bold">{t('shell.mfaBanner.title')}</p>
          <p className="text-muted-foreground">{t('shell.mfaBanner.description')}</p>
        </div>
        {me.mfa_enabled ? (
          <Button size="sm" variant="outline" onClick={logout}>
            {t('shell.mfaBanner.relogin')}
          </Button>
        ) : (
          location.pathname !== '/guvenlik' && (
            <Button size="sm" asChild>
              <Link to="/guvenlik?tab=mfa">{t('shell.mfaBanner.enable')}</Link>
            </Button>
          )
        )}
      </div>
    </div>
  )
}
