import { SearchIcon, Trash2Icon } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { day } from '@/features/schedule/layout'
import { ifMatch } from '@/shared/api/etag'
import { fieldErrors } from '@/shared/api/errors'
import {
  useAddScheduleSlotMutation,
  useCreateSectionMutation,
  useListBuildingsQuery,
  useListClassroomsQuery,
  useListProgramsQuery,
  useSearchInstructorsQuery,
  useSetSectionInstructorsMutation,
  useSetSectionQuotasMutation,
  useUpdateSectionMutation,
  type InstructorRole,
  type OfferingDetail,
  type Section,
  type SectionRequest,
} from '@/shared/api/generated'
import { useErrorMessage } from '@/shared/api/useErrorMessage'
import { useDebounced } from '@/shared/lib/useDebounced'
import { useLocalized } from '@/shared/lib/localized'
import { Button } from '@/shared/ui/button'
import { Field } from '@/shared/ui/field'
import { FormDialog } from '@/shared/ui/form-dialog'
import { Input } from '@/shared/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/shared/ui/select'

const ONLINE = 'ONLINE'

/** firstError, sunucunun alan hatalarından ilkini gösterir (formda karşılığı olmayan alanlar için). */
function useFirstError() {
  const message = useErrorMessage()
  return (error: Parameters<typeof message>[0], messages: Parameters<typeof message>[1] = {}) => {
    if (!error) {
      return null
    }
    const fields = Object.values(fieldErrors(error))
    return fields[0]?.[0]?.message ?? message(error, messages)
  }
}

