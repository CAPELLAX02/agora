import { useTranslation } from 'react-i18next'

import {
  useListFacultiesQuery,
  useListFacultyDepartmentsQuery,
  useListProgramsQuery,
} from '@/shared/api/generated'
import { Field } from '@/shared/ui/field'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/shared/ui/select'

import { emptyOrg, type OrgSelection } from './orgSelection'

/**
 * OrgPicker, birim → bölüm → program zincirini seçtirir. depth, ne kadar derine
 * inileceğidir (ör. bölüm kapsamlı bir rol için 'department').
 */
export function OrgPicker({
  depth,
  value,
  onChange,
  error,
}: {
  depth: 'faculty' | 'department' | 'program'
  value: OrgSelection
  onChange: (v: OrgSelection) => void
  error?: string | undefined
}) {
  const { t, i18n } = useTranslation()
  const en = i18n.language === 'en'
  const faculties = useListFacultiesQuery({})
  const departments = useListFacultyDepartmentsQuery(
    { id: value.facultyId },
    { skip: !value.facultyId || depth === 'faculty' },
  )
  const programs = useListProgramsQuery(
    { departmentId: value.departmentId, limit: 100 },
    { skip: !value.departmentId || depth !== 'program' },
  )

  const name = (x: { name_tr: string; name_en?: string }) => (en && x.name_en ? x.name_en : x.name_tr)

  return (
    <div className="grid gap-4">
      <Field label={t('org.faculty')} error={depth === 'faculty' ? error : undefined}>
        {(p) => (
          <Select value={value.facultyId} onValueChange={(v) => onChange({ ...emptyOrg, facultyId: v })}>
            <SelectTrigger {...p} className="w-full">
              <SelectValue placeholder={t('org.choose')} />
            </SelectTrigger>
            <SelectContent>
              {faculties.data?.items.map((f) => (
                <SelectItem key={f.id} value={f.id}>
                  {name(f)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        )}
      </Field>
      {depth !== 'faculty' && (
        <Field label={t('org.department')} error={depth === 'department' ? error : undefined}>
          {(p) => (
            <Select
              value={value.departmentId}
              disabled={!value.facultyId}
              onValueChange={(v) => onChange({ ...value, departmentId: v, programId: '' })}
            >
              <SelectTrigger {...p} className="w-full">
                <SelectValue placeholder={t('org.choose')} />
              </SelectTrigger>
              <SelectContent>
                {departments.data?.items.map((d) => (
                  <SelectItem key={d.id} value={d.id}>
                    {name(d)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          )}
        </Field>
      )}
      {depth === 'program' && (
        <Field label={t('org.program')} error={error}>
          {(p) => (
            <Select
              value={value.programId}
              disabled={!value.departmentId}
              onValueChange={(v) => onChange({ ...value, programId: v })}
            >
              <SelectTrigger {...p} className="w-full">
                <SelectValue placeholder={t('org.choose')} />
              </SelectTrigger>
              <SelectContent>
                {programs.data?.items.map((pr) => (
                  <SelectItem key={pr.id} value={pr.id}>
                    {pr.code} · {name(pr)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          )}
        </Field>
      )}
    </div>
  )
}
