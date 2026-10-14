import { FilePenLineIcon } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { TermSelect } from '@/features/calendar/TermSelect'
import {
  useGetCurrentTermQuery,
  useGetGradebookQuery,
  useGetLetterGradesQuery,
  useMyTeachingQuery,
  usePreviewGradeChangeQuery,
  useRequestGradeChangeMutation,
  type GradeChangeInput,
} from '@/shared/api/generated'
import { useErrorMessage } from '@/shared/api/useErrorMessage'
import { useLocalized } from '@/shared/lib/localized'
import { Alert, AlertDescription } from '@/shared/ui/alert'
import { Button } from '@/shared/ui/button'
import { Checkbox } from '@/shared/ui/checkbox'
import { FormDialog } from '@/shared/ui/form-dialog'
import { Input } from '@/shared/ui/input'
import { Label } from '@/shared/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/shared/ui/select'
import { Spinner } from '@/shared/ui/spinner'
import { Textarea } from '@/shared/ui/textarea'

import { GradeDiff } from './ChangeCard'

/**
 * NewChangeDialog, öğretim elemanının not düzeltme talebidir: dönem, şube ve öğrenci seçilir;
 * bir bileşenin puanı düzeltilir (harf yeniden hesaplanır) ya da harf doğrudan değiştirilir.
 * Sonuç gerekçe yazılmadan önce sunucuda hesaplanıp gösterilir.
 */
