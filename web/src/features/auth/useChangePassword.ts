import { zodResolver } from '@hookform/resolvers/zod'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'

import { errorCode } from '@/shared/api/errors'
import { useChangePasswordMutation } from '@/shared/api/generated'
import { useErrorMessage } from '@/shared/api/useErrorMessage'

import { useNewPasswordSchema, usePasswordViolation } from './passwordPolicy'

export function useChangePasswordForm() {
  const s = useNewPasswordSchema()
  const { t } = useTranslation()
  return useForm({
    resolver: zodResolver(
      z
        .object({
          currentPassword: z.string().min(1, t('common.required')),
          newPassword: s.newPassword,
          confirm: s.confirm,
        })
        .refine((v) => v.newPassword === v.confirm, s.mismatch),
    ),
    defaultValues: { currentPassword: '', newPassword: '', confirm: '' },
  })
}

export type ChangePasswordForm = ReturnType<typeof useChangePasswordForm>

/** useChangePasswordSubmit, parola değiştirme isteğini gönderir ve hataları forma yazar. */
export function useChangePasswordSubmit(form: ChangePasswordForm, onSuccess: () => void) {
  const { t } = useTranslation()
  const message = useErrorMessage()
  const violation = usePasswordViolation()
  const [change, { isLoading, error }] = useChangePasswordMutation()

  const submit = form.handleSubmit(async ({ currentPassword, newPassword }) => {
    const res = await change({ body: { current_password: currentPassword, new_password: newPassword } })
    if (!res.error) {
      form.reset()
      onSuccess()
      return
    }
    if (errorCode(res.error) === 'INVALID_CURRENT_PASSWORD') {
      form.setError('currentPassword', { message: t('password.invalidCurrent') })
      return
    }
    const v = violation(res.error)
    if (v) {
      form.setError('newPassword', { message: v })
    }
  })

  const shown =
    error && errorCode(error) !== 'INVALID_CURRENT_PASSWORD' && !violation(error)
      ? message(error, { ACCOUNT_LOCKED: ({ time }) => t('auth.errors.ACCOUNT_LOCKED', { time }) })
      : null
  return { submit, isLoading, generalError: shown }
}
