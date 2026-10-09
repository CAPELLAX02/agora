import { CheckIcon, CopyIcon, DownloadIcon, KeyRoundIcon, ShieldCheckIcon, ShieldOffIcon } from 'lucide-react'
import { QRCodeSVG } from 'qrcode.react'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { useAppDispatch } from '@/app/hooks'
import { formatDate } from '@/i18n/format'
import { refreshSession } from '@/shared/api/baseQuery'
import { errorCode, type AnyError } from '@/shared/api/errors'
import {
  agoraApi,
  useDisableMfaMutation,
  useEnableMfaMutation,
  useGetMeQuery,
  useGetMyMfaQuery,
  useRegenerateRecoveryCodesMutation,
  useStartMfaSetupMutation,
  type MfaSetup,
} from '@/shared/api/generated'
import { useErrorMessage } from '@/shared/api/useErrorMessage'
import { Alert, AlertDescription } from '@/shared/ui/alert'
import { Badge } from '@/shared/ui/badge'
import { Button } from '@/shared/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/shared/ui/card'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/shared/ui/dialog'
import { FormDialog } from '@/shared/ui/form-dialog'
import { Field } from '@/shared/ui/field'
import { Input } from '@/shared/ui/input'
import { OtpInput } from '@/shared/ui/otp-input'
import { PasswordInput } from '@/shared/ui/password-input'
import { Skeleton } from '@/shared/ui/skeleton'
import { ErrorState } from '@/shared/ui/states'

/**
 * useSessionUpgrade, MFA açılıp kapanınca oturumun access token'ını yeniler: sunucu
 * oturumun doğrulama yöntemlerini (AMR) değiştirdi, yeni token onu taşır. Ardından
 * hesap bilgileri ve yetkiler yeni token'la yeniden okunur.
 */
function useSessionUpgrade() {
  const dispatch = useAppDispatch()
  return async () => {
    await refreshSession(dispatch, { retry: true })
    // Mutasyonun kendi geçersiz kılmasıyla başlamış istekler eski token'la gidiyor:
    // onlar bitmeden geçersiz kılınırsa RTK Query devam edenleri yeniden çekmez.
    await Promise.all(dispatch(agoraApi.util.getRunningQueriesThunk()))
    dispatch(agoraApi.util.invalidateTags(['Hesap']))
  }
}

export function MfaSection() {
  const { t } = useTranslation()
  const { data, error, refetch } = useGetMyMfaQuery()
  const [dialog, setDialog] = useState<'setup' | 'disable' | 'regenerate' | null>(null)

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          {t('mfa.title')}
          {data && (
            <Badge variant={data.enabled ? 'success' : 'muted'}>
              {t(data.enabled ? 'mfa.on' : 'mfa.off')}
            </Badge>
          )}
        </CardTitle>
        <CardDescription className="max-w-2xl">{t('mfa.about')}</CardDescription>
      </CardHeader>
      <CardContent>
        {error ? (
          <ErrorState error={error} onRetry={() => void refetch()} />
        ) : !data ? (
          <Skeleton className="h-16" />
        ) : data.enabled ? (
          <div className="space-y-4">
            <div className="flex items-center gap-3 text-sm">
              <ShieldCheckIcon className="size-5 text-success" aria-hidden />
              <span>{t('mfa.enabledSince', { date: formatDate(data.enabled_at) })}</span>
            </div>
            <p className="flex items-center gap-2 text-sm text-muted-foreground">
              <KeyRoundIcon className="size-4" aria-hidden />
              {t('mfa.recoveryRemaining', { count: data.recovery_codes_remaining })}
            </p>
            {data.recovery_codes_remaining <= 3 && (
              <Alert variant="warning">
                <KeyRoundIcon />
                <AlertDescription>{t('mfa.recoveryLow')}</AlertDescription>
              </Alert>
            )}
            <div className="flex flex-wrap gap-2">
              <Button variant="outline" onClick={() => setDialog('regenerate')}>
                {t('mfa.regenerate')}
              </Button>
              <Button variant="ghost" className="text-destructive" onClick={() => setDialog('disable')}>
                <ShieldOffIcon />
                {t('mfa.disable')}
              </Button>
            </div>
          </div>
        ) : (
          <Button onClick={() => setDialog('setup')}>
            <ShieldCheckIcon />
            {t('mfa.start')}
          </Button>
        )}
      </CardContent>

      {dialog === 'setup' && <SetupDialog onClose={() => setDialog(null)} />}
      {dialog === 'disable' && <DisableDialog onClose={() => setDialog(null)} />}
      {dialog === 'regenerate' && <RegenerateDialog onClose={() => setDialog(null)} />}
    </Card>
  )
}