export function NewChangeDialog({ onClose }: { onClose: () => void }) {
  const { t, i18n } = useTranslation()
  const localized = useLocalized()
  const message = useErrorMessage()
  const current = useGetCurrentTermQuery()
  const [termId, setTermId] = useState('')
  const term = termId || current.data?.id || ''
  const teaching = useMyTeachingQuery({ termId: term }, { skip: !term })
  const [sectionId, setSectionId] = useState('')
  const book = useGetGradebookQuery({ id: sectionId }, { skip: !sectionId })
  const letters = useGetLetterGradesQuery({ id: sectionId }, { skip: !sectionId })
  const [enrollmentId, setEnrollmentId] = useState('')
  const [kind, setKind] = useState<'SCORE' | 'LETTER'>('SCORE')
  const [componentId, setComponentId] = useState('')
  const [score, setScore] = useState('')
  const [absent, setAbsent] = useState(false)
  const [letter, setLetter] = useState('')
  const [reason, setReason] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [request, { isLoading }] = useRequestGradeChangeMutation()

  const sections = new Map<string, { code: string; name: string }>()
  for (const e of teaching.data?.items ?? []) {
    sections.set(e.section_id, { code: `${e.course.code}-${e.section_code}`, name: localized(e.course) })
  }
  const published = Boolean(book.data?.section.grades_published_at)
  const students = (book.data?.students ?? []).filter((s) => s.letter)
  const en = i18n.language.startsWith('en')

  const body: GradeChangeInput | null = !enrollmentId
    ? null
    : kind === 'LETTER'
      ? letter
        ? { enrollment_id: enrollmentId, letter }
        : null
      : componentId && (absent || (score !== '' && !Number.isNaN(Number(score))))
        ? {
            enrollment_id: enrollmentId,
            component_id: componentId,
            ...(absent ? { absent: true } : { score: Number(score) }),
          }
        : null
  // Puan yazılırken her tuşta değil, kısa bir duraksamadan sonra hesaplatılır.
  const [settled, setSettled] = useState<GradeChangeInput | null>(null)
  const key = JSON.stringify(body)
  useEffect(() => {
    const timer = setTimeout(() => setSettled(JSON.parse(key) as GradeChangeInput | null), 300)
    return () => clearTimeout(timer)
  }, [key])
  const preview = usePreviewGradeChangeQuery({ gradeChangeInput: settled! }, { skip: !settled })
  const shown = settled && JSON.stringify(settled) === key ? preview : undefined

  const reset = (level: 'section' | 'student') => {
    if (level === 'section') {
      setEnrollmentId('')
    }
    setComponentId('')
    setScore('')
    setAbsent(false)
    setLetter('')
  }

  return (
    <FormDialog
      title={t('gradeChanges.new.title')}
      description={t('gradeChanges.new.description')}
      icon={FilePenLineIcon}
      tone="amber"
      error={error}
      onClose={onClose}
      onSubmit={() => {
        if (!body) {
          setError(t('gradeChanges.new.incomplete'))
          return
        }
        if (reason.trim().length < 10) {
          setError(t('gradeChanges.new.reasonRequired'))
          return
        }
        void request({ gradeChangeInput: { ...body, reason: reason.trim() } }).then((res) => {
          if ('error' in res && res.error) {
            setError(message(res.error))
            return
          }
          toast.success(t('gradeChanges.new.sent'))
          onClose()
        })
      }}
      submit={
        <Button type="submit" loading={isLoading} disabled={!body || !published}>
          {t('gradeChanges.new.submit')}
        </Button>
      }
    >
      <div className="space-y-3">
        <div className="space-y-1.5">
          <Label>{t('gradeChanges.new.term')}</Label>
          <TermSelect
            value={term}
            onChange={(id) => {
              setTermId(id)
              setSectionId('')
              reset('section')
            }}
          />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="change-section">{t('gradeChanges.new.section')}</Label>
          <Select
            value={sectionId}
            onValueChange={(id) => {
              setSectionId(id)
              reset('section')
            }}
          >
            <SelectTrigger id="change-section" className="w-full">
              <SelectValue placeholder={t('gradeChanges.new.choose')} />
            </SelectTrigger>
            <SelectContent>
              {[...sections].map(([id, s]) => (
                <SelectItem key={id} value={id}>
                  {s.code} {s.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </div>
      {sectionId && book.data && !published && (
        <Alert>
          <AlertDescription>{t('gradeChanges.new.notPublished')}</AlertDescription>
        </Alert>
      )}
      {sectionId && published && (
        <>
          <div className="space-y-1.5">
            <Label htmlFor="change-student">{t('gradeChanges.student')}</Label>
            <Select
              value={enrollmentId}
              onValueChange={(id) => {
                setEnrollmentId(id)
                reset('student')
              }}
            >
              <SelectTrigger id="change-student" className="w-full">
                <SelectValue placeholder={t('gradeChanges.new.choose')} />
              </SelectTrigger>
              <SelectContent>
                {students.map((s) => (
                  <SelectItem key={s.enrollment_id} value={s.enrollment_id}>
                    {s.student.student_no} · {s.student.first_name} {s.student.last_name} · {s.letter}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          {enrollmentId && (
            <fieldset className="space-y-3">
              <legend className="sr-only">{t('gradeChanges.new.kind')}</legend>
              <div className="flex gap-1.5" role="radiogroup" aria-label={t('gradeChanges.new.kind')}>
                {(['SCORE', 'LETTER'] as const).map((k) => (
                  <Button
                    key={k}
                    type="button"
                    size="sm"
                    role="radio"
                    aria-checked={kind === k}
                    variant={kind === k ? 'soft' : 'ghost'}
                    onClick={() => {
                      setKind(k)
                      reset('student')
                    }}
                  >
                    {t(`gradeChanges.kinds.${k}`)}
                  </Button>
                ))}
              </div>
              {kind === 'SCORE' ? (
                <div className="grid gap-3 sm:grid-cols-[1fr_8rem]">
                  <div className="space-y-1.5">
                    <Label htmlFor="change-component">{t('gradeChanges.new.component')}</Label>
                    <Select value={componentId} onValueChange={setComponentId}>
                      <SelectTrigger id="change-component" className="w-full">
                        <SelectValue placeholder={t('gradeChanges.new.choose')} />
                      </SelectTrigger>
                      <SelectContent>
                        {(book.data?.components ?? []).map((c) => {
                          const existing = book.data?.students.find((s) => s.enrollment_id === enrollmentId)
                            ?.scores[c.id]
                          const value = existing
                            ? existing.absent
                              ? t('grades.didNotSit')
                              : existing.score
                            : '—'
                          return (
                            <SelectItem key={c.id} value={c.id}>
                              {en ? c.label_en : c.label_tr} (%{c.weight}) · {value}
                            </SelectItem>
                          )
                        })}
                      </SelectContent>
                    </Select>
                  </div>
                  <div className="space-y-1.5">
                    <Label htmlFor="change-score">{t('gradeChanges.new.score')}</Label>
                    <Input
                      id="change-score"
                      inputMode="decimal"
                      value={score}
                      disabled={absent}
                      onChange={(e) => setScore(e.target.value.replace(',', '.'))}
                    />
                  </div>
                  <label className="flex items-center gap-2 text-sm sm:col-span-2">
                    <Checkbox checked={absent} onCheckedChange={(v) => setAbsent(v === true)} />
                    {t('gradeChanges.new.absent')}
                  </label>
                </div>
              ) : (
                <div className="space-y-1.5">
                  <Label htmlFor="change-letter">{t('gradeChanges.new.letter')}</Label>
                  <Select value={letter} onValueChange={setLetter}>
                    <SelectTrigger id="change-letter" className="w-full">
                      <SelectValue placeholder={t('gradeChanges.new.choose')} />
                    </SelectTrigger>
                    <SelectContent>
                      {(letters.data?.scale ?? []).map((g) => (
                        <SelectItem key={g.letter} value={g.letter}>
                          {g.letter}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
              )}
            </fieldset>
          )}
          {shown && (
            <div
              className="rounded-xl border border-amber-400/40 bg-amber-50/60 p-3 dark:bg-amber-950/30"
              role="status"
            >
              <p className="mb-2 text-xs font-semibold text-muted-foreground">
                {t('gradeChanges.new.preview')}
              </p>
              {shown.isFetching ? (
                <Spinner className="size-4" />
              ) : shown.error ? (
                <p className="text-sm text-destructive">{message(shown.error)}</p>
              ) : shown.data ? (
                <GradeDiff
                  kind={shown.data.kind}
                  componentLabel={
                    shown.data.component
                      ? en
                        ? shown.data.component.label_en
                        : shown.data.component.label_tr
                      : undefined
                  }
                  old={shown.data.old}
                  next={shown.data.new}
                />
              ) : null}
            </div>
          )}
          <div className="space-y-1.5">
            <Label htmlFor="change-reason">{t('gradeChanges.new.reason')}</Label>
            <Textarea
              id="change-reason"
              value={reason}
              maxLength={500}
              placeholder={t('gradeChanges.new.reasonHint')}
              onChange={(e) => setReason(e.target.value)}
            />
          </div>
        </>
      )}
    </FormDialog>
  )
}
