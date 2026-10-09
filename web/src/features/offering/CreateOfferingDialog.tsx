import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router'
import { toast } from 'sonner'

import { CourseSearch } from '@/features/catalog/CourseSearch'
import { useCreateOfferingMutation, type Course } from '@/shared/api/generated'
import { useErrorMessage } from '@/shared/api/useErrorMessage'
import { Button } from '@/shared/ui/button'
import { Field } from '@/shared/ui/field'
import { FormDialog } from '@/shared/ui/form-dialog'
import { Input } from '@/shared/ui/input'

/**
 * CreateOfferingDialog, seçili bölüm adına dönemde bir ders açar. Açılan ders planlama
 * durumunda başlar; şubeler ve program ayrıntı sayfasında kurulur.
 */
export function CreateOfferingDialog({
  termId,
  departmentId,
  onClose,
}: {
  termId: string
  departmentId: string
  onClose: () => void
}) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const message = useErrorMessage()
  const [create, { error, isLoading }] = useCreateOfferingMutation()
  const [course, setCourse] = useState<Course | null>(null)
  const [externalRef, setExternalRef] = useState('')
  const [missing, setMissing] = useState(false)

  const submit = async () => {
    if (!course) {
      setMissing(true)
      return
    }
    const res = await create({
      id: termId,
      offeringCreateRequest: {
        course_id: course.id,
        department_id: departmentId,
        ...(externalRef.trim() && { external_ref: externalRef.trim() }),
      },
    })
    if (res.data) {
      toast.success(t('offering.create.success'))
      onClose()
      void navigate(`/ders-acma/${res.data.id}`)
    }
  }

  return (
    <FormDialog
      title={t('offering.create.title')}
      description={t('offering.create.description')}
      error={
        error ? message(error, { COURSE_ALREADY_OFFERED: t('offering.errors.COURSE_ALREADY_OFFERED') }) : null
      }
      onClose={onClose}
      onSubmit={() => void submit()}
      submit={
        <Button type="submit" loading={isLoading}>
          {t('offering.create.submit')}
        </Button>
      }
    >
      <Field label={t('curriculum.edit.course')} error={missing ? t('common.required') : undefined}>
        {(p) => (
          <CourseSearch
            id={p.id}
            value={course}
            invalid={p['aria-invalid']}
            describedBy={p['aria-describedby']}
            onChange={(c) => {
              setCourse(c)
              setMissing(false)
            }}
          />
        )}
      </Field>
      <Field label={t('offering.create.externalRef')} hint={t('offering.create.externalRefHint')}>
        {(p) => (
          <Input {...p} value={externalRef} onChange={(e) => setExternalRef(e.target.value)} maxLength={50} />
        )}
      </Field>
    </FormDialog>
  )
}
