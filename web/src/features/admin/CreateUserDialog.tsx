import { zodResolver } from '@hookform/resolvers/zod'
import { useMemo, useState } from 'react'
import { Controller, useForm, useWatch } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router'
import { toast } from 'sonner'
import { z } from 'zod'

import { fieldErrors } from '@/shared/api/errors'
import { useCreateUserMutation, type CreateUserRequest } from '@/shared/api/generated'
import { useErrorMessage } from '@/shared/api/useErrorMessage'
import { Button } from '@/shared/ui/button'
import { Field } from '@/shared/ui/field'
import { FormDialog } from '@/shared/ui/form-dialog'
import { Input } from '@/shared/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/shared/ui/select'

import { OrgPicker } from '@/features/org/OrgPicker'
import { emptyOrg, type OrgSelection } from '@/features/org/orgSelection'

const titles = [
  'PROF',
  'ASSOC_PROF',
  'ASSIST_PROF',
  'LECTURER_DR',
  'LECTURER',
  'RESEARCH_ASSISTANT_DR',
  'RESEARCH_ASSISTANT',
] as const
const NO_TITLE = 'NONE'

/** CreateUserDialog, yeni bir öğrenci ya da personel hesabı açar. Hesap aktivasyon e-postasıyla başlar. */
export function CreateUserDialog({ onClose }: { onClose: () => void }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const message = useErrorMessage()
  const [create, { error, isLoading }] = useCreateUserMutation()
  const [org, setOrg] = useState<OrgSelection>(emptyOrg)

  const schema = useMemo(() => {
    const required = t('common.required')
    return z.object({
      kind: z.enum(['STUDENT', 'STAFF']),
      number: z.string().trim().min(1, required),
      firstName: z.string().trim().min(1, required).max(100),
      lastName: z.string().trim().min(1, required).max(100),
      email: z.email(t('common.invalidEmail')),
      staffType: z.enum(['ACADEMIC', 'ADMINISTRATIVE']),
      academicTitle: z.string(),
    })
  }, [t])

  const form = useForm({
    resolver: zodResolver(schema),
    defaultValues: {
      kind: 'STUDENT' as const,
      number: '',
      firstName: '',
      lastName: '',
      email: '',
      staffType: 'ACADEMIC' as const,
      academicTitle: NO_TITLE,
    },
  })
  const kind = useWatch({ control: form.control, name: 'kind' })
  const staffType = useWatch({ control: form.control, name: 'staffType' })

  const submit = form.handleSubmit(async (v) => {
    const body: CreateUserRequest = {
      kind: v.kind,
      number: v.number,
      first_name: v.firstName,
      last_name: v.lastName,
      email: v.email,
    }
    if (v.kind === 'STAFF') {
      body.staff_type = v.staffType
      if (v.staffType === 'ACADEMIC' && v.academicTitle !== NO_TITLE) {
        body.academic_title = v.academicTitle
      }
      if (org.departmentId) {
        body.department_id = org.departmentId
      }
    }
    const res = await create({ createUserRequest: body })
    if (res.data) {
      toast.success(t('users.create.success'))
      onClose()
      void navigate(`/yonetim/kullanicilar/${res.data.id}`)
      return
    }
    // Sunucunun alan hataları ilgili alanlara yazılır.
    const fields = fieldErrors(res.error)
    const map = { number: 'number', first_name: 'firstName', last_name: 'lastName', email: 'email' } as const
    for (const [server, local] of Object.entries(map)) {
      const e = fields[server]?.[0]
      if (e) {
        form.setError(local, { message: e.message })
      }
    }
  })

  const generalError =
    error && Object.keys(fieldErrors(error)).length === 0
      ? message(error, { ACCOUNT_EXISTS: t('users.errors.ACCOUNT_EXISTS') })
      : null
  const e = form.formState.errors

  return (
    <FormDialog
      title={t('users.create.title')}
      description={t('users.create.description')}
      error={generalError}
      onClose={onClose}
      onSubmit={() => void submit()}
      submit={
        <Button type="submit" loading={isLoading}>
          {t('users.create.submit')}
        </Button>
      }
    >
      <Field label={t('users.create.kind')}>
        {(p) => (
          <Controller
            control={form.control}
            name="kind"
            render={({ field }) => (
              <Select value={field.value} onValueChange={field.onChange}>
                <SelectTrigger {...p} className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="STUDENT">{t('users.kinds.STUDENT')}</SelectItem>
                  <SelectItem value="STAFF">{t('users.kinds.STAFF')}</SelectItem>
                </SelectContent>
              </Select>
            )}
          />
        )}
      </Field>
      <Field
        label={t(kind === 'STUDENT' ? 'users.create.studentNumber' : 'users.create.staffNumber')}
        error={e.number?.message}
      >
        {(p) => <Input {...p} {...form.register('number')} autoComplete="off" className="font-mono" />}
      </Field>
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label={t('users.create.firstName')} error={e.firstName?.message}>
          {(p) => <Input {...p} {...form.register('firstName')} autoComplete="off" />}
        </Field>
        <Field label={t('users.create.lastName')} error={e.lastName?.message}>
          {(p) => <Input {...p} {...form.register('lastName')} autoComplete="off" />}
        </Field>
      </div>
      <Field label={t('users.create.email')} error={e.email?.message}>
        {(p) => <Input {...p} {...form.register('email')} type="email" autoComplete="off" />}
      </Field>
      {kind === 'STAFF' && (
        <>
          <div className="grid gap-4 sm:grid-cols-2">
            <Field label={t('users.create.staffType')}>
              {(p) => (
                <Controller
                  control={form.control}
                  name="staffType"
                  render={({ field }) => (
                    <Select value={field.value} onValueChange={field.onChange}>
                      <SelectTrigger {...p} className="w-full">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="ACADEMIC">{t('users.create.staffTypes.ACADEMIC')}</SelectItem>
                        <SelectItem value="ADMINISTRATIVE">
                          {t('users.create.staffTypes.ADMINISTRATIVE')}
                        </SelectItem>
                      </SelectContent>
                    </Select>
                  )}
                />
              )}
            </Field>
            {staffType === 'ACADEMIC' && (
              <Field label={t('users.create.academicTitle')}>
                {(p) => (
                  <Controller
                    control={form.control}
                    name="academicTitle"
                    render={({ field }) => (
                      <Select value={field.value} onValueChange={field.onChange}>
                        <SelectTrigger {...p} className="w-full">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value={NO_TITLE}>{t('users.create.noTitle')}</SelectItem>
                          {titles.map((ti) => (
                            <SelectItem key={ti} value={ti}>
                              {t(`users.create.titles.${ti}`)}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    )}
                  />
                )}
              </Field>
            )}
          </div>
          <OrgPicker depth="department" value={org} onChange={setOrg} />
        </>
      )}
    </FormDialog>
  )
}
