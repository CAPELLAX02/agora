import { zodResolver } from '@hookform/resolvers/zod'
import { AlertCircleIcon, InfoIcon, KeyRoundIcon, ShieldCheckIcon, SmartphoneIcon } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { Link, useLocation } from 'react-router'
import { z } from 'zod'

import { useAppDispatch, useAppSelector } from '@/app/hooks'
import { baseApi } from '@/shared/api/baseApi'
import { errorCode } from '@/shared/api/errors'
import {
  useLoginMutation,
  useVerifyMfaMutation,
  type MfaChallenge,
  type Tokens,
} from '@/shared/api/generated'
import { useErrorMessage, type Messages } from '@/shared/api/useErrorMessage'
import { Alert, AlertDescription } from '@/shared/ui/alert'
import { Button } from '@/shared/ui/button'
import { Field } from '@/shared/ui/field'
import { Input } from '@/shared/ui/input'
import { OtpInput } from '@/shared/ui/otp-input'
import { PasswordInput } from '@/shared/ui/password-input'

import { AuthLayout } from './AuthLayout'
import { signedIn } from './authSlice'

/** LoginNotice, başka bir ekrandan giriş ekranına yönlendirirken gösterilecek mesajdır. */
export type LoginNotice = 'passwordSet' | 'accountActivated'

/** LoginPage, parolayla girişi ve gerekiyorsa ikinci adımı (TOTP ya da kurtarma kodu) yürütür. */
export function LoginPage() {
  const dispatch = useAppDispatch()
  const [challenge, setChallenge] = useState<MfaChallenge | null>(null)
  const [restartReason, setRestartReason] = useState<string | null>(null)

  // Oturum açıldığında GuestOnly kullanıcıyı geldiği sayfaya yönlendirir. Önceki
  // kullanıcıdan kalmış önbellek temizlenir.
  const finish = (tokens: Tokens) => {
    dispatch(baseApi.util.resetApiState())
    dispatch(signedIn(tokens))
  }

  return (
    <AuthLayout>
      {challenge ? (
        <MfaStep
          challenge={challenge}
          onDone={finish}
          onRestart={(reason) => {
            setChallenge(null)
            setRestartReason(reason)
          }}
        />
      ) : (
        <PasswordStep
          restartReason={restartReason}
          onDone={finish}
          onChallenge={(c) => {
            setRestartReason(null)
            setChallenge(c)
          }}
        />
      )}
    </AuthLayout>
  )
}

function PasswordStep({
  restartReason,
  onDone,
  onChallenge,
}: {
  restartReason: string | null
  onDone: (tokens: Tokens) => void
  onChallenge: (c: MfaChallenge) => void
}) {
  const { t } = useTranslation()
  const location = useLocation()
  const reason = useAppSelector((s) => s.auth.reason)
  const notice = (location.state as { notice?: LoginNotice } | null)?.notice
  const message = useErrorMessage()
  const [login, { error, isLoading }] = useLoginMutation()

  const schema = useMemo(
    () =>
      z.object({
        username: z.string().trim().min(1, t('auth.login.usernameRequired')),
        password: z.string().min(1, t('auth.login.passwordRequired')),
      }),
    [t],
  )
  const form = useForm({ resolver: zodResolver(schema), defaultValues: { username: '', password: '' } })

  const onSubmit = form.handleSubmit(async (values) => {
    const res = await login({ 'X-Agora-Client': 'web', body: values })
    if (!res.data) {
      form.setValue('password', '')
      form.setFocus('password')
      return
    }
    if ('mfa_required' in res.data) {
      onChallenge(res.data)
    } else {
      onDone(res.data)
    }
  })

  const loginMessages: Messages = {
    INVALID_CREDENTIALS: t('auth.errors.INVALID_CREDENTIALS'),
    ACCOUNT_LOCKED: ({ time }) => t('auth.errors.ACCOUNT_LOCKED', { time }),
    ACCOUNT_DISABLED: t('auth.errors.ACCOUNT_DISABLED'),
    RATE_LIMITED: ({ time }) => t('auth.errors.RATE_LIMITED', { time }),
  }

  const info = restartReason
    ? { variant: 'warning' as const, text: restartReason }
    : notice
      ? { variant: 'success' as const, text: t(`auth.messages.${notice}`) }
      : reason === 'expired'
        ? { variant: 'info' as const, text: t('auth.messages.expired') }
        : reason === 'loggedOut'
          ? { variant: 'success' as const, text: t('auth.messages.loggedOut') }
          : reason === 'disabled'
            ? { variant: 'destructive' as const, text: t('auth.errors.ACCOUNT_DISABLED') }
            : null

  return (
    <div className="space-y-6">
      <div className="space-y-2">
        <h1 className="text-2xl font-semibold tracking-tight">{t('auth.login.title')}</h1>
        <p className="text-sm text-muted-foreground">{t('auth.login.description')}</p>
      </div>

      {error ? (
        <Alert variant="destructive">
          <AlertCircleIcon />
          <AlertDescription>{message(error, loginMessages)}</AlertDescription>
        </Alert>
      ) : (
        info && (
          <Alert variant={info.variant}>
            <InfoIcon />
            <AlertDescription>{info.text}</AlertDescription>
          </Alert>
        )
      )}

      <form onSubmit={onSubmit} className="space-y-4" noValidate>
        <Field label={t('auth.login.username')} error={form.formState.errors.username?.message}>
          {(p) => (
            <Input
              {...p}
              {...form.register('username')}
              autoComplete="username"
              autoCapitalize="none"
              spellCheck={false}
              placeholder={t('auth.login.usernamePlaceholder')}
              autoFocus
            />
          )}
        </Field>
        <Field label={t('auth.login.password')} error={form.formState.errors.password?.message}>
          {(p) => (
            <PasswordInput
              {...p}
              {...form.register('password')}
              autoComplete="current-password"
              showLabel={t('common.showPassword')}
              hideLabel={t('common.hidePassword')}
            />
          )}
        </Field>
        <div className="-mt-2 flex justify-end">
          <Link to="/sifremi-unuttum" className="text-sm text-primary hover:underline">
            {t('auth.login.forgot')}
          </Link>
        </div>
        <Button type="submit" className="w-full" loading={isLoading}>
          {t('auth.login.submit')}
        </Button>
      </form>
    </div>
  )
}

