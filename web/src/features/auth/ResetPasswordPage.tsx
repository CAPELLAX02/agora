import { zodResolver } from '@hookform/resolvers/zod'
import { AlertCircleIcon, LinkIcon } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate } from 'react-router'
import { z } from 'zod'

import { formatDateTime } from '@/i18n/format'
import { errorCode } from '@/shared/api/errors'
import { useResetPasswordMutation, useVerifyResetTokenMutation } from '@/shared/api/generated'
import { useErrorMessage } from '@/shared/api/useErrorMessage'
import { Alert, AlertDescription } from '@/shared/ui/alert'
import { Button } from '@/shared/ui/button'
import { Spinner } from '@/shared/ui/spinner'

import { AuthLayout } from './AuthLayout'
import type { LoginNotice } from './LoginPage'
import { NewPasswordFields } from './passwordForm'
import { useNewPasswordSchema, usePasswordViolation } from './passwordPolicy'

/**
 * readToken, bağlantıdaki token'ı URL parçasından (#token=...) okur ve adres çubuğundan
 * siler: parça sunucuya hiç gönderilmez, silinince tarayıcı geçmişinde de kalmaz.
 */
function readToken(): string {
  const token = new URLSearchParams(window.location.hash.slice(1)).get('token') ?? ''
  if (window.location.hash) {
    window.history.replaceState(window.history.state, '', window.location.pathname + window.location.search)
  }
  return token
}

/** ResetPasswordPage, parola sıfırlama ve hesap aktivasyonu bağlantılarının sayfasıdır. */
export function ResetPasswordPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [token] = useState(readToken)
  const [verify, verifyState] = useVerifyResetTokenMutation()
  const [reset, resetState] = useResetPasswordMutation()
  const message = useErrorMessage()
  const violation = usePasswordViolation()

  const s = useNewPasswordSchema()
  const form = useForm({
    resolver: zodResolver(
      z
        .object({ newPassword: s.newPassword, confirm: s.confirm })
        .refine((v) => v.newPassword === v.confirm, s.mismatch),
    ),
    defaultValues: { newPassword: '', confirm: '' },
  })

  useEffect(() => {
    if (token) {
      void verify({ resetTokenBody: { token } })
    }
  }, [token, verify])

  const invalid =
    !token ||
    errorCode(verifyState.error) === 'INVALID_RESET_TOKEN' ||
    errorCode(resetState.error) === 'INVALID_RESET_TOKEN'

  if (invalid) {
    return (
      <AuthLayout>
        <div className="space-y-4">
          <span className="flex size-11 items-center justify-center rounded-xl bg-destructive/10 text-destructive">
            <LinkIcon className="size-5" aria-hidden />
          </span>
          <h1 className="text-2xl font-semibold tracking-tight">{t('auth.reset.invalidTitle')}</h1>
          <p className="text-sm text-muted-foreground">{t('auth.reset.invalidDescription')}</p>
          <Button asChild>
            <Link to="/sifremi-unuttum">{t('auth.reset.requestNew')}</Link>
          </Button>
        </div>
      </AuthLayout>
    )
  }

  const info = verifyState.data
  if (!info) {
    return (
      <AuthLayout>
        {verifyState.error ? (
          <Alert variant="destructive">
            <AlertCircleIcon />
            <AlertDescription>{message(verifyState.error)}</AlertDescription>
          </Alert>
        ) : (
          <p className="flex items-center gap-2 text-sm text-muted-foreground">
            <Spinner /> {t('auth.reset.verifying')}
          </p>
        )}
      </AuthLayout>
    )
  }

  const activation = info.purpose === 'ACTIVATION'
  const onSubmit = form.handleSubmit(async ({ newPassword }) => {
    const res = await reset({ body: { token, new_password: newPassword } })
    if (!res.error) {
      const notice: LoginNotice = activation ? 'accountActivated' : 'passwordSet'
      void navigate('/giris', { replace: true, state: { notice } })
      return
    }
    const v = violation(res.error)
    if (v) {
      form.setError('newPassword', { message: v })
    }
  })
  const generalError = resetState.error && !violation(resetState.error) ? message(resetState.error) : null

  return (
    <AuthLayout>
      <div className="space-y-6">
        <div className="space-y-2">
          <h1 className="text-2xl font-semibold tracking-tight">
            {t(activation ? 'auth.reset.activationTitle' : 'auth.reset.resetTitle')}
          </h1>
          <p className="text-sm text-muted-foreground">
            {t(activation ? 'auth.reset.activationDescription' : 'auth.reset.resetDescription')}
          </p>
          <p className="text-xs text-muted-foreground">
            {t('auth.reset.expiresAt', { time: formatDateTime(info.expires_at) })}
          </p>
        </div>
        {generalError && (
          <Alert variant="destructive">
            <AlertCircleIcon />
            <AlertDescription>{generalError}</AlertDescription>
          </Alert>
        )}
        <form onSubmit={onSubmit} className="space-y-4" noValidate>
          <NewPasswordFields
            newPassword={form.register('newPassword')}
            confirm={form.register('confirm')}
            errors={{
              newPassword: form.formState.errors.newPassword?.message,
              confirm: form.formState.errors.confirm?.message,
            }}
          />
          <Button type="submit" className="w-full" loading={resetState.isLoading}>
            {t('auth.reset.submit')}
          </Button>
        </form>
      </div>
    </AuthLayout>
  )
}
