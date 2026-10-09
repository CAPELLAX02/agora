import { zodResolver } from '@hookform/resolvers/zod'
import { useMemo } from 'react'
import { Controller, useForm, useWatch } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

import { ifMatch } from '@/shared/api/etag'
import { fieldErrors } from '@/shared/api/errors'
import {
  useCreateCalendarEventMutation,
  useListCalendarEventTypesQuery,
  useListFacultiesQuery,
  useUpdateCalendarEventMutation,
  type CalendarEvent,
  type CalendarEventRequest,
} from '@/shared/api/generated'
import { useErrorMessage } from '@/shared/api/useErrorMessage'
import { useLocalized } from '@/shared/lib/localized'
import { Button } from '@/shared/ui/button'
import { Checkbox } from '@/shared/ui/checkbox'
import { Field } from '@/shared/ui/field'
import { FormDialog } from '@/shared/ui/form-dialog'
import { Input } from '@/shared/ui/input'
import { Label } from '@/shared/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/shared/ui/select'
import { Textarea } from '@/shared/ui/textarea'

import { toLocalInput } from './term'

/**
 * EventDialog, bir takvim olayı ekler ya da düzenler. Olay türü sonradan değişmez.
 * Kapsam üniversite ya da bir birimdir; program kapsamlı olaylar kapsamlarıyla korunur.
 */