function MfaStep({
  challenge,
  onDone,
  onRestart,
}: {
  challenge: MfaChallenge
  onDone: (tokens: Tokens) => void
  onRestart: (reason: string | null) => void
}) {
  const { t } = useTranslation()
  const message = useErrorMessage()
  const [verify, { error, isLoading, reset }] = useVerifyMfaMutation()
  const [useRecovery, setUseRecovery] = useState(false)
  const [value, setValue] = useState('')
  const [localError, setLocalError] = useState<string | null>(null)

  const submit = async (input: string) => {
    if (useRecovery ? input.trim() === '' : input.length !== 6) {
      setLocalError(t(useRecovery ? 'auth.mfa.recoveryRequired' : 'auth.mfa.codeRequired'))
      return
    }
    setLocalError(null)
    const res = await verify({
      'X-Agora-Client': 'web',
      body: useRecovery
        ? { mfa_token: challenge.mfa_token, recovery_code: input.trim() }
        : { mfa_token: challenge.mfa_token, code: input },
    })
    if (res.data) {
      onDone(res.data)
      return
    }
    const code = errorCode(res.error)
    // Token geçersiz, süresi dolmuş ya da deneme hakkı bitmiş: parola yeniden girilmeli.
    if (code === 'INVALID_MFA_TOKEN' || code === 'ACCOUNT_LOCKED' || code === 'ACCOUNT_DISABLED') {
      onRestart(
        message(res.error, {
          INVALID_MFA_TOKEN: t('auth.errors.INVALID_MFA_TOKEN'),
          ACCOUNT_LOCKED: ({ time }) => t('auth.errors.ACCOUNT_LOCKED', { time }),
          ACCOUNT_DISABLED: t('auth.errors.ACCOUNT_DISABLED'),
        }),
      )
      return
    }
    setValue('')
  }

  const shownError =
    localError ??
    (error
      ? message(error, {
          INVALID_MFA_CODE: t('auth.errors.INVALID_MFA_CODE'),
          RATE_LIMITED: ({ time }) => t('auth.errors.RATE_LIMITED', { time }),
        })
      : null)

  return (
    <div className="space-y-6">
      <div className="space-y-2">
        <span className="flex size-11 items-center justify-center rounded-xl bg-primary/10 text-primary">
          <ShieldCheckIcon className="size-5" aria-hidden />
        </span>
        <h1 className="text-2xl font-semibold tracking-tight">{t('auth.mfa.title')}</h1>
        <p className="text-sm text-muted-foreground">
          {t(useRecovery ? 'auth.mfa.recoveryDescription' : 'auth.mfa.description')}
        </p>
      </div>

      {shownError && (
        <Alert variant="destructive">
          <AlertCircleIcon />
          <AlertDescription>{shownError}</AlertDescription>
        </Alert>
      )}

      <form
        className="space-y-4"
        noValidate
        onSubmit={(e) => {
          e.preventDefault()
          void submit(value)
        }}
      >
        <Field label={t(useRecovery ? 'auth.mfa.recoveryCode' : 'auth.mfa.code')}>
          {(p) =>
            useRecovery ? (
              <Input
                {...p}
                key="recovery"
                value={value}
                onChange={(e) => setValue(e.target.value)}
                autoComplete="off"
                autoCapitalize="none"
                spellCheck={false}
                placeholder={t('auth.mfa.recoveryPlaceholder')}
                className="font-mono"
                autoFocus
              />
            ) : (
              <OtpInput
                {...p}
                key="otp"
                value={value}
                onChange={(e) => {
                  setValue(e.target.value)
                  // Altıncı hane girilince kod kendiliğinden gönderilir.
                  if (e.target.value.length === 6 && !isLoading) {
                    void submit(e.target.value)
                  }
                }}
                autoFocus
              />
            )
          }
        </Field>
        <Button type="submit" className="w-full" loading={isLoading}>
          {t('auth.mfa.submit')}
        </Button>
      </form>

      <div className="flex flex-col items-center gap-2 text-sm">
        <Button
          variant="link"
          size="sm"
          onClick={() => {
            setUseRecovery((v) => !v)
            setValue('')
            setLocalError(null)
            reset()
          }}
        >
          {useRecovery ? <SmartphoneIcon /> : <KeyRoundIcon />}
          {t(useRecovery ? 'auth.mfa.useApp' : 'auth.mfa.useRecovery')}
        </Button>
        <Button variant="link" size="sm" className="text-muted-foreground" onClick={() => onRestart(null)}>
          {t('auth.mfa.otherAccount')}
        </Button>
      </div>
    </div>
  )
}
