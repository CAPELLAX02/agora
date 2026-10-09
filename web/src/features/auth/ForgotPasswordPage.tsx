import { zodResolver } from '@hookform/resolvers/zod'
import { AlertCircleIcon, ArrowLeftIcon, MailCheckIcon } from 'lucide-react'
import { useMemo } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'
import { z } from 'zod'

import { useForgotPasswordMutation } from '@/shared/api/generated'
import { useErrorMessage } from '@/shared/api/useErrorMessage'
import { Alert, AlertDescription } from '@/shared/ui/alert'
import { Button } from '@/shared/ui/button'
import { Field } from '@/shared/ui/field'
import { Input } from '@/shared/ui/input'

import { AuthLayout } from './AuthLayout'

export function ForgotPasswordPage() {
  const { t } = useTranslation()
  const message = useErrorMessage()
  const [request, { isLoading, isSuccess, error }] = useForgotPasswordMutation()
  const schema = useMemo(
    () => z.object({ identifier: z.string().trim().min(1, t('common.required')).max(254) }),
    [t],
  )
  const form = useForm({ resolver: zodResolver(schema), defaultValues: { identifier: '' } })

  const back = (
    <Button variant="link" asChild className="px-0">
      <Link to="/giris">
        <ArrowLeftIcon />
        {t('auth.forgot.backToLogin')}
      </Link>
    </Button>
  )

  return (
    <AuthLayout>
      {isSuccess ? (
        <div className="space-y-4">
          <span className="flex size-11 items-center justify-center rounded-xl bg-success/15 text-success">
            <MailCheckIcon className="size-5" aria-hidden />
          </span>
          <h1 className="text-2xl font-semibold tracking-tight">{t('auth.forgot.sentTitle')}</h1>
          <p className="text-sm text-muted-foreground">{t('auth.forgot.sentDescription')}</p>
          {back}
        </div>
      ) : (
        <div className="space-y-6">
          <div className="space-y-2">
            <h1 className="text-2xl font-semibold tracking-tight">{t('auth.forgot.title')}</h1>
            <p className="text-sm text-muted-foreground">{t('auth.forgot.description')}</p>
          </div>
          {error && (
            <Alert variant="destructive">
              <AlertCircleIcon />
              <AlertDescription>{message(error)}</AlertDescription>
            </Alert>
          )}
          <form
            className="space-y-4"
            noValidate
            onSubmit={form.handleSubmit(async ({ identifier }) => {
              await request({ body: { identifier } })
            })}
          >
            <Field label={t('auth.forgot.identifier')} error={form.formState.errors.identifier?.message}>
              {(p) => (
                <Input
                  {...p}
                  {...form.register('identifier')}
                  autoComplete="username"
                  autoCapitalize="none"
                  spellCheck={false}
                  autoFocus
                />
              )}
            </Field>
            <Button type="submit" className="w-full" loading={isLoading}>
              {t('auth.forgot.submit')}
            </Button>
          </form>
          {back}
        </div>
      )}
    </AuthLayout>
  )
}