export function EventDialog({
  termId,
  event,
  onClose,
}: {
  termId: string
  event: CalendarEvent | null
  onClose: () => void
}) {
  const { t } = useTranslation()
  const message = useErrorMessage()
  const localized = useLocalized()
  const types = useListCalendarEventTypesQuery()
  const faculties = useListFacultiesQuery({})
  const [create, created] = useCreateCalendarEventMutation()
  const [update, updated] = useUpdateCalendarEventMutation()
  const error = created.error ?? updated.error

  const schema = useMemo(() => {
    const required = t('common.required')
    return z
      .object({
        type: z.string().min(1, required),
        titleTr: z.string().trim().max(200),
        titleEn: z.string().trim().max(200),
        startsAt: z.string().min(1, required),
        endsAt: z.string().min(1, required),
        scopeType: z.enum(['UNIVERSITY', 'FACULTY', 'PROGRAM']),
        scopeId: z.string(),
        published: z.boolean(),
        note: z.string().trim().max(1000),
      })
      .refine((v) => !v.startsAt || !v.endsAt || v.endsAt > v.startsAt, {
        path: ['endsAt'],
        message: t('calendar.form.endAfterStart'),
      })
      .refine((v) => v.scopeType === 'UNIVERSITY' || v.scopeId !== '', {
        path: ['scopeId'],
        message: required,
      })
  }, [t])

  const form = useForm({
    resolver: zodResolver(schema),
    defaultValues: {
      type: event?.type.code ?? '',
      titleTr: event?.title_tr ?? '',
      titleEn: event?.title_en ?? '',
      startsAt: event ? toLocalInput(event.starts_at) : '',
      endsAt: event ? toLocalInput(event.ends_at) : '',
      scopeType: event?.scope_type ?? 'UNIVERSITY',
      scopeId: event?.scope?.id ?? '',
      published: event?.is_published ?? true,
      note: event?.note ?? '',
    },
  })
  const scopeType = useWatch({ control: form.control, name: 'scopeType' })

  const submit = form.handleSubmit(async (v) => {
    const body: CalendarEventRequest = {
      starts_at: new Date(v.startsAt).toISOString(),
      ends_at: new Date(v.endsAt).toISOString(),
      scope_type: v.scopeType,
      is_published: v.published,
      ...(v.titleTr && { title_tr: v.titleTr }),
      ...(v.titleEn && { title_en: v.titleEn }),
      ...(v.note && { note: v.note }),
      ...(v.scopeType !== 'UNIVERSITY' && { scope_id: v.scopeId }),
    }
    const res = event
      ? await update({ id: event.id, 'If-Match': ifMatch(event.version), calendarEventRequest: body })
      : await create({ id: termId, calendarEventRequest: { ...body, type: v.type } })
    if (res.data) {
      toast.success(t(event ? 'calendar.form.updated' : 'calendar.form.created'))
      onClose()
      return
    }
    const fields = fieldErrors(res.error)
    const map = {
      type: 'type',
      starts_at: 'startsAt',
      ends_at: 'endsAt',
      scope_id: 'scopeId',
      note: 'note',
    } as const
    for (const [server, local] of Object.entries(map)) {
      const e = fields[server]?.[0]
      if (e) {
        form.setError(local, { message: e.message })
      }
    }
  })

  const generalError =
    error && Object.keys(fieldErrors(error)).length === 0
      ? message(error, {
          EVENT_OVERLAP: t('calendar.form.errors.EVENT_OVERLAP'),
          VERSION_MISMATCH: t('calendar.form.errors.VERSION_MISMATCH'),
        })
      : null
  const e = form.formState.errors

  return (
    <FormDialog
      title={t(event ? 'calendar.form.editTitle' : 'calendar.form.newTitle')}
      description={t('calendar.form.description')}
      error={generalError}
      onClose={onClose}
      onSubmit={() => void submit()}
      submit={
        <Button type="submit" loading={created.isLoading || updated.isLoading}>
          {t('common.save')}
        </Button>
      }
    >
      <Field label={t('calendar.form.type')} error={e.type?.message}>
        {(p) => (
          <Controller
            control={form.control}
            name="type"
            render={({ field }) => (
              <Select value={field.value} onValueChange={field.onChange} disabled={Boolean(event)}>
                <SelectTrigger {...p} className="w-full">
                  <SelectValue placeholder={t('calendar.form.typePlaceholder')} />
                </SelectTrigger>
                <SelectContent>
                  {types.data?.items.map((ty) => (
                    <SelectItem key={ty.code} value={ty.code}>
                      {localized(ty)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            )}
          />
        )}
      </Field>
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label={t('calendar.form.startsAt')} error={e.startsAt?.message}>
          {(p) => <Input {...p} {...form.register('startsAt')} type="datetime-local" />}
        </Field>
        <Field
          label={t('calendar.form.endsAt')}
          error={e.endsAt?.message}
          hint={t('calendar.form.endsAtHint')}
        >
          {(p) => <Input {...p} {...form.register('endsAt')} type="datetime-local" />}
        </Field>
      </div>
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label={t('calendar.form.scope')}>
          {(p) => (
            <Controller
              control={form.control}
              name="scopeType"
              render={({ field }) => (
                <Select
                  value={field.value}
                  onValueChange={field.onChange}
                  disabled={event?.scope_type === 'PROGRAM'}
                >
                  <SelectTrigger {...p} className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="UNIVERSITY">{t('calendar.scopes.UNIVERSITY')}</SelectItem>
                    <SelectItem value="FACULTY">{t('calendar.scopes.FACULTY')}</SelectItem>
                    {event?.scope_type === 'PROGRAM' && (
                      <SelectItem value="PROGRAM">{t('calendar.scopes.PROGRAM')}</SelectItem>
                    )}
                  </SelectContent>
                </Select>
              )}
            />
          )}
        </Field>
        {scopeType === 'FACULTY' && (
          <Field label={t('calendar.form.faculty')} error={e.scopeId?.message}>
            {(p) => (
              <Controller
                control={form.control}
                name="scopeId"
                render={({ field }) => (
                  <Select value={field.value} onValueChange={field.onChange}>
                    <SelectTrigger {...p} className="w-full">
                      <SelectValue placeholder={t('calendar.form.facultyPlaceholder')} />
                    </SelectTrigger>
                    <SelectContent>
                      {faculties.data?.items.map((f) => (
                        <SelectItem key={f.id} value={f.id}>
                          {localized(f)}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                )}
              />
            )}
          </Field>
        )}
        {scopeType === 'PROGRAM' && event?.scope && (
          <Field label={t('calendar.scopes.PROGRAM')}>
            {(p) => <Input {...p} value={event.scope?.name ?? ''} readOnly />}
          </Field>
        )}
      </div>
      <div className="grid gap-4 sm:grid-cols-2">
        <Field
          label={t('calendar.form.titleTr')}
          hint={t('calendar.form.titleHint')}
          error={e.titleTr?.message}
        >
          {(p) => <Input {...p} {...form.register('titleTr')} />}
        </Field>
        <Field label={t('calendar.form.titleEn')} error={e.titleEn?.message}>
          {(p) => <Input {...p} {...form.register('titleEn')} />}
        </Field>
      </div>
      <Field label={t('calendar.form.note')} error={e.note?.message}>
        {(p) => <Textarea {...p} {...form.register('note')} rows={2} />}
      </Field>
      <Controller
        control={form.control}
        name="published"
        render={({ field }) => (
          <div className="flex items-start gap-2">
            <Checkbox
              id="event-published"
              checked={field.value}
              onCheckedChange={(v) => field.onChange(v === true)}
            />
            <div className="grid gap-1">
              <Label htmlFor="event-published">{t('calendar.form.published')}</Label>
              <p className="text-sm text-muted-foreground">{t('calendar.form.publishedHint')}</p>
            </div>
          </div>
        )}
      />
    </FormDialog>
  )
}
