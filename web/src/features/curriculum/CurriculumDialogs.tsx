import { zodResolver } from '@hookform/resolvers/zod'
import { useMemo, useState } from 'react'
import { Controller, useForm, useWatch } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

import { CourseSearch } from '@/features/catalog/CourseSearch'
import { ifMatch } from '@/shared/api/etag'
import { fieldErrors } from '@/shared/api/errors'
import {
  useAddCurriculumItemMutation,
  useCreateCurriculumMutation,
  useListElectiveGroupsQuery,
  useUpdateCurriculumMutation,
  type Course,
  type Curriculum,
  type CurriculumItemRequest,
  type CurriculumRequest,
} from '@/shared/api/generated'
import { useErrorMessage } from '@/shared/api/useErrorMessage'
import { useLocalized } from '@/shared/lib/localized'
import { Button } from '@/shared/ui/button'
import { Field } from '@/shared/ui/field'
import { FormDialog } from '@/shared/ui/form-dialog'
import { Input } from '@/shared/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/shared/ui/select'
import { Tabs, TabsList, TabsTrigger } from '@/shared/ui/tabs'

const NONE = 'NONE'

/**
 * VersionDialog, programa yeni bir taslak sürüm açar (çoğunlukla bir öncekinden
 * kopyalanarak) ya da bir sürümün bilgilerini düzenler. Yürürlükteki sürümde giriş yılı
 * başlangıcı ve AKTS toplamı değişmez (sunucu reddeder).
 */
