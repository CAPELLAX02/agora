import { useTranslation } from 'react-i18next'

import { Button } from '@/shared/ui/button'
import { Field } from '@/shared/ui/field'
import { PasswordInput } from '@/shared/ui/password-input'

import { NewPasswordFields } from './passwordForm'
import type { ChangePasswordForm } from './useChangePassword'

export function ChangePasswordFields({
  form,
  onSubmit,
  isLoading,
  submitLabel,
}: {
  form: ChangePasswordForm
  onSubmit: () => void
  isLoading: boolean
  submitLabel: string
}) {
  const { t } = useTranslation()
  return (
    <form onSubmit={onSubmit} className="space-y-4" noValidate>
      <Field label={t('password.current')} error={form.formState.errors.currentPassword?.message}>
        {(p) => (
          <PasswordInput
            {...p}
            {...form.register('currentPassword')}
            autoComplete="current-password"
            showLabel={t('common.showPassword')}
            hideLabel={t('common.hidePassword')}
          />
        )}
      </Field>
      <NewPasswordFields
        newPassword={form.register('newPassword')}
        confirm={form.register('confirm')}
        errors={{
          newPassword: form.formState.errors.newPassword?.message,
          confirm: form.formState.errors.confirm?.message,
        }}
      />
      <Button type="submit" loading={isLoading}>
        {submitLabel}
      </Button>
    </form>
  )
}