function useMfaErrors() {
  const { t } = useTranslation()
  const message = useErrorMessage()
  return (error: AnyError) =>
    message(error, {
      INVALID_MFA_CODE: t('mfa.errors.INVALID_MFA_CODE'),
      INVALID_CURRENT_PASSWORD: t('password.invalidCurrent'),
      MFA_ALREADY_ENABLED: t('mfa.errors.MFA_ALREADY_ENABLED'),
      MFA_NOT_ENABLED: t('mfa.errors.MFA_NOT_ENABLED'),
      MFA_SETUP_REQUIRED: t('mfa.errors.MFA_SETUP_REQUIRED'),
      ACCOUNT_LOCKED: ({ time }) => t('auth.errors.ACCOUNT_LOCKED', { time }),
    })
}

/** SetupDialog, kurulumu üç adımda yürütür: QR kodu okut, kodu doğrula, kurtarma kodlarını kaydet. */
function SetupDialog({ onClose }: { onClose: () => void }) {
  const { t } = useTranslation()
  const upgrade = useSessionUpgrade()
  const mfaError = useMfaErrors()
  const [start, startState] = useStartMfaSetupMutation()
  const [enable, enableState] = useEnableMfaMutation()
  const [setup, setSetup] = useState<MfaSetup | null>(null)
  const [codes, setCodes] = useState<string[] | null>(null)
  const [code, setCode] = useState('')
  const [password, setPassword] = useState('')
  const started = useRef(false)

  // Pencere açılınca bir kez yeni sır istenir. StrictMode efekti iki kez çalıştırır:
  // iki istek iki farklı sır üretir ve ekrandaki QR kod sunucudakiyle uyuşmayabilirdi.
  useEffect(() => {
    if (started.current) {
      return
    }
    started.current = true
    void start().then((res) => res.data && setSetup(res.data))
  }, [start])

  const submit = async () => {
    const res = await enable({ body: { current_password: password, code } })
    if (res.data) {
      setCodes(res.data.recovery_codes)
      toast.success(t('mfa.enabled'))
      await upgrade()
      return
    }
    setCode('')
    if (errorCode(res.error) === 'MFA_SETUP_REQUIRED') {
      setSetup(null)
      void start().then((r) => r.data && setSetup(r.data))
    }
  }

  if (codes) {
    return <RecoveryCodesDialog codes={codes} onClose={onClose} />
  }

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent closeLabel={t('common.close')}>
        <DialogHeader>
          <DialogTitle>{t('mfa.setup.title')}</DialogTitle>
          <DialogDescription>{t('mfa.setup.scan')}</DialogDescription>
        </DialogHeader>
        {startState.error ? (
          <ErrorState error={startState.error} />
        ) : !setup ? (
          <Skeleton className="mx-auto size-48" />
        ) : (
          <div className="space-y-5">
            <div className="flex flex-col items-center gap-3">
              {/* QR kod koyu temada da beyaz zeminde: bazı kamera uygulamaları ters renkleri okuyamaz. */}
              <div className="rounded-lg border bg-white p-3">
                <QRCodeSVG value={setup.otpauth_uri} size={176} level="M" title={t('mfa.setup.scan')} />
              </div>
              <p className="text-center text-xs text-muted-foreground">{t('mfa.setup.manual')}</p>
              <SecretKey secret={setup.secret} />
            </div>
            <form
              className="space-y-4"
              noValidate
              onSubmit={(e) => {
                e.preventDefault()
                void submit()
              }}
            >
              <p className="text-sm font-medium">{t('mfa.setup.verifyTitle')}</p>
              <p className="-mt-3 text-sm text-muted-foreground">{t('mfa.setup.verifyDescription')}</p>
              {enableState.error && (
                <Alert variant="destructive">
                  <AlertDescription>{mfaError(enableState.error)}</AlertDescription>
                </Alert>
              )}
              <Field label={t('auth.mfa.code')}>
                {(p) => <OtpInput {...p} value={code} onChange={(e) => setCode(e.target.value)} />}
              </Field>
              <Field label={t('password.current')}>
                {(p) => (
                  <PasswordInput
                    {...p}
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    autoComplete="current-password"
                    showLabel={t('common.showPassword')}
                    hideLabel={t('common.hidePassword')}
                  />
                )}
              </Field>
              <DialogFooter>
                <Button type="button" variant="outline" onClick={onClose}>
                  {t('common.cancel')}
                </Button>
                <Button
                  type="submit"
                  loading={enableState.isLoading}
                  disabled={code.length !== 6 || password === ''}
                >
                  {t('mfa.setup.enable')}
                </Button>
              </DialogFooter>
            </form>
          </div>
        )}
      </DialogContent>
    </Dialog>
  )
}