export function VersionDialog({
  programId,
  versions,
  editing,
  onClose,
  onCreated,
}: {
  programId: string
  versions: Curriculum[]
  editing?: Curriculum
  onClose: () => void
  onCreated?: (id: string) => void
}) {
  const { t } = useTranslation()
  const message = useErrorMessage()
  const localized = useLocalized()
  const [create, created] = useCreateCurriculumMutation()
  const [update, updated] = useUpdateCurriculumMutation()
  const error = created.error ?? updated.error
  const latest = versions[0]

  const schema = useMemo(() => {
    const required = t('common.required')
    const year = z.coerce.number<string>(required).int().min(1946).max(2100)
    return z
      .object({
        nameTr: z.string().trim().min(1, required).max(200),
        nameEn: z.string().trim().min(1, required).max(200),
        from: year,
        to: z.string(),
        total: z.coerce.number<string>(required).positive().max(999),
        decision: z.string().trim().max(200),
        copyFrom: z.string(),
      })
      .refine((v) => v.to === '' || Number(v.to) >= v.from, {
        path: ['to'],
        message: t('curriculum.form.toAfterFrom'),
      })
  }, [t])

  const form = useForm({
    resolver: zodResolver(schema),
    defaultValues: {
      nameTr: editing?.name_tr ?? '',
      nameEn: editing?.name_en ?? '',
      from: String(editing?.effective_from_year ?? new Date().getFullYear()),
      to: editing?.effective_to_year ? String(editing.effective_to_year) : '',
      total: String(editing?.total_ects_required ?? latest?.total_ects_required ?? 240),
      decision: editing?.decision_ref ?? '',
      copyFrom: latest && !editing ? latest.id : NONE,
    },
  })

  const submit = form.handleSubmit(async (v) => {
    const body: CurriculumRequest = {
      name_tr: v.nameTr,
      name_en: v.nameEn,
      effective_from_year: v.from,
      effective_to_year: v.to === '' ? null : Number(v.to),
      total_ects_required: v.total,
      ...(v.decision && { decision_ref: v.decision }),
    }
    const res = editing
      ? await update({ id: editing.id, 'If-Match': ifMatch(editing.version), curriculumRequest: body })
      : await create({
          id: programId,
          body: { ...body, ...(v.copyFrom !== NONE && { copy_from_id: v.copyFrom }) },
        })
    if (res.data) {
      toast.success(t(editing ? 'curriculum.form.updated' : 'curriculum.form.created'))
      onCreated?.(res.data.id)
      onClose()
      return
    }
    const fields = fieldErrors(res.error)
    const map = {
      name_tr: 'nameTr',
      name_en: 'nameEn',
      effective_from_year: 'from',
      effective_to_year: 'to',
      total_ects_required: 'total',
    } as const
    for (const [server, local] of Object.entries(map)) {
      const e = fields[server]?.[0]
      if (e) {
        form.setError(local, { message: e.message })
      }
    }
  })

  const generalError = error && Object.keys(fieldErrors(error)).length === 0 ? message(error) : null
  const e = form.formState.errors

  return (
    <FormDialog
      title={t(editing ? 'curriculum.form.editTitle' : 'curriculum.form.newTitle')}
      description={t(editing ? 'curriculum.form.editDescription' : 'curriculum.form.newDescription')}
      error={generalError}
      onClose={onClose}
      onSubmit={() => void submit()}
      submit={
        <Button type="submit" loading={created.isLoading || updated.isLoading}>
          {t('common.save')}
        </Button>
      }
    >
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label={t('curriculum.form.nameTr')} error={e.nameTr?.message}>
          {(p) => <Input {...p} {...form.register('nameTr')} placeholder="2027 Ders Planı" />}
        </Field>
        <Field label={t('curriculum.form.nameEn')} error={e.nameEn?.message}>
          {(p) => <Input {...p} {...form.register('nameEn')} placeholder="2027 Curriculum" />}
        </Field>
      </div>
      <div className="grid gap-4 sm:grid-cols-3">
        <Field label={t('curriculum.form.from')} error={e.from?.message}>
          {(p) => <Input {...p} {...form.register('from')} inputMode="numeric" />}
        </Field>
        <Field label={t('curriculum.form.to')} error={e.to?.message} hint={t('curriculum.form.toHint')}>
          {(p) => <Input {...p} {...form.register('to')} inputMode="numeric" />}
        </Field>
        <Field label={t('curriculum.form.total')} error={e.total?.message}>
          {(p) => <Input {...p} {...form.register('total')} inputMode="decimal" />}
        </Field>
      </div>
      <Field label={t('curriculum.form.decision')} hint={t('curriculum.form.decisionHint')}>
        {(p) => <Input {...p} {...form.register('decision')} />}
      </Field>
      {!editing && (
        <Field label={t('curriculum.form.copyFrom')}>
          {(p) => (
            <Controller
              control={form.control}
              name="copyFrom"
              render={({ field }) => (
                <Select value={field.value} onValueChange={field.onChange}>
                  <SelectTrigger {...p} className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value={NONE}>{t('curriculum.form.empty')}</SelectItem>
                    {versions.map((v) => (
                      <SelectItem key={v.id} value={v.id}>
                        {localized(v)}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              )}
            />
          )}
        </Field>
      )}
    </FormDialog>
  )
}

/** ItemDialog, taslak sürüme bir ders ya da seçmeli yuva ekler. */
export function ItemDialog({ curriculumId, onClose }: { curriculumId: string; onClose: () => void }) {
  const { t } = useTranslation()
  const message = useErrorMessage()
  const localized = useLocalized()
  const groups = useListElectiveGroupsQuery({})
  const [add, { error, isLoading }] = useAddCurriculumItemMutation()
  const [course, setCourse] = useState<Course | null>(null)
  const [courseError, setCourseError] = useState<string | undefined>()

  const schema = useMemo(() => {
    const required = t('common.required')
    const num = (max: number) => z.coerce.number<string>(required).min(0).max(max)
    return z
      .object({
        type: z.enum(['COURSE', 'ELECTIVE_SLOT']),
        semester: z.coerce.number<string>(required).int().min(1).max(12),
        group: z.string(),
        theory: num(40),
        practice: num(40),
        credit: num(60),
        ects: num(60),
        count: z.coerce.number<string>(required).int().min(1).max(20),
      })
      .refine((v) => v.type === 'COURSE' || v.group !== '', { path: ['group'], message: required })
  }, [t])

  const form = useForm({
    resolver: zodResolver(schema),
    defaultValues: {
      type: 'COURSE' as const,
      semester: '1',
      group: '',
      theory: '3',
      practice: '0',
      credit: '3',
      ects: '4',
      count: '1',
    },
  })
  const type = useWatch({ control: form.control, name: 'type' })

  const submit = form.handleSubmit(async (v) => {
    if (v.type === 'COURSE' && !course) {
      setCourseError(t('common.required'))
      return
    }
    const body: CurriculumItemRequest =
      v.type === 'COURSE'
        ? { semester_no: v.semester, item_type: 'COURSE', course_id: course?.id ?? '' }
        : {
            semester_no: v.semester,
            item_type: 'ELECTIVE_SLOT',
            elective_group_id: v.group,
            theory_hours: v.theory,
            practice_hours: v.practice,
            national_credit: v.credit,
            ects: v.ects,
            course_count: v.count,
          }
    const res = await add({ id: curriculumId, curriculumItemRequest: body })
    if (res.data) {
      toast.success(t('curriculum.edit.itemAdded'))
      onClose()
    }
  })

  const generalError = error
    ? message(error, { COURSE_ALREADY_IN_CURRICULUM: t('curriculum.errors.COURSE_ALREADY_IN_CURRICULUM') })
    : null
  const e = form.formState.errors

  return (
    <FormDialog
      title={t('curriculum.edit.addTitle')}
      description={t('curriculum.edit.addDescription')}
      error={generalError}
      onClose={onClose}
      onSubmit={() => void submit()}
      submit={
        <Button type="submit" loading={isLoading}>
          {t('curriculum.edit.add')}
        </Button>
      }
    >
      <Controller
        control={form.control}
        name="type"
        render={({ field }) => (
          <Tabs value={field.value} onValueChange={field.onChange}>
            <TabsList className="w-full">
              <TabsTrigger value="COURSE">{t('curriculum.edit.course')}</TabsTrigger>
              <TabsTrigger value="ELECTIVE_SLOT">{t('curriculum.edit.slot')}</TabsTrigger>
            </TabsList>
          </Tabs>
        )}
      />
      <Field label={t('curriculum.edit.semester')} error={e.semester?.message}>
        {(p) => <Input {...p} {...form.register('semester')} inputMode="numeric" className="w-24" />}
      </Field>
      {type === 'COURSE' ? (
        <Field label={t('curriculum.edit.course')} error={courseError}>
          {(p) => (
            <CourseSearch
              id={p.id}
              value={course}
              invalid={p['aria-invalid']}
              describedBy={p['aria-describedby']}
              onChange={(c) => {
                setCourse(c)
                setCourseError(undefined)
              }}
            />
          )}
        </Field>
      ) : (
        <>
          <Field label={t('curriculum.edit.group')} error={e.group?.message}>
            {(p) => (
              <Controller
                control={form.control}
                name="group"
                render={({ field }) => (
                  <Select value={field.value} onValueChange={field.onChange}>
                    <SelectTrigger {...p} className="w-full">
                      <SelectValue placeholder={t('org.choose')} />
                    </SelectTrigger>
                    <SelectContent>
                      {groups.data?.items.map((g) => (
                        <SelectItem key={g.id} value={g.id}>
                          {g.code} · {localized(g)}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                )}
              />
            )}
          </Field>
          <div className="grid grid-cols-2 gap-4 sm:grid-cols-5">
            <Field label={t('curriculum.edit.count')} error={e.count?.message}>
              {(p) => <Input {...p} {...form.register('count')} inputMode="numeric" />}
            </Field>
            <Field label={t('curriculum.edit.theory')} error={e.theory?.message}>
              {(p) => <Input {...p} {...form.register('theory')} inputMode="numeric" />}
            </Field>
            <Field label={t('curriculum.edit.practice')} error={e.practice?.message}>
              {(p) => <Input {...p} {...form.register('practice')} inputMode="numeric" />}
            </Field>
            <Field label={t('catalog.credit')} error={e.credit?.message}>
              {(p) => <Input {...p} {...form.register('credit')} inputMode="decimal" />}
            </Field>
            <Field label={t('catalog.ects')} error={e.ects?.message}>
              {(p) => <Input {...p} {...form.register('ects')} inputMode="decimal" />}
            </Field>
          </div>
          <p className="text-xs text-muted-foreground">{t('curriculum.edit.slotHint')}</p>
        </>
      )}
    </FormDialog>
  )
}
