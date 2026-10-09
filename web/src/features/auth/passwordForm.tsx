import type { UseFormRegisterReturn } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Field } from '@/shared/ui/field'
import { PasswordInput } from '@/shared/ui/password-input'

/** NewPasswordFields, yeni parola ve tekrarı alanlarıdır. */
export function NewPasswordFields({
  newPassword,
  confirm,
  errors,
}: {
  newPassword: UseFormRegisterReturn
  confirm: UseFormRegisterReturn
  errors: { newPassword?: string | undefined; confirm?: string | undefined }
}) {
  const { t } = useTranslation()
  return (
    <>
      <Field label={t('password.new')} error={errors.newPassword} hint={t('password.hint')}>
        {(p) => (
          <PasswordInput
            {...p}
            {...newPassword}
            autoComplete="new-password"
            showLabel={t('common.showPassword')}
            hideLabel={t('common.hidePassword')}
          />
        )}
      </Field>
      <Field label={t('password.confirm')} error={errors.confirm}>
        {(p) => (
          <PasswordInput
            {...p}
            {...confirm}
            autoComplete="new-password"
            showLabel={t('common.showPassword')}
            hideLabel={t('common.hidePassword')}
          />
        )}
      </Field>
    </>
  )
}
