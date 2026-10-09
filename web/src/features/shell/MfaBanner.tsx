import { ShieldAlertIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Link, useLocation } from 'react-router'

import { useLogout } from '@/features/auth/useLogout'
import { useGetMeQuery } from '@/shared/api/generated'
import { Alert, AlertDescription, AlertTitle } from '@/shared/ui/alert'
import { Button } from '@/shared/ui/button'

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
      <Alert variant="warning" className="mx-auto max-w-6xl">
        <ShieldAlertIcon />
        <AlertTitle>{t('shell.mfaBanner.title')}</AlertTitle>
        <AlertDescription>
          <p>{t('shell.mfaBanner.description')}</p>
          {me.mfa_enabled ? (
            <Button size="sm" variant="outline" className="mt-2" onClick={logout}>
              {t('shell.mfaBanner.relogin')}
            </Button>
          ) : (
            location.pathname !== '/guvenlik' && (
              <Button size="sm" variant="outline" className="mt-2" asChild>
                <Link to="/guvenlik?tab=mfa">{t('shell.mfaBanner.enable')}</Link>
              </Button>
            )
          )}
        </AlertDescription>
      </Alert>
    </div>
  )
}