/** SectionDialog, açılan derse şube ekler (offering) ya da bir şubeyi düzenler (section). */
export function SectionDialog({
  offering,
  section,
  onClose,
}: {
  offering?: OfferingDetail
  section?: Section
  onClose: () => void
}) {
  const { t } = useTranslation()
  const firstError = useFirstError()
  const [create, created] = useCreateSectionMutation()
  const [update, updated] = useUpdateSectionMutation()
  const nextCode = String((offering?.sections.length ?? 0) + 1)
  const [form, setForm] = useState({
    code: section?.section_code ?? nextCode,
    capacity: String(section?.capacity ?? 60),
    quotaMode: section?.quota_mode ?? 'OPEN',
    mode: section?.instruction_mode ?? 'IN_PERSON',
    language: section?.language ?? offering?.course.language ?? 'TR',
  })
  const error = created.error ?? updated.error

  const submit = async () => {
    const body: SectionRequest = {
      capacity: Number(form.capacity),
      quota_mode: form.quotaMode,
      instruction_mode: form.mode,
      language: form.language,
    }
    const res = section
      ? await update({ id: section.id, 'If-Match': ifMatch(section.version), sectionRequest: body })
      : await create({ id: offering?.id ?? '', sectionRequest: { ...body, section_code: form.code.trim() } })
    if (res.data) {
      toast.success(t(section ? 'offering.section.updated' : 'offering.section.created'))
      onClose()
    }
  }

  return (
    <FormDialog
      title={
        section ? t('offering.section.editTitle', { code: section.section_code }) : t('offering.section.add')
      }
      description={t('offering.section.dialogDescription')}
      error={firstError(error, { SECTION_CODE_TAKEN: t('offering.errors.SECTION_CODE_TAKEN') })}
      onClose={onClose}
      onSubmit={() => void submit()}
      submit={
        <Button type="submit" loading={created.isLoading || updated.isLoading}>
          {t('common.save')}
        </Button>
      }
    >
      <div className="grid gap-4 sm:grid-cols-2">
        {!section && (
          <Field label={t('offering.section.code')} hint={t('offering.section.codeHint')}>
            {(p) => (
              <Input
                {...p}
                value={form.code}
                onChange={(e) => setForm({ ...form, code: e.target.value })}
                maxLength={3}
              />
            )}
          </Field>
        )}
        <Field label={t('offering.section.capacity')}>
          {(p) => (
            <Input
              {...p}
              value={form.capacity}
              onChange={(e) => setForm({ ...form, capacity: e.target.value })}
              inputMode="numeric"
            />
          )}
        </Field>
        <Field label={t('offering.section.quotaMode')} hint={t('offering.section.quotaModeHint')}>
          {(p) => (
            <Select
              value={form.quotaMode}
              onValueChange={(v) => setForm({ ...form, quotaMode: v as typeof form.quotaMode })}
            >
              <SelectTrigger {...p} className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="OPEN">{t('offering.section.quotaModes.OPEN')}</SelectItem>
                <SelectItem value="RESERVED">{t('offering.section.quotaModes.RESERVED')}</SelectItem>
              </SelectContent>
            </Select>
          )}
        </Field>
        <Field label={t('offering.section.mode')}>
          {(p) => (
            <Select
              value={form.mode}
              onValueChange={(v) => setForm({ ...form, mode: v as typeof form.mode })}
            >
              <SelectTrigger {...p} className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {(['IN_PERSON', 'ONLINE', 'HYBRID'] as const).map((m) => (
                  <SelectItem key={m} value={m}>
                    {t(`offering.section.modes.${m}`)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          )}
        </Field>
        <Field label={t('catalog.detail.language')}>
          {(p) => (
            <Select
              value={form.language}
              onValueChange={(v) => setForm({ ...form, language: v as typeof form.language })}
            >
              <SelectTrigger {...p} className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="TR">{t('catalog.languages.TR')}</SelectItem>
                <SelectItem value="EN">{t('catalog.languages.EN')}</SelectItem>
              </SelectContent>
            </Select>
          )}
        </Field>
      </div>
    </FormDialog>
  )
}

/**
 * SlotDialog, şubeye haftalık bir oturum ekler. Derslik, şubenin başka oturumu ya da öğretim
 * elemanının başka bir dersiyle çakışırsa sunucu reddeder; mesaj çakışan dersi ve saati söyler.
 */
export function SlotDialog({ section, onClose }: { section: Section; onClose: () => void }) {
  const { t } = useTranslation()
  const firstError = useFirstError()
  const [add, { error, isLoading }] = useAddScheduleSlotMutation()
  const [form, setForm] = useState({
    day: '1',
    start: '09:00',
    end: '11:50',
    type: 'THEORY' as 'THEORY' | 'PRACTICE' | 'LAB',
    building: '',
    classroom: '',
  })
  const buildings = useListBuildingsQuery({})
  const classrooms = useListClassroomsQuery(
    { buildingId: form.building, limit: 100 },
    { skip: !form.building || form.building === ONLINE },
  )

  const submit = async () => {
    const res = await add({
      id: section.id,
      scheduleSlotRequest: {
        day_of_week: Number(form.day),
        start_time: form.start,
        end_time: form.end,
        session_type: form.type,
        ...(form.classroom && form.building !== ONLINE && { classroom_id: form.classroom }),
      },
    })
    if (res.data) {
      toast.success(t('offering.slot.added'))
      onClose()
    }
  }

  return (
    <FormDialog
      title={t('offering.slot.add')}
      description={t('offering.slot.description', { capacity: section.capacity })}
      error={firstError(error)}
      onClose={onClose}
      onSubmit={() => void submit()}
      submit={
        <Button type="submit" loading={isLoading}>
          {t('offering.slot.add')}
        </Button>
      }
    >
      <div className="grid gap-4 sm:grid-cols-3">
        <Field label={t('offering.slot.day')}>
          {(p) => (
            <Select value={form.day} onValueChange={(v) => setForm({ ...form, day: v })}>
              <SelectTrigger {...p} className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {[1, 2, 3, 4, 5, 6, 7].map((d) => (
                  <SelectItem key={d} value={String(d)}>
                    {t(`schedule.days.${day(d)}`)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          )}
        </Field>
        <Field label={t('offering.slot.start')}>
          {(p) => (
            <Input
              {...p}
              type="time"
              value={form.start}
              onChange={(e) => setForm({ ...form, start: e.target.value })}
            />
          )}
        </Field>
        <Field label={t('offering.slot.end')} hint={t('offering.slot.endHint')}>
          {(p) => (
            <Input
              {...p}
              type="time"
              value={form.end}
              onChange={(e) => setForm({ ...form, end: e.target.value })}
            />
          )}
        </Field>
      </div>
      <Field label={t('offering.slot.type')}>
        {(p) => (
          <Select value={form.type} onValueChange={(v) => setForm({ ...form, type: v as typeof form.type })}>
            <SelectTrigger {...p} className="w-full sm:w-56">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {(['THEORY', 'PRACTICE', 'LAB'] as const).map((s) => (
                <SelectItem key={s} value={s}>
                  {t(`schedule.sessions.${s}`)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        )}
      </Field>
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label={t('schedule.building')}>
          {(p) => (
            <Select
              value={form.building}
              onValueChange={(v) => setForm({ ...form, building: v, classroom: '' })}
            >
              <SelectTrigger {...p} className="w-full">
                <SelectValue placeholder={t('org.choose')} />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={ONLINE}>{t('offering.slot.online')}</SelectItem>
                {buildings.data?.items.map((b) => (
                  <SelectItem key={b.id} value={b.id}>
                    {b.code} · {b.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          )}
        </Field>
        {form.building && form.building !== ONLINE && (
          <Field label={t('schedule.classroom')}>
            {(p) => (
              <Select value={form.classroom} onValueChange={(v) => setForm({ ...form, classroom: v })}>
                <SelectTrigger {...p} className="w-full">
                  <SelectValue placeholder={t('org.choose')} />
                </SelectTrigger>
                <SelectContent>
                  {classrooms.data?.items.map((c) => (
                    <SelectItem key={c.id} value={c.id}>
                      {c.code} · {t('schedule.capacity', { count: c.capacity })}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            )}
          </Field>
        )}
      </div>
    </FormDialog>
  )
}

type Assignment = { staffId: string; name: string; role: InstructorRole }

/** InstructorsDialog, şubenin öğretim elemanlarını düzenler: tek sorumlu, ortak ve yardımcılar. */
export function InstructorsDialog({ section, onClose }: { section: Section; onClose: () => void }) {
  const { t } = useTranslation()
  const localized = useLocalized()
  const firstError = useFirstError()
  const [save, { error, isLoading }] = useSetSectionInstructorsMutation()
  const [items, setItems] = useState<Assignment[]>(
    section.instructors.map((i) => ({
      staffId: i.staff_id,
      name: [i.title, i.first_name, i.last_name].filter(Boolean).join(' '),
      role: i.role,
    })),
  )
  const [query, setQuery] = useState('')
  const q = useDebounced(query.trim())
  const results = useSearchInstructorsQuery({ q, limit: 6 }, { skip: q.length < 2 })

  const submit = async () => {
    const res = await save({
      id: section.id,
      body: { items: items.map((i) => ({ staff_id: i.staffId, role: i.role })) },
    })
    if (res.data) {
      toast.success(t('offering.instructors.saved'))
      onClose()
    }
  }

  return (
    <FormDialog
      title={t('offering.instructors.title', { code: section.section_code })}
      description={t('offering.instructors.description')}
      error={firstError(error)}
      onClose={onClose}
      onSubmit={() => void submit()}
      submit={
        <Button type="submit" loading={isLoading}>
          {t('common.save')}
        </Button>
      }
    >
      {items.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t('offering.section.noInstructors')}</p>
      ) : (
        <ul className="space-y-2">
          {items.map((it, idx) => (
            <li key={it.staffId} className="flex items-center gap-2">
              <span className="flex-1 text-sm">{it.name}</span>
              <Select
                value={it.role}
                onValueChange={(v) =>
                  setItems(items.map((x, i) => (i === idx ? { ...x, role: v as InstructorRole } : x)))
                }
              >
                <SelectTrigger
                  className="w-40"
                  aria-label={t('offering.instructors.role', { name: it.name })}
                >
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {(['PRIMARY', 'CO_INSTRUCTOR', 'ASSISTANT'] as const).map((r) => (
                    <SelectItem key={r} value={r}>
                      {t(`teaching.roles.${r}`)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <Button
                type="button"
                variant="ghost"
                size="icon"
                onClick={() => setItems(items.filter((_, i) => i !== idx))}
                aria-label={t('offering.instructors.remove', { name: it.name })}
              >
                <Trash2Icon />
              </Button>
            </li>
          ))}
        </ul>
      )}
      <div className="space-y-2">
        <div className="relative">
          <SearchIcon className="absolute top-2.5 left-3 size-4 text-muted-foreground" aria-hidden />
          <Input
            type="search"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder={t('schedule.instructorPlaceholder')}
            aria-label={t('offering.instructors.search')}
            className="pl-9"
          />
        </div>
        {q.length >= 2 && results.data && (
          <ul className="rounded-md border" aria-label={t('catalog.search.results')}>
            {results.data.items
              .filter((s) => !items.some((i) => i.staffId === s.staff_id))
              .map((s) => {
                const name = [s.title, s.first_name, s.last_name].filter(Boolean).join(' ')
                return (
                  <li key={s.staff_id}>
                    <button
                      type="button"
                      className="flex w-full items-center justify-between gap-2 px-3 py-2 text-left text-sm hover:bg-accent focus-visible:bg-accent focus-visible:outline-none"
                      onClick={() => {
                        setItems([
                          ...items,
                          {
                            staffId: s.staff_id,
                            name,
                            role: items.some((i) => i.role === 'PRIMARY') ? 'CO_INSTRUCTOR' : 'PRIMARY',
                          },
                        ])
                        setQuery('')
                      }}
                    >
                      <span>{name}</span>
                      {s.department && (
                        <span className="text-xs text-muted-foreground">{localized(s.department)}</span>
                      )}
                    </button>
                  </li>
                )
              })}
          </ul>
        )}
      </div>
    </FormDialog>
  )
}

/** QuotasDialog, RESERVED şubede programlara ayrılan kontenjanları düzenler. */
export function QuotasDialog({ section, onClose }: { section: Section; onClose: () => void }) {
  const { t } = useTranslation()
  const localized = useLocalized()
  const firstError = useFirstError()
  const programs = useListProgramsQuery({ limit: 100 })
  const [save, { error, isLoading }] = useSetSectionQuotasMutation()
  const [rows, setRows] = useState(
    section.quotas.map((q) => ({ programId: q.program.id, quota: String(q.quota) })),
  )
  const total = rows.reduce((sum, r) => sum + (Number(r.quota) || 0), 0)

  const submit = async () => {
    const res = await save({
      id: section.id,
      body: {
        items: rows
          .filter((r) => r.programId)
          .map((r) => ({ program_id: r.programId, quota: Number(r.quota) })),
      },
    })
    if (res.data) {
      toast.success(t('offering.quotas.saved'))
      onClose()
    }
  }

  return (
    <FormDialog
      title={t('offering.quotas.title', { code: section.section_code })}
      description={t('offering.quotas.description', { total, capacity: section.capacity })}
      error={firstError(error)}
      onClose={onClose}
      onSubmit={() => void submit()}
      submit={
        <Button type="submit" loading={isLoading}>
          {t('common.save')}
        </Button>
      }
    >
      <ul className="space-y-2">
        {rows.map((r, idx) => (
          <li key={idx} className="flex items-center gap-2">
            <Select
              value={r.programId}
              onValueChange={(v) => setRows(rows.map((x, i) => (i === idx ? { ...x, programId: v } : x)))}
            >
              <SelectTrigger className="min-w-0 flex-1" aria-label={t('org.program')}>
                <SelectValue placeholder={t('org.program')} />
              </SelectTrigger>
              <SelectContent>
                {programs.data?.items.map((p) => (
                  <SelectItem key={p.id} value={p.id}>
                    {p.code} · {localized(p)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Input
              value={r.quota}
              onChange={(e) => setRows(rows.map((x, i) => (i === idx ? { ...x, quota: e.target.value } : x)))}
              inputMode="numeric"
              className="w-20"
              aria-label={t('offering.quotas.quota')}
            />
            <Button
              type="button"
              variant="ghost"
              size="icon"
              onClick={() => setRows(rows.filter((_, i) => i !== idx))}
              aria-label={t('offering.quotas.remove')}
            >
              <Trash2Icon />
            </Button>
          </li>
        ))}
      </ul>
      <Button
        type="button"
        variant="outline"
        size="sm"
        onClick={() => setRows([...rows, { programId: '', quota: '10' }])}
      >
        {t('offering.quotas.add')}
      </Button>
    </FormDialog>
  )
}
