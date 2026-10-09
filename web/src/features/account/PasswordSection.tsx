import { AlertCircleIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ChangePasswordFields } from '@/features/auth/ChangePasswordFields'
import { useChangePasswordForm, useChangePasswordSubmit } from '@/features/auth/useChangePassword'
import { Alert, AlertDescription } from '@/shared/ui/alert'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/shared/ui/card'

export function PasswordSection() {
  const { t } = useTranslation()
  const form = useChangePasswordForm()
  const { submit, isLoading, generalError } = useChangePasswordSubmit(form, () =>
    toast.success(t('password.change.success')),
  )
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('password.change.title')}</CardTitle>
        <CardDescription>{t('password.change.description')}</CardDescription>
      </CardHeader>
      <CardContent className="max-w-md space-y-4">
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
          submitLabel={t('password.change.submit')}
        />
      </CardContent>
    </Card>
  )
}
