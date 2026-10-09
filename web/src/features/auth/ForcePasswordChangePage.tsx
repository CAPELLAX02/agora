import { AlertCircleIcon, KeyRoundIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Navigate } from 'react-router'

import { useAppDispatch, useAppSelector } from '@/app/hooks'
import { Alert, AlertDescription } from '@/shared/ui/alert'
import { Button } from '@/shared/ui/button'

import { AuthLayout } from './AuthLayout'
import { passwordChanged } from './authSlice'
import { ChangePasswordFields } from './ChangePasswordFields'
import { useChangePasswordForm, useChangePasswordSubmit } from './useChangePassword'
import { useLogout } from './useLogout'

/** ForcePasswordChangePage, geçici parolayla giren kullanıcının parolasını değiştirdiği ekrandır. */
export function ForcePasswordChangePage() {
  const { t } = useTranslation()
  const dispatch = useAppDispatch()
  const mustChange = useAppSelector((s) => s.auth.mustChangePassword)
  const logout = useLogout()
  const form = useChangePasswordForm()
  const { submit, isLoading, generalError } = useChangePasswordSubmit(form, () => dispatch(passwordChanged()))

  if (!mustChange) {
    return <Navigate to="/" replace />
  }

  return (
    <AuthLayout>
      <div className="space-y-6">
        <div className="space-y-2">
          <span className="flex size-11 items-center justify-center rounded-xl bg-warning/20 text-warning-foreground dark:text-warning">
            <KeyRoundIcon className="size-5" aria-hidden />
          </span>
          <h1 className="text-2xl font-semibold tracking-tight">{t('password.force.title')}</h1>
          <p className="text-sm text-muted-foreground">{t('password.force.description')}</p>
        </div>
        {generalError && (
          <Alert variant="destructive">
            <AlertCircleIcon />
            <AlertDescription>{generalError}</AlertDescription>
          </Alert>
        )}
        <ChangePasswordFields
          form={form}
          onSubmit={submit}
          isLoading={isLoading}
          submitLabel={t('password.force.submit')}
        />
        <Button variant="link" size="sm" className="px-0 text-muted-foreground" onClick={logout}>
          {t('nav.logout')}
        </Button>
      </div>
    </AuthLayout>
  )
}