/** SecretKey, kurulum anahtarını okunur gruplar halinde gösterir ve kopyalatır. */
function SecretKey({ secret }: { secret: string }) {
  const { t } = useTranslation()
  const [copied, setCopied] = useState(false)
  const grouped = secret.match(/.{1,4}/g)?.join(' ') ?? secret
  return (
    <div className="flex w-full items-center gap-2 rounded-md bg-muted px-3 py-2">
      <code
        className="flex-1 text-center font-mono text-sm tracking-wider break-all"
        aria-label={t('mfa.setup.secretLabel')}
      >
        {grouped}
      </code>
      <Button
        type="button"
        variant="ghost"
        size="icon-sm"
        aria-label={copied ? t('common.copied') : t('common.copy')}
        onClick={() =>
          void navigator.clipboard.writeText(secret).then(() => {
            setCopied(true)
            setTimeout(() => setCopied(false), 2000)
          })
        }
      >
        {copied ? <CheckIcon /> : <CopyIcon />}
      </Button>
    </div>
  )
}

/** RecoveryCodesDialog, kurtarma kodlarını bir kez gösterir. Kullanıcı kaydettiğini onaylamadan kapanmaz. */
function RecoveryCodesDialog({ codes, onClose }: { codes: string[]; onClose: () => void }) {
  const { t } = useTranslation()
  const { data: me } = useGetMeQuery()
  const [saved, setSaved] = useState(false)
  const text = `${t('mfa.codes.fileHeader', { username: me?.username ?? '' })}\n\n${codes.join('\n')}\n`

  const download = () => {
    const url = URL.createObjectURL(new Blob([text], { type: 'text/plain;charset=utf-8' }))
    const a = document.createElement('a')
    a.href = url
    a.download = t('mfa.codes.fileName')
    a.click()
    URL.revokeObjectURL(url)
  }

  return (
    <Dialog open onOpenChange={(open) => !open && saved && onClose()}>
      <DialogContent
        onEscapeKeyDown={(e) => !saved && e.preventDefault()}
        onInteractOutside={(e) => e.preventDefault()}
      >
        <DialogHeader>
          <DialogTitle>{t('mfa.codes.title')}</DialogTitle>
          <DialogDescription>{t('mfa.codes.description')}</DialogDescription>
        </DialogHeader>
        <ul className="grid grid-cols-2 gap-x-6 gap-y-2 rounded-lg bg-muted p-4 font-mono text-sm">
          {codes.map((c) => (
            <li key={c} className="text-center">
              {c}
            </li>
          ))}
        </ul>
        <div className="flex flex-wrap gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={() => void navigator.clipboard.writeText(codes.join('\n'))}
          >
            <CopyIcon />
            {t('common.copy')}
          </Button>
          <Button variant="outline" size="sm" onClick={download}>
            <DownloadIcon />
            {t('common.download')}
          </Button>
        </div>
        <label className="flex items-center gap-2 text-sm">
          <input
            type="checkbox"
            className="size-4 accent-primary"
            checked={saved}
            onChange={(e) => setSaved(e.target.checked)}
          />
          {t('mfa.codes.confirm')}
        </label>
        <DialogFooter>
          <Button disabled={!saved} onClick={onClose}>
            {t('common.close')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function DisableDialog({ onClose }: { onClose: () => void }) {
  const { t } = useTranslation()
  const upgrade = useSessionUpgrade()
  const mfaError = useMfaErrors()
  const [disable, { error, isLoading }] = useDisableMfaMutation()
  const [password, setPassword] = useState('')
  const [code, setCode] = useState('')
  const [recovery, setRecovery] = useState(false)

  const submit = async () => {
    const res = await disable({
      body: recovery
        ? { current_password: password, recovery_code: code }
        : { current_password: password, code },
    })
    if (!res.error) {
      toast.success(t('mfa.disabled'))
      await upgrade()
      onClose()
    }
  }

  return (
    <FormDialog
      title={t('mfa.disableDialog.title')}
      description={t('mfa.disableDialog.description')}
      error={error ? mfaError(error) : null}
      onClose={onClose}
      onSubmit={() => void submit()}
      submit={
        <Button type="submit" variant="destructive" loading={isLoading} disabled={!password || !code}>
          {t('mfa.disable')}
        </Button>
      }
    >
      <Field label={t('password.current')}>
        {(p) => (
          <PasswordInput
            {...p}
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            autoComplete="current-password"
            showLabel={t('common.showPassword')}
            hideLabel={t('common.hidePassword')}
          />
        )}
      </Field>
      <Field label={t(recovery ? 'auth.mfa.recoveryCode' : 'auth.mfa.code')}>
        {(p) =>
          recovery ? (
            <Input
              {...p}
              value={code}
              onChange={(e) => setCode(e.target.value)}
              className="font-mono"
              placeholder={t('auth.mfa.recoveryPlaceholder')}
            />
          ) : (
            <OtpInput {...p} value={code} onChange={(e) => setCode(e.target.value)} />
          )
        }
      </Field>
      <Button
        type="button"
        variant="link"
        size="sm"
        className="px-0"
        onClick={() => {
          setRecovery((v) => !v)
          setCode('')
        }}
      >
        {t(recovery ? 'auth.mfa.useApp' : 'auth.mfa.useRecovery')}
      </Button>
    </FormDialog>
  )
}

function RegenerateDialog({ onClose }: { onClose: () => void }) {
  const { t } = useTranslation()
  const mfaError = useMfaErrors()
  const [regenerate, { error, isLoading }] = useRegenerateRecoveryCodesMutation()
  const [code, setCode] = useState('')
  const [codes, setCodes] = useState<string[] | null>(null)

  if (codes) {
    return <RecoveryCodesDialog codes={codes} onClose={onClose} />
  }
  return (
    <FormDialog
      title={t('mfa.regenerateDialog.title')}
      description={t('mfa.regenerateDialog.description')}
      error={error ? mfaError(error) : null}
      onClose={onClose}
      onSubmit={() =>
        void regenerate({ body: { code } }).then((res) => {
          if (res.data) {
            toast.success(t('mfa.regenerated'))
            setCodes(res.data.recovery_codes)
          } else {
            setCode('')
          }
        })
      }
      submit={
        <Button type="submit" loading={isLoading} disabled={code.length !== 6}>
          {t('mfa.regenerate')}
        </Button>
      }
    >
      <Field label={t('auth.mfa.code')}>
        {(p) => <OtpInput {...p} value={code} onChange={(e) => setCode(e.target.value)} autoFocus />}
      </Field>
    </FormDialog>
  )
}
